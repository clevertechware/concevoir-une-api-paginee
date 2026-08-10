.PHONY: help build run test test-unit test-integration explain mocks vet fmt tidy clean

BIN := bin

help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-18s\033[0m %s\n", $$1, $$2}'

build: ## Build the API binary
	go build -o $(BIN)/server ./cmd/server

run: ## Run the API on :8080 (needs `make db-up seed` at the repository root)
	go run ./cmd/server

test: ## Run every test (requires Docker)
	go test -count=1 ./...

test-unit: ## Run the fast tests only, no Docker needed
	go test -short -race -count=1 ./...

test-integration: ## Run the testcontainers-backed tests
	go test -v -count=1 ./internal/postgres/...

explain: ## Show only the plan measurements, with their numbers
	go test -v -count=1 -run 'TestRepositorySuite/TestExplain' ./internal/postgres/...

mocks: ## Regenerate the testify mocks declared in .mockery.yml
	go tool mockery

vet: ## Run go vet
	go vet ./...

fmt: ## Format the code
	gofmt -s -w .

tidy: ## Tidy go.mod
	go mod tidy

clean: ## Remove build artifacts
	rm -rf $(BIN)
	go clean
