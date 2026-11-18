package arm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/zerodeth/azemu/internal/store"
)

const apimImportLimit = 8 << 20

type apimSwaggerImport struct {
	properties  map[string]any
	operations  map[string]map[string]any
	definitions map[string]any
}

// Swagger imports are management-plane documents, not gateway handlers.
// Retain the complete source alongside the ARM metadata and materialize the
// imported operations/schemas so an import is not just a successful no-op.
func importAPIMSwagger(ctx context.Context, format string, value any) (*apimSwaggerImport, error) {
	if format != "swagger-json" && format != "swagger-link-json" {
		return nil, fmt.Errorf("unsupported import format %q: only swagger-json and swagger-link-json are implemented", format)
	}
	content, ok := value.(string)
	if !ok || content == "" {
		return nil, fmt.Errorf("a nonempty import value is required")
	}
	if format == "swagger-link-json" {
		u, err := url.Parse(content)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
			return nil, fmt.Errorf("import link must be an absolute HTTP(S) URL without credentials")
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, content, nil)
		if err != nil {
			return nil, fmt.Errorf("build import request: %w", err)
		}
		client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 || (req.URL.Scheme != "https" && req.URL.Scheme != "http") || req.URL.User != nil {
				return fmt.Errorf("unsupported import redirect")
			}
			return nil
		}}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetch import document: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("import link returned HTTP %d", resp.StatusCode)
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, apimImportLimit+1))
		if err != nil {
			return nil, fmt.Errorf("read import document: %w", err)
		}
		content = string(data)
	}
	if len(content) > apimImportLimit {
		return nil, fmt.Errorf("import document exceeds 8 MiB")
	}
	var document map[string]any
	if err := json.Unmarshal([]byte(content), &document); err != nil {
		return nil, fmt.Errorf("invalid Swagger JSON: %w", err)
	}
	if document["swagger"] != "2.0" {
		return nil, fmt.Errorf("only Swagger 2.0 documents are implemented")
	}
	info, ok := document["info"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("missing Swagger info")
	}
	title, _ := info["title"].(string)
	version, _ := info["version"].(string)
	if title == "" || version == "" {
		return nil, fmt.Errorf("missing Swagger info.title or info.version")
	}
	// External references cannot be resolved by this importer. Reject them
	// before persisting anything rather than publishing an incomplete API.
	definitions, _ := document["definitions"].(map[string]any)
	if err := validateAPIMSwaggerRefs(document, definitions); err != nil {
		return nil, err
	}
	props := map[string]any{"displayName": title, "_importDocument": document}
	for source, target := range map[string]string{
		"description": "description", "contact": "contact", "license": "license", "termsOfService": "termsOfServiceUrl",
	} {
		if v, ok := info[source]; ok {
			props[target] = v
		}
	}
	protocols, _ := document["schemes"].([]any)
	if len(protocols) == 0 {
		protocols = []any{"https"}
	}
	for _, protocol := range protocols {
		if protocol != "http" && protocol != "https" {
			return nil, fmt.Errorf("only HTTP(S) Swagger schemes are implemented")
		}
	}
	props["protocols"] = protocols
	if host, _ := document["host"].(string); host != "" {
		basePath, _ := document["basePath"].(string)
		backend := protocols[0].(string) + "://" + host + basePath
		u, err := url.Parse(backend)
		if err != nil || u.Host == "" || u.User != nil {
			return nil, fmt.Errorf("invalid Swagger host/basePath")
		}
		props["serviceUrl"] = backend
	}
	paths, ok := document["paths"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("missing Swagger paths object")
	}
	result := &apimSwaggerImport{properties: props, operations: map[string]map[string]any{}}
	result.definitions = map[string]any{}
	if definitions, ok := document["definitions"].(map[string]any); ok {
		for name, definition := range definitions {
			result.definitions[name] = definition
		}
	}
	for path, raw := range paths {
		item, ok := raw.(map[string]any)
		if !ok || !strings.HasPrefix(path, "/") {
			return nil, fmt.Errorf("invalid Swagger path %q", path)
		}
		if _, ref := item["$ref"]; ref {
			return nil, fmt.Errorf("referenced Swagger path items are not implemented")
		}
		for method, rawOp := range item {
			switch method {
			case "get", "put", "post", "delete", "options", "head", "patch":
			default:
				continue
			}
			op, ok := rawOp.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid Swagger operation %s %s", method, path)
			}
			operationID, _ := op["operationId"].(string)
			if operationID == "" {
				operationID = method + "-" + strings.Trim(path, "/")
			}
			operationID = strings.Trim(apimOperationUnsafe.ReplaceAllString(operationID, "-"), "-")
			if operationID == "" || result.operations[operationID] != nil {
				return nil, fmt.Errorf("empty or duplicate imported operation ID")
			}
			display, _ := op["summary"].(string)
			if display == "" {
				display = operationID
			}
			operation := map[string]any{
				"displayName": display, "method": strings.ToUpper(method), "urlTemplate": path,
				"description": op["description"],
			}
			parameters := []any{}
			for _, source := range []map[string]any{item, op} {
				if list, ok := source["parameters"].([]any); ok {
					parameters = append(parameters, list...)
				}
			}
			template, query, headers, representations, form := []any{}, []any{}, []any{}, []any{}, []any{}
			for _, rawParam := range parameters {
				param, ok := rawParam.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("invalid Swagger parameter")
				}
				if _, ref := param["$ref"]; ref {
					return nil, fmt.Errorf("referenced Swagger parameters are not implemented")
				}
				converted := map[string]any{
					"name": param["name"], "type": param["type"], "required": param["required"] == true,
					"description": param["description"],
				}
				if param["type"] == "array" {
					if items, ok := param["items"].(map[string]any); ok {
						if itemType, ok := items["type"].(string); ok {
							converted["type"] = itemType + "[]"
						}
						for _, key := range []string{"default", "enum"} {
							if v, ok := items[key]; ok {
								field := map[string]string{"default": "defaultValue", "enum": "values"}[key]
								converted[field] = v
							}
						}
					}
				}
				for _, field := range []string{"default", "enum"} {
					if v, ok := param[field]; ok {
						key := map[string]string{"default": "defaultValue", "enum": "values"}[field]
						converted[key] = v
					}
				}
				switch param["in"] {
				case "path":
					template = append(template, converted)
				case "query":
					query = append(query, converted)
				case "header":
					headers = append(headers, converted)
				case "body":
					representations = append(representations, apimSwaggerRepresentations(result, document, op, "consumes", operationID+"-request", param["schema"])...)
				case "formData":
					form = append(form, converted)
				default:
					return nil, fmt.Errorf("unsupported Swagger parameter location")
				}
			}
			if len(form) != 0 {
				types, ok := op["consumes"].([]any)
				if !ok {
					types = []any{"multipart/form-data"}
				}
				for _, contentType := range types {
					representations = append(representations, map[string]any{
						"contentType": contentType, "formParameters": form,
					})
				}
			}
			operation["templateParameters"] = template
			operation["request"] = map[string]any{"queryParameters": query, "headers": headers, "representations": representations}
			responses := []any{}
			rawResponses, ok := op["responses"].(map[string]any)
			if !ok {
				return nil, fmt.Errorf("missing Swagger operation responses")
			}
			for code, rawResponse := range rawResponses {
				response, ok := rawResponse.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("invalid Swagger response")
				}
				if _, ref := response["$ref"]; ref {
					return nil, fmt.Errorf("referenced Swagger responses are not implemented")
				}
				status, err := strconv.Atoi(code)
				if code == "default" {
					status = 0
				} else if err != nil {
					return nil, fmt.Errorf("invalid Swagger response code %q", code)
				}
				responseHeaders := []any{}
				if values, ok := response["headers"].(map[string]any); ok {
					for name, value := range values {
						if header, ok := value.(map[string]any); ok {
							responseHeaders = append(responseHeaders, map[string]any{
								"name": name, "type": header["type"], "description": header["description"],
							})
						}
					}
				}
				responses = append(responses, map[string]any{
					"statusCode": status, "description": response["description"],
					"headers":         responseHeaders,
					"representations": apimSwaggerRepresentations(result, document, op, "produces", operationID+"-response-"+code, response["schema"]),
				})
			}
			operation["responses"] = responses
			result.operations[operationID] = operation
		}
	}
	return result, nil
}

var apimOperationUnsafe = regexp.MustCompile(`[^a-zA-Z0-9_.-]+`)

func apimSwaggerDefinitionName(ref string) string {
	name := strings.TrimPrefix(ref, "#/definitions/")
	// Decode ~1 before ~0 so ~01 denotes the literal ~1, not a slash.
	return strings.ReplaceAll(strings.ReplaceAll(name, "~1", "/"), "~0", "~")
}

func validateAPIMSwaggerRefs(value any, definitions map[string]any) error {
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			if key == "$ref" {
				ref, ok := child.(string)
				if !ok || !strings.HasPrefix(ref, "#/definitions/") {
					return fmt.Errorf("only local Swagger definition references are implemented")
				}
				name := apimSwaggerDefinitionName(ref)
				if _, exists := definitions[name]; !exists {
					return fmt.Errorf("unresolved Swagger definition reference %q", ref)
				}
			}
			if err := validateAPIMSwaggerRefs(child, definitions); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range v {
			if err := validateAPIMSwaggerRefs(child, definitions); err != nil {
				return err
			}
		}
	}
	return nil
}

func apimSwaggerRepresentations(imported *apimSwaggerImport, document, operation map[string]any, key, schemaName string, schema any) []any {
	result := []any{}
	if schema == nil {
		return result
	}
	contentTypes, ok := operation[key].([]any)
	if !ok {
		contentTypes, _ = document[key].([]any)
	}
	if len(contentTypes) == 0 {
		contentTypes = []any{"application/json"}
	}
	for _, contentType := range contentTypes {
		representation := map[string]any{"contentType": contentType}
		if object, ok := schema.(map[string]any); ok {
			representation["schemaId"] = "swagger-definitions"
			if ref, ok := object["$ref"].(string); ok {
				representation["typeName"] = apimSwaggerDefinitionName(ref)
			} else {
				// Inline objects, arrays and primitives also need a schema,
				// otherwise their structure disappears from operation reads.
				name := schemaName
				for imported.definitions[name] != nil {
					name += "-inline"
				}
				imported.definitions[name] = object
				representation["typeName"] = name
			}
		}
		result = append(result, representation)
	}
	return result
}

func (a *Router) saveAPIMSwagger(id string, imported *apimSwaggerImport) error {
	for _, collection := range []string{"operations", "schemas"} {
		for _, old := range a.store.List(id + "/" + collection + "/") {
			// Explicit operation PUTs take ownership of imported operations
			// and may add operations not present in the document. Re-import
			// refreshes projected children, not these managed contracts.
			if old.Type == apimAPIOperationType && old.Properties["_explicitOperation"] == true {
				continue
			}
			a.store.Delete(old.ID)
		}
	}
	for name, props := range imported.operations {
		childID := id + "/operations/" + name
		if old, exists := a.store.Get(childID); exists && old.Properties["_explicitOperation"] == true {
			continue
		}
		if err := a.store.Put(childID, &store.Resource{
			ID: childID, Name: name, Type: apimAPIType + "/operations", Properties: props,
		}); err != nil {
			return fmt.Errorf("save imported operation: %w", err)
		}
	}
	if len(imported.definitions) != 0 {
		document, err := json.Marshal(map[string]any{"definitions": imported.definitions})
		if err != nil {
			return fmt.Errorf("encode imported definitions: %w", err)
		}
		childID := id + "/schemas/swagger-definitions"
		if err := a.store.Put(childID, &store.Resource{
			ID: childID, Name: "swagger-definitions", Type: apimAPIType + "/schemas",
			Properties: map[string]any{
				"contentType": "application/vnd.ms-azure-apim.swagger.definitions+json",
				"document":    map[string]any{"value": string(document)},
			},
		}); err != nil {
			return fmt.Errorf("save imported definitions: %w", err)
		}
	}
	return nil
}
