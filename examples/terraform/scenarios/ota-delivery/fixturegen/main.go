// Command fixturegen is the ota-delivery scenario's own minimal build and
// release pipeline. It is written from the public Expo Updates Protocol v1
// shape; it is NOT a port of any production pipeline (see the scenario README
// and the brief's boundary note). Its only job is to produce a believable,
// signed artefact set so the CDN read-path assertions have something to fetch.
//
// Subcommands:
//
//	publish  build a bundle + asset, build and sign an Expo Updates Protocol v1
//	         manifest with the Key Vault key, render a multipart/mixed object,
//	         and upload the immutable artefacts to the Blob container (Azurite).
//	promote  server-side copy the pre-signed multipart to the live manifest.json
//	         path and write rollout.json. No signing, no Key Vault access: this
//	         is the release half's zero-trust boundary.
//	verify   fetch the manifest, rollout, and an asset through the CDN host and
//	         check the multipart Content-Type, the cache TTLs, and the manifest
//	         signature against the exported public key.
//
// Authentication to Azurite uses Shared Key (the well-known dev key azemu's
// listKeys returns). Reads on the CDN read path are anonymous and are checked
// by the verify subcommand. Operational logs go to stderr via zerolog; stdout
// carries only the VERSION=/ASSET= machine-readable contract that e2e.sh reads.
package main

import (
	"bytes"
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

func main() {
	if len(os.Args) < 2 {
		fail("usage: fixturegen <publish|promote|verify> [flags]")
	}
	var err error
	switch os.Args[1] {
	case "publish":
		err = runPublish(os.Args[2:])
	case "promote":
		err = runPromote(os.Args[2:])
	case "verify":
		err = runVerify(os.Args[2:])
	default:
		fail(fmt.Sprintf("unknown subcommand %q", os.Args[1]))
	}
	if err != nil {
		fail(err.Error())
	}
}

// fail logs the message via the repo-standard structured logger (stderr) and
// exits non-zero. stdout is reserved for the machine-readable VERSION=/ASSET=
// contract that e2e.sh parses.
func fail(msg string) {
	log.Error().Msg("fixturegen: " + msg)
	os.Exit(1)
}

// config is the shared set of inputs both subcommands need, read from flags
// (which the make target fills from terraform outputs).
type config struct {
	azurite   string // blob service base, e.g. http://127.0.0.1:10000
	account   string // storage account name
	accntKey  string // base64 Shared Key
	container string // blob container, e.g. ota
	prefix    string // blob path prefix: {rv}/{channel}/{platform}
	version   int    // v{n}
	kid       string // versioned Key Vault key id (sign endpoint base)
	cacert    string // PEM bundle to trust for the Key Vault TLS call
}

// parseFlags reads the common -flag value pairs both subcommands accept and
// returns the assembled config, failing if the required account/key are absent.
func parseFlags(args []string) config {
	c := config{azurite: "http://127.0.0.1:10000", container: "ota", version: 1}
	for i := 0; i+1 < len(args); i += 2 {
		v := args[i+1]
		switch args[i] {
		case "-azurite":
			c.azurite = strings.TrimRight(v, "/")
		case "-account":
			c.account = v
		case "-key":
			c.accntKey = v
		case "-container":
			c.container = v
		case "-prefix":
			c.prefix = strings.Trim(v, "/")
		case "-version":
			if n, err := strconv.Atoi(v); err == nil {
				c.version = n
			}
		case "-kid":
			c.kid = v
		case "-cacert":
			c.cacert = v
		}
	}
	if c.account == "" || c.accntKey == "" {
		fail("-account and -key are required")
	}
	return c
}

const cacheImmutable = "public, max-age=31536000, immutable"
const cacheShort = "public, max-age=30"
const multipartBoundary = "azemu-ota-boundary"
const multipartType = "multipart/mixed; boundary=" + multipartBoundary

// blobPath returns the full Azurite path for a blob: {container}/{prefix}/rest.
func (c config) blobPath(rest string) string {
	return fmt.Sprintf("%s/%s/%s", c.container, c.prefix, strings.TrimLeft(rest, "/"))
}

// versionDir returns the blob path for an artefact under the current version's
// immutable directory: {container}/{prefix}/v{n}/{rest}.
func (c config) versionDir(rest string) string {
	return c.blobPath(fmt.Sprintf("v%d/%s", c.version, rest))
}

// b64url returns unpadded base64url, the encoding the Expo protocol and the
// Key Vault sign API both use for digests and signatures.
func b64url(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

// runPublish builds and signs an update, then uploads the immutable artefacts.
func runPublish(args []string) error {
	c := parseFlags(args)

	// 1. Build a deterministic bundle and one asset, content-addressed by hash.
	bundle := []byte("// azemu ota-delivery fixture bundle\nexport const version = " +
		fmt.Sprint(c.version) + ";\n")
	asset := []byte("azemu-ota-fixture-asset-v" + fmt.Sprint(c.version))
	bundleHash := sha256.Sum256(bundle)
	assetHash := sha256.Sum256(asset)
	bundleName := "bundle-" + hexShort(bundleHash[:]) + ".js"
	assetName := "asset-" + hexShort(assetHash[:]) + ".bin"

	// 2. Build the Expo Updates Protocol v1 manifest.
	manifest := map[string]interface{}{
		"id":             fmt.Sprintf("00000000-0000-4000-8000-%012d", c.version),
		"createdAt":      time.Unix(int64(1_700_000_000+c.version), 0).UTC().Format(time.RFC3339),
		"runtimeVersion": pathSegment(c.prefix, 0),
		"launchAsset": map[string]interface{}{
			"key":         bundleName,
			"contentType": "application/javascript",
			"url":         bundleName,
			"hash":        b64url(bundleHash[:]),
		},
		"assets": []interface{}{
			map[string]interface{}{
				"key":         assetName,
				"contentType": "application/octet-stream",
				"url":         assetName,
				"hash":        b64url(assetHash[:]),
			},
		},
		"metadata": map[string]interface{}{},
		"extra":    map[string]interface{}{},
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}

	// 3. Sign the manifest digest with the Key Vault key (RS256 over SHA-256).
	digest := sha256.Sum256(manifestBytes)
	sig, err := c.signWithKeyVault(digest[:])
	if err != nil {
		return fmt.Errorf("key vault sign: %w", err)
	}

	// 4. Render the multipart/mixed object, carrying the signature in the
	// manifest part header so a server-side copy preserves it.
	multipart := renderMultipart(manifestBytes, sig)

	// 5. Upload the immutable artefacts. The live manifest.json + rollout.json
	// are written by promote, not here.
	if err := c.createContainer(); err != nil {
		return fmt.Errorf("create container: %w", err)
	}
	uploads := []struct {
		path, contentType, cache string
		body                     []byte
	}{
		{c.versionDir(bundleName), "application/javascript", cacheImmutable, bundle},
		{c.versionDir(assetName), "application/octet-stream", cacheImmutable, asset},
		{c.versionDir("update.multipart"), multipartType, cacheShort, multipart},
	}
	for _, u := range uploads {
		// Immutable artefacts are written once with overwrite disabled
		// (If-None-Match: *), so a re-publish of the same version cannot silently
		// replace already-published bytes.
		if err := c.putBlob(u.path, u.contentType, u.cache, u.body, true); err != nil {
			return fmt.Errorf("put %s: %w", u.path, err)
		}
		log.Info().Str("blob", u.path).Msg("published immutable artefact")
	}
	// Machine-readable contract on stdout for e2e.sh; everything else is stderr.
	fmt.Printf("VERSION=%d\nASSET=%s\n", c.version, assetName)
	return nil
}

// runPromote copies the pre-signed multipart to the live manifest and writes
// rollout.json. It performs no signing and needs no Key Vault access.
func runPromote(args []string) error {
	c := parseFlags(args)

	// Server-side copy the pre-signed multipart to the live manifest path. The
	// copy preserves the multipart Content-Type and the signature in the part
	// header, so promotion never re-signs.
	src := c.versionDir("update.multipart")
	if err := c.copyBlob(src, c.blobPath("manifest.json")); err != nil {
		return fmt.Errorf("promote copy: %w", err)
	}
	log.Info().Str("from", src).Str("to", c.blobPath("manifest.json")).Msg("promoted to live manifest")

	// Write rollout state. Minimal shape: the client self-selects its cohort.
	rollout := fmt.Sprintf(`{"version":%d,"percent":100}`, c.version)
	// rollout.json is a live, mutable object: each promotion overwrites it.
	if err := c.putBlob(c.blobPath("rollout.json"), "application/json", cacheShort, []byte(rollout), false); err != nil {
		return fmt.Errorf("put rollout.json: %w", err)
	}
	log.Info().Str("blob", c.blobPath("rollout.json")).Str("rollout", rollout).Msg("wrote rollout state")
	return nil
}

// renderMultipart wraps the manifest bytes in a multipart/mixed body, carrying
// the signature in the manifest part header so a server-side copy preserves it.
func renderMultipart(manifestBytes []byte, sig string) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "--%s\r\n", multipartBoundary)
	fmt.Fprint(&b, "Content-Type: application/json\r\n")
	fmt.Fprint(&b, "Content-Disposition: form-data; name=\"manifest\"\r\n")
	fmt.Fprintf(&b, "expo-signature: sig=\"%s\", keyid=\"manifest-signing-key\", alg=\"rsa-v1_5-sha256\"\r\n", sig)
	fmt.Fprint(&b, "\r\n")
	fmt.Fprintf(&b, "%s", manifestBytes)
	fmt.Fprintf(&b, "\r\n--%s--\r\n", multipartBoundary)
	return b.Bytes()
}

// signWithKeyVault POSTs the digest to the Key Vault sign endpoint and returns
// the base64url RS256 signature, the same call az keyvault key sign makes.
func (c config) signWithKeyVault(digest []byte) (string, error) {
	if c.kid == "" {
		return "", fmt.Errorf("-kid (versioned key id) is required for publish")
	}
	reqBody, _ := json.Marshal(map[string]string{"alg": "RS256", "value": b64url(digest)})
	req, err := http.NewRequest(http.MethodPost, c.kid+"/sign?api-version=7.4", bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.kvClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("sign returned %d: %s", resp.StatusCode, string(body))
	}
	var out struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("decode sign response: %w", err)
	}
	if out.Value == "" {
		return "", fmt.Errorf("sign response had no value: %s", string(body))
	}
	return out.Value, nil
}

// kvClient returns an HTTP client for the Key Vault TLS call. It trusts the
// azemu cert bundle (when -cacert is given) and remaps the per-vault host
// ({vault}.vault.localhost) to 127.0.0.1, so the call works whether or not the
// host resolves *.localhost to loopback (GitHub runners do; many dev hosts do
// not).
func (c config) kvClient() *http.Client {
	tr := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if host, port, err := net.SplitHostPort(addr); err == nil &&
				strings.HasSuffix(host, ".vault.localhost") {
				addr = "127.0.0.1:" + port
			}
			return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, addr)
		},
	}
	if c.cacert != "" {
		if pemBytes, err := os.ReadFile(c.cacert); err == nil {
			pool := x509.NewCertPool()
			if pool.AppendCertsFromPEM(pemBytes) {
				tr.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
			}
		}
	}
	// Overall timeout so a stalled Key Vault response cannot hang the publish.
	return &http.Client{Transport: tr, Timeout: 30 * time.Second}
}

// --- Azurite Shared Key data-plane helpers ---

// createContainer creates the blob container with public blob read access,
// tolerating a 409 when it already exists from a prior run.
func (c config) createContainer() error {
	u := fmt.Sprintf("%s/%s/%s?restype=container", c.azurite, c.account, c.container)
	req, err := http.NewRequest(http.MethodPut, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("x-ms-blob-public-access", "blob")
	resp, err := c.signedDo(req, "")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// 201 Created, or 409 if the container already exists from a prior run.
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusConflict {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("create container returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// putBlob uploads a block blob with the given Content-Type and (stored)
// Cache-Control. When immutable is set the write is overwrite-disabled via
// If-None-Match: *, so re-publishing an existing version fails instead of
// silently replacing bytes.
func (c config) putBlob(path, contentType, cache string, body []byte, immutable bool) error {
	u := fmt.Sprintf("%s/%s/%s", c.azurite, c.account, path)
	req, err := http.NewRequest(http.MethodPut, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("x-ms-blob-type", "BlockBlob")
	req.Header.Set("Content-Type", contentType)
	// The stored blob's Cache-Control comes from x-ms-blob-cache-control on Put
	// Blob (the plain Cache-Control header is a request directive, not stored).
	// Copy Blob then carries this value to the promoted manifest.
	req.Header.Set("x-ms-blob-cache-control", cache)
	if immutable {
		// Overwrite disabled: the conditional PUT fails (412) if the blob exists.
		req.Header.Set("If-None-Match", "*")
	}
	req.ContentLength = int64(len(body))
	resp, err := c.signedDo(req, contentType)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		rb, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("put blob returned %d: %s", resp.StatusCode, string(rb))
	}
	return nil
}

// copyBlob server-side copies a blob. Azure Copy Blob preserves the source's
// Content-Type and Cache-Control on the destination, so promoting the
// short-TTL, multipart/mixed update.multipart to manifest.json carries both
// across without a separate Set Blob Properties call (which would clear the
// Content-Type it does not name).
func (c config) copyBlob(srcPath, dstPath string) error {
	src := fmt.Sprintf("%s/%s/%s", c.azurite, c.account, srcPath)
	u := fmt.Sprintf("%s/%s/%s", c.azurite, c.account, dstPath)
	req, err := http.NewRequest(http.MethodPut, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("x-ms-copy-source", src)
	resp, err := c.signedDo(req, "")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusCreated {
		rb, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("copy blob returned %d: %s", resp.StatusCode, string(rb))
	}
	return nil
}

// signedDo stamps the request with the Date and Shared Key Authorization header
// Azurite requires for writes, then sends it.
// azuriteHTTPClient carries an overall timeout so a stalled Azurite response
// cannot hang publish/promote indefinitely (a connect-only dial timeout does
// not bound the full request).
var azuriteHTTPClient = &http.Client{Timeout: 30 * time.Second}

// signedDo stamps the request with the date and Shared Key Authorization header
// Azurite requires for writes, then sends it via the timeout-bounded client.
func (c config) signedDo(req *http.Request, contentType string) (*http.Response, error) {
	date := time.Now().UTC().Format(http.TimeFormat)
	req.Header.Set("x-ms-date", date)
	req.Header.Set("x-ms-version", "2021-08-06")

	stringToSign, err := c.stringToSign(req, contentType)
	if err != nil {
		return nil, err
	}
	key, err := base64.StdEncoding.DecodeString(c.accntKey)
	if err != nil {
		return nil, fmt.Errorf("decode account key: %w", err)
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(stringToSign))
	sig := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	req.Header.Set("Authorization", "SharedKey "+c.account+":"+sig)
	return azuriteHTTPClient.Do(req)
}

// stringToSign builds the canonical Shared Key string for blob requests, per
// the Azure Storage Shared Key (blob) signing scheme.
func (c config) stringToSign(req *http.Request, contentType string) (string, error) {
	contentLength := ""
	if req.ContentLength > 0 {
		contentLength = fmt.Sprint(req.ContentLength)
	}
	canonHeaders := canonicalizedHeaders(req.Header)
	canonResource, err := c.canonicalizedResource(req.URL)
	if err != nil {
		return "", err
	}
	parts := []string{
		req.Method,
		"",                              // Content-Encoding
		"",                              // Content-Language
		contentLength,                   // Content-Length ("" when 0)
		"",                              // Content-MD5
		contentType,                     // Content-Type
		"",                              // Date (using x-ms-date instead)
		"",                              // If-Modified-Since
		req.Header.Get("If-Match"),      // If-Match
		req.Header.Get("If-None-Match"), // If-None-Match
		"",                              // If-Unmodified-Since
		"",                              // Range
		canonHeaders + canonResource,
	}
	return strings.Join(parts, "\n"), nil
}

// canonicalizedHeaders builds the sorted, lowercased x-ms-* header block for
// the Shared Key string-to-sign.
func canonicalizedHeaders(h http.Header) string {
	var keys []string
	for k := range h {
		lk := strings.ToLower(k)
		if strings.HasPrefix(lk, "x-ms-") {
			keys = append(keys, lk)
		}
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%s:%s\n", k, strings.TrimSpace(h.Get(k)))
	}
	return b.String()
}

// canonicalizedResource builds the "/account/path" plus sorted query block for
// the Shared Key string-to-sign.
func (c config) canonicalizedResource(u *url.URL) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "/%s%s", c.account, u.EscapedPath())
	q := u.Query()
	var keys []string
	for k := range q {
		keys = append(keys, strings.ToLower(k))
	}
	sort.Strings(keys)
	for _, k := range keys {
		vals := q[k]
		sort.Strings(vals)
		fmt.Fprintf(&b, "\n%s:%s", k, strings.Join(vals, ","))
	}
	return b.String(), nil
}

// runVerify asserts the Front Door read path: it fetches the live manifest,
// rollout, and an asset through the {endpoint}.azurefd.net host and checks the cache
// TTLs, the multipart Content-Type, the rollout body, and the manifest
// signature against the public key the client would embed. All assertions live
// here (rather than in fragile shell) so the read path is checked robustly.
func runVerify(args []string) error {
	var fqdn, basePath, pubkeyPath, cacert, asset string
	port := "4566"
	version := 1
	for i := 0; i+1 < len(args); i += 2 {
		v := args[i+1]
		switch args[i] {
		case "-fqdn":
			fqdn = v
		case "-base":
			basePath = strings.Trim(v, "/")
		case "-pubkey":
			pubkeyPath = v
		case "-cacert":
			cacert = v
		case "-asset":
			asset = v
		case "-port":
			port = v
		case "-version":
			if n, err := strconv.Atoi(v); err == nil {
				version = n
			}
		}
	}
	if fqdn == "" || basePath == "" || pubkeyPath == "" {
		return fmt.Errorf("-fqdn, -base, and -pubkey are required")
	}
	client := cdnClient(fqdn, port, cacert)
	base := fmt.Sprintf("https://%s:%s/%s", fqdn, port, basePath)

	// 1. manifest.json: multipart/mixed, short TTL, signature verifies.
	mResp, mBody, err := fetch(client, base+"/manifest.json")
	if err != nil {
		return err
	}
	if ct := mResp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "multipart/mixed") {
		return fmt.Errorf("manifest Content-Type = %q, want multipart/mixed", ct)
	}
	if err := assertShortTTL(mResp.Header.Get("Cache-Control")); err != nil {
		return fmt.Errorf("manifest cache: %w", err)
	}
	if err := verifyManifestSignature(mResp, mBody, pubkeyPath); err != nil {
		return fmt.Errorf("manifest signature: %w", err)
	}
	log.Info().Msg("ok: manifest.json multipart + short TTL + signature verified")

	// 2. rollout.json: expected body, short TTL.
	rResp, rBody, err := fetch(client, base+"/rollout.json")
	if err != nil {
		return err
	}
	wantRollout := fmt.Sprintf(`{"version":%d,"percent":100}`, version)
	if strings.TrimSpace(string(rBody)) != wantRollout {
		return fmt.Errorf("rollout body = %q, want %q", strings.TrimSpace(string(rBody)), wantRollout)
	}
	if err := assertShortTTL(rResp.Header.Get("Cache-Control")); err != nil {
		return fmt.Errorf("rollout cache: %w", err)
	}
	log.Info().Msg("ok: rollout.json body + short TTL")

	// 3. asset: immutable long TTL.
	if asset != "" {
		aResp, _, err := fetch(client, fmt.Sprintf("%s/v%d/%s", base, version, asset))
		if err != nil {
			return err
		}
		if cc := aResp.Header.Get("Cache-Control"); !strings.Contains(cc, "immutable") {
			return fmt.Errorf("asset Cache-Control = %q, want immutable", cc)
		}
		log.Info().Msg("ok: asset immutable TTL")
	}
	return nil
}

// cdnClient builds an HTTP client that resolves the CDN host to 127.0.0.1 (the
// equivalent of curl --resolve) and trusts the azemu cert bundle.
func cdnClient(fqdn, port, cacert string) *http.Client {
	tr := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if addr == fqdn+":"+port {
				addr = "127.0.0.1:" + port
			}
			return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, addr)
		},
	}
	if cacert != "" {
		if pem, err := os.ReadFile(cacert); err == nil {
			pool := x509.NewCertPool()
			if pool.AppendCertsFromPEM(pem) {
				tr.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
			}
		}
	}
	// Overall timeout so verify cannot block forever on a wedged CDN endpoint.
	return &http.Client{Transport: tr, Timeout: 30 * time.Second}
}

// fetch GETs a URL, reads the body, and errors on any non-200 status.
func fetch(client *http.Client, urlStr string) (*http.Response, []byte, error) {
	resp, err := client.Get(urlStr)
	if err != nil {
		return nil, nil, fmt.Errorf("GET %s: %w", urlStr, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("GET %s returned %d: %s", urlStr, resp.StatusCode, string(body))
	}
	return resp, body, nil
}

// assertShortTTL checks the Cache-Control carries a max-age in the 20-60s band
// the brief requires for the live (frequently-changing) objects.
func assertShortTTL(cc string) error {
	m := regexp.MustCompile(`max-age=(\d+)`).FindStringSubmatch(cc)
	if m == nil {
		return fmt.Errorf("no max-age in %q", cc)
	}
	age, _ := strconv.Atoi(m[1])
	if age < 20 || age > 60 {
		return fmt.Errorf("max-age=%d not in the 20-60s band", age)
	}
	return nil
}

// verifyManifestSignature parses the multipart body, extracts the manifest part
// and its expo-signature, and verifies the RS256 signature against the public
// key, mirroring the client's verification step.
func verifyManifestSignature(resp *http.Response, body []byte, pubkeyPath string) error {
	mediaType, params, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || !strings.HasPrefix(mediaType, "multipart/") {
		return fmt.Errorf("not a multipart response: %v", err)
	}
	mr := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	part, err := mr.NextPart()
	if err != nil {
		return fmt.Errorf("read manifest part: %w", err)
	}
	manifestBytes, err := io.ReadAll(part)
	if err != nil {
		return fmt.Errorf("read manifest body: %w", err)
	}
	sigB64 := parseExpoSignature(part.Header.Get("expo-signature"))
	if sigB64 == "" {
		return fmt.Errorf("no expo-signature in manifest part header")
	}
	sig, err := base64.RawURLEncoding.DecodeString(sigB64)
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}

	pubPEM, err := os.ReadFile(pubkeyPath)
	if err != nil {
		return fmt.Errorf("read public key: %w", err)
	}
	block, _ := pem.Decode(pubPEM)
	if block == nil {
		return fmt.Errorf("public key is not PEM")
	}
	pubAny, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("parse public key: %w", err)
	}
	rsaPub, ok := pubAny.(*rsa.PublicKey)
	if !ok {
		return fmt.Errorf("public key is not RSA")
	}
	digest := sha256.Sum256(manifestBytes)
	return rsa.VerifyPKCS1v15(rsaPub, crypto.SHA256, digest[:], sig)
}

// parseExpoSignature pulls the base64url sig out of an expo-signature header of
// the form: sig="<b64url>", keyid="...", alg="...".
func parseExpoSignature(h string) string {
	m := regexp.MustCompile(`sig="([^"]+)"`).FindStringSubmatch(h)
	if m == nil {
		return ""
	}
	return m[1]
}

// hexShort returns the first 8 bytes of a hash as a 16-char hex string, used to
// content-address bundle and asset filenames.
func hexShort(b []byte) string {
	const hexdigits = "0123456789abcdef"
	out := make([]byte, 16)
	for i := 0; i < 8; i++ {
		out[i*2] = hexdigits[b[i]>>4]
		out[i*2+1] = hexdigits[b[i]&0x0f]
	}
	return string(out)
}

// pathSegment returns the i-th slash-separated segment of the prefix, used to
// pull the runtime version out of {rv}/{channel}/{platform}.
func pathSegment(prefix string, i int) string {
	segs := strings.Split(prefix, "/")
	if i < len(segs) {
		return segs[i]
	}
	return ""
}
