# career — build, test and lint targets.
#
# Go lives at /usr/local/go and is symlinked into ~/.local/bin; if `make` cannot
# find it, run `make doctor`.

GO      ?= go
BIN     := bin/career
PKG     := ./...
GOFILES := $(shell find . -name '*.go' -not -path './bin/*')

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: ## Build the career binary into bin/
	$(GO) build -o $(BIN) ./cmd/career

.PHONY: install
install: ## Install career into $GOBIN (or ~/go/bin)
	$(GO) install ./cmd/career

.PHONY: test
test: ## Run all tests
	$(GO) test $(PKG)

.PHONY: race
race: ## Run tests under the race detector
	$(GO) test -race $(PKG)

.PHONY: cover
cover: ## Report per-package test coverage
	$(GO) test -cover $(PKG)

.PHONY: cover-html
cover-html: ## Open an HTML coverage report
	$(GO) test -coverprofile=coverage.out $(PKG)
	$(GO) tool cover -html=coverage.out

.PHONY: fmt
fmt: ## Format all Go source
	gofmt -w $(GOFILES)

.PHONY: vet
vet: ## Run go vet
	$(GO) vet $(PKG)

.PHONY: lint
lint: ## Fail if anything is unformatted, then vet
	@unformatted=$$(gofmt -l $(GOFILES)); \
	if [ -n "$$unformatted" ]; then \
		echo "unformatted files:"; echo "$$unformatted"; exit 1; \
	fi
	$(GO) vet $(PKG)

.PHONY: check
check: lint test ## Everything CI runs

.PHONY: tidy
tidy: ## Tidy go.mod and go.sum
	$(GO) mod tidy

.PHONY: clean
clean: ## Remove build artefacts
	rm -rf bin coverage.out

.PHONY: doctor
doctor: ## Diagnose a missing or misconfigured Go toolchain
	@command -v $(GO) >/dev/null 2>&1 \
		&& echo "go: $$($(GO) version)" \
		|| { echo "go not on PATH."; \
		     echo "Go is installed at /usr/local/go. Fix with:"; \
		     echo "  ln -sf /usr/local/go/bin/go /usr/local/go/bin/gofmt ~/.local/bin/"; \
		     exit 1; }
	@echo "GOPATH: $$($(GO) env GOPATH)"
