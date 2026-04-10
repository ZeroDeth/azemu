.PHONY: build run test docker clean

BINARY := azemu
MODULE := github.com/zerodeth/azemu

build:
	go build -o bin/$(BINARY) ./cmd/azemu

run: build
	./bin/$(BINARY)

test:
	go test ./... -v -count=1

docker:
	docker build -t azemu:latest .

docker-run: docker
	docker run --rm -p 4566:4566 -p 4567:4567 azemu:latest

clean:
	rm -rf bin/

# Quick smoke test: metadata endpoint should return JSON
smoke:
	@echo "Starting azemu..."
	@./bin/$(BINARY) &
	@sleep 1
	@echo "\n--- Metadata endpoints ---"
	@curl -sk https://localhost:4567/metadata/endpoints | jq .name
	@echo "\n--- List subscriptions ---"
	@curl -s http://localhost:4566/subscriptions?api-version=2022-12-01 | jq '.value[0].subscriptionId'
	@echo "\n--- Create resource group ---"
	@curl -s -X PUT http://localhost:4566/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/test-rg?api-version=2023-07-01 \
		-H 'Content-Type: application/json' \
		-d '{"location":"uksouth"}' | jq .name
	@echo "\n--- Get resource group ---"
	@curl -s http://localhost:4566/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/test-rg?api-version=2023-07-01 | jq .
	@kill %1 2>/dev/null || true
