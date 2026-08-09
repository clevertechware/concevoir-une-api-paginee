.PHONY: help db-up db-down db-shell seed bench go-run go-test java-run java-test test clean

# 10 000 000 is the article's dataset. Use ROWS=100000 for a quick loop; the
# depths hard-coded in sql/benchmark.sql then no longer exist.
ROWS ?= 10000000

PSQL := docker compose exec -T postgres psql -U postgres -d pagination -v ON_ERROR_STOP=1

help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-14s\033[0m %s\n", $$1, $$2}'

db-up: ## Start PostgreSQL and apply the schema
	docker compose up -d --wait

db-down: ## Stop PostgreSQL and drop its volume
	docker compose down -v

db-shell: ## Open a psql prompt
	docker compose exec postgres psql -U postgres -d pagination

seed: ## Load the dataset (override with ROWS=100000)
	@echo "seeding $(ROWS) transactions, this takes a while for 10M…"
	$(PSQL) -v rows=$(ROWS) < sql/seed.sql

bench: ## Replay the measurements quoted in the article
	$(PSQL) < sql/benchmark.sql

go-run: ## Run the Go API on :8080
	$(MAKE) -C go run

go-test: ## Run the Go test suite
	$(MAKE) -C go test

java-run: ## Run the Spring Boot API on :8081
	$(MAKE) -C spring-boot run

java-test: ## Run the Spring Boot test suite
	$(MAKE) -C spring-boot test

test: go-test java-test ## Run both test suites

clean: ## Remove build artifacts from both projects
	$(MAKE) -C go clean
	$(MAKE) -C spring-boot clean
