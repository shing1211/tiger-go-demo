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
build: ## Compile all binaries into ./bin
	@mkdir -p $(BIN)
	$(GO) build -o $(BIN)/quote     ./cmd/quote
	$(GO) build -o $(BIN)/trade     ./cmd/trade
	$(GO) build -o $(BIN)/push      ./cmd/push
	$(GO) build -o $(BIN)/options   ./cmd/options
	$(GO) build -o $(BIN)/futures   ./cmd/futures
	$(GO) build -o $(BIN)/reference ./cmd/reference
	$(GO) build -o $(BIN)/corporate ./cmd/corporate
	@echo "built: quote trade push options futures reference corporate"

.PHONY: run-quote
run-quote: ## Run the quote command (read-only market data)
	$(GO) run ./cmd/quote $(ARGS)

.PHONY: run-trade
run-trade: ## Run the trade command (queries, or gated writes)
	$(GO) run ./cmd/trade $(ARGS)

.PHONY: run-push
run-push: ## Run the push command (real-time subscription feed)
	$(GO) run ./cmd/push $(ARGS)

.PHONY: run-options
run-options: ## Run the options command (read-only option market data)
	$(GO) run ./cmd/options $(ARGS)

.PHONY: run-futures
run-futures: ## Run the futures command (read-only futures market data)
	$(GO) run ./cmd/futures $(ARGS)

.PHONY: run-reference
run-reference: ## Run the reference command (read-only reference and fundamental data)
	$(GO) run ./cmd/reference $(ARGS)

.PHONY: run-corporate
run-corporate: ## Run the corporate command (read-only corporate actions, warrants, funds)
	$(GO) run ./cmd/corporate $(ARGS)

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
