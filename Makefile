.PHONY: build run test docker docker-run docker-compose docker-compose-down clean smoke tf-test tf-test-scenarios tf-test-scenarios-v4 tf-test-scenarios-v5 coverage ota-delivery console-build console-dev

BINARY  := azemu
MODULE  := github.com/zerodeth/azemu
VERSION ?= dev
TF_SCENARIO_RUNNER ?= native

build:
	go build -ldflags "-X main.Version=$(VERSION)" -o bin/$(BINARY) ./cmd/azemu

run: build
	./bin/$(BINARY)

test:
	go test ./... -v -count=1

coverage:
	go test ./... -coverprofile=coverage.out -covermode=atomic
	go tool cover -html=coverage.out -o coverage.html
	@echo "open coverage.html"

docker:
	docker build -t azemu:latest .

docker-run: docker
	docker run --rm -p 4566:4566 -p 4567:4567 -p 4568:4568 azemu:latest

docker-compose:
	docker compose up -d --build

docker-compose-down:
	docker compose down -v

tf-test:
	cd examples/terraform && terraform init -input=false && terraform test

# Keep the existing scenario command as the v4 compatibility alias.
tf-test-scenarios: tf-test-scenarios-v4

tf-test-scenarios-v4: SCENARIO_ROOT = examples/terraform/scenarios
tf-test-scenarios-v4: PROVIDER_MAJOR = v4
tf-test-scenarios-v5: SCENARIO_ROOT = examples/terraform/scenarios-v5
tf-test-scenarios-v5: PROVIDER_MAJOR = v5

# Each provider major has its own scenarios and lockfiles. Do not pass
# -upgrade: intentional provider upgrades should update that suite's lockfiles.
tf-test-scenarios-v4 tf-test-scenarios-v5:
	@sh scripts/test-terraform-scenarios.sh "$(TF_SCENARIO_RUNNER)" "$(SCENARIO_ROOT)" "$(PROVIDER_MAJOR)"

console-build:
	cd console && npm ci && npm run build
	rm -rf internal/console/dist
	cp -r console/dist internal/console/dist

console-dev:
	cd console && npm run dev

clean:
	rm -rf bin/ coverage.out coverage.html

# Quick smoke test: metadata endpoint should return JSON
smoke:
	@echo "Starting azemu..."
	@./bin/$(BINARY) &
	@sleep 1
	@echo ""
	@echo "--- Metadata endpoints ---"
	@curl -sfk https://localhost:4567/metadata/endpoints | jq .name
	@echo ""
	@echo "--- List subscriptions ---"
	@curl -sfk https://localhost:4566/subscriptions?api-version=2022-12-01 | jq '.value[0].subscriptionId'
	@echo ""
	@echo "--- Create resource group ---"
	@curl -sfk -X PUT https://localhost:4566/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/test-rg?api-version=2023-07-01 \
		-H 'Content-Type: application/json' \
		-d '{"location":"uksouth"}' | jq .name
	@echo ""
	@echo "--- Get resource group ---"
	@curl -sfk https://localhost:4566/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/test-rg?api-version=2023-07-01 | jq .
	@echo ""
	@echo "--- Delete resource group ---"
	@curl -sfk -X DELETE -o /dev/null -w "%{http_code}" https://localhost:4566/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/test-rg?api-version=2023-07-01
	@echo ""
	@echo "--- GET deleted resource group (expect 404) ---"
	@curl -sfk --fail -o /dev/null https://localhost:4566/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/test-rg?api-version=2023-07-01 && echo "FAIL: expected 404" && kill %1 2>/dev/null && exit 1 || echo "OK: got 404"
	@echo ""
	@echo "--- Bare request without api-version (expect 400) ---"
	@curl -sfk --fail -o /dev/null https://localhost:4566/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/no-version && echo "FAIL: expected 400" && kill %1 2>/dev/null && exit 1 || echo "OK: got 400"
	@echo ""
	@echo "--- All smoke tests passed ---"
	@kill %1 2>/dev/null || true

# End-to-end OTA delivery scenario (local-only): brings up the stack, then
# provisions ARM, publishes a signed update, promotes it, and asserts the CDN
# read path before tearing everything down. CI runs only the ARM-half tftest
# (examples/terraform/scenarios/ota-delivery/main.tftest.hcl) via
# tf-test-scenarios; this target adds the publish + serve-assert loop that needs
# the live Azurite data plane.
ota-delivery:
	@echo "Starting azemu + Azurite..."
	@docker compose up -d --build
	@ready=0; for i in $$(seq 1 30); do \
		if curl -sf http://localhost:4568/health >/dev/null 2>&1; then ready=1; break; fi; \
		echo "waiting for azemu ($$i/30)..."; sleep 2; \
	done; \
	if [ "$$ready" -ne 1 ]; then echo "azemu never became healthy"; docker compose down -v; exit 1; fi
	@SSL_CERT_FILE=$$PWD/.azemu/cert-bundle.pem bash examples/terraform/scenarios/ota-delivery/e2e.sh; \
		status=$$?; docker compose down -v; exit $$status
