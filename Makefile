.PHONY: build run test docker docker-run docker-compose docker-compose-down clean smoke tf-test coverage

BINARY  := azemu
MODULE  := github.com/zerodeth/azemu
VERSION ?= dev

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
	@for dir in examples/terraform examples/terraform/scenarios/*/; do \
		echo "--- terraform test: $$dir ---"; \
		(cd "$$dir" && terraform init -upgrade -input=false && terraform test) || exit 1; \
	done

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
