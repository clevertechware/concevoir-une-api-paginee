.PHONY: help run build package test test-unit test-integration clean

MVN ?= ./mvnw

help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-16s\033[0m %s\n", $$1, $$2}'

run: ## Run the API on :8081 against localhost:5432 (make db-up at the repository root first)
	$(MVN) spring-boot:run

build: ## Compile
	$(MVN) -B compile

package: ## Build the executable jar, tests included
	$(MVN) -B -Pintegration clean package

test: test-unit test-integration ## Run the whole suite

test-unit: ## Run the fast tests, no Docker required
	$(MVN) -B test

test-integration: ## Run the Testcontainers tests, Docker required
	$(MVN) -B -Pintegration-only test

clean: ## Remove target/
	$(MVN) -B clean
