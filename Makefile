# tiger-go-demo — Makefile
#
# Go is not on PATH by default on this machine; the GO variable below prepends
# /usr/local/go/bin so every target works from a plain shell.

GO      ?= /usr/local/go/bin/go
BIN     ?= bin
PKG     := ./...
.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) \
		| awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: ## Compile all three binaries into ./bin
	@mkdir -p $(BIN)
	$(GO) build -o $(BIN)/quote  ./cmd/quote
	$(GO) build -o $(BIN)/trade  ./cmd/trade
	$(GO) build -o $(BIN)/push   ./cmd/push
	@echo "built: $(BIN)/quote $(BIN)/trade $(BIN)/push"

.PHONY: run-quote
run-quote: ## Run the quote command (read-only market data)
	$(GO) run ./cmd/quote $(ARGS)

.PHONY: run-trade
run-trade: ## Run the trade command (queries, or gated writes)
	$(GO) run ./cmd/trade $(ARGS)

.PHONY: run-push
run-push: ## Run the push command (real-time subscription feed)
	$(GO) run ./cmd/push $(ARGS)

.PHONY: test
test: ## Run unit tests
	$(GO) test -race $(PKG)

.PHONY: vet
vet: ## Run go vet
	$(GO) vet $(PKG)

.PHONY: fmt
fmt: ## Format all Go files
	$(GO) fmt $(PKG)

.PHONY: fmt-check
fmt-check: ## Fail if any file is unformatted
	@out="$$(gofmt -l .)"; \
	if [ -n "$$out" ]; then echo "unformatted files:"; echo "$$out"; exit 1; fi; \
	echo "gofmt: clean"

.PHONY: tidy
tidy: ## Tidy go.mod / go.sum
	$(GO) mod tidy

.PHONY: verify
verify: fmt-check vet test build ## Everything CI should run

.PHONY: demo-dry-run
demo-dry-run: ## Prove the safety gate refuses a real order (no credentials needed beyond dummy)
	$(GO) run ./cmd/trade -command place -symbol AAPL -quantity 1 -limit-price 100 ; \
	echo "exit=$$?"

.PHONY: clean
clean: ## Remove build output
	rm -rf $(BIN)
	$(GO) clean -testcache
