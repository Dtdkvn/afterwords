GO ?= go
NPM ?= npm
VERSION ?= dev
BIN_DIR ?= bin
TOOLS_DIR ?= .tools/bin
GOLANGCI_LINT := $(TOOLS_DIR)/golangci-lint
override PROJECT_ROOT := $(realpath $(dir $(lastword $(MAKEFILE_LIST))))

.DEFAULT_GOAL := help

.PHONY: help setup dev lint test e2e build demo clean

help: ## Show the available commands.
	@awk 'BEGIN {FS = ":.*## "; printf "Afterwords development commands\n\n"} /^[a-zA-Z_-]+:.*## / {printf "  %-10s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

setup: ## Download Go, frontend, and browser-test dependencies.
	$(GO) mod download
	mkdir -p $(TOOLS_DIR)
	GOBIN=$(abspath $(TOOLS_DIR)) $(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2
	cd web && $(NPM) ci
	@if [ -f e2e/package-lock.json ]; then cd e2e && $(NPM) ci; fi
	@if [ -f e2e/package.json ]; then cd e2e && $(NPM) run install:browsers --if-present; fi

dev: ## Run the API, Vite, and Mailpit in containers.
	docker compose -f docker-compose.dev.yml up --build

lint: ## Check Go formatting/static analysis and frontend lint rules.
	@test -z "$$(gofmt -l $$(find . -name '*.go' -not -path './vendor/*'))" || (echo "Go files are not formatted; run gofmt -w." && exit 1)
	$(GO) vet ./...
	@test -x $(GOLANGCI_LINT) || (echo "golangci-lint is missing; run make setup." && exit 1)
	$(GOLANGCI_LINT) run ./...
	cd web && $(NPM) run lint --if-present

test: lint ## Run backend and browser-crypto unit tests.
	$(GO) test -race -cover ./...
	cd web && $(NPM) test

e2e: ## Run the Playwright release-flow suite.
	@if [ -f e2e/package.json ]; then cd e2e && $(NPM) test; else cd web && $(NPM) run e2e; fi

build: ## Build the embedded production frontend and single binary.
	cd web && $(NPM) run build
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o $(BIN_DIR)/afterwords ./cmd/afterwords

demo: ## Run the accelerated-time demo scenario.
	bash scripts/demo.sh

clean: ## Remove generated build and test artifacts.
	@case "$(PROJECT_ROOT)" in ""|/|[A-Za-z]:/) echo "Refusing to clean an unsafe project root: $(PROJECT_ROOT)" >&2; exit 1;; esac
	@test -f "$(PROJECT_ROOT)/Makefile" && test -d "$(PROJECT_ROOT)/internal/server/webdist" || (echo "Refusing to clean outside an Afterwords checkout." >&2; exit 1)
	rm -rf -- "$(PROJECT_ROOT)/bin" "$(PROJECT_ROOT)/internal/server/webdist/assets" "$(PROJECT_ROOT)/web/coverage" "$(PROJECT_ROOT)/e2e/test-results" "$(PROJECT_ROOT)/e2e/playwright-report"
