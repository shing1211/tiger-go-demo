# tiger-go-demo â€” Makefile
#
# Go is resolved from PATH. Override it if your install is elsewhere:
#
#     make verify GO=/usr/local/go/bin/go
#
# These targets are a convenience layer. The gate itself is scripts/verify, and
# it is four go commands, so the same checks run on a machine with no GNU make
# installed at all.

GO      ?= go
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
	$(GO) build -o $(BIN)/token     ./cmd/token
	@echo "built: quote trade push options futures reference corporate token"

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

.PHONY: run-token
run-token: ## Run the token command (bearer token in memory, local push subscription state)
	$(GO) run ./cmd/token $(ARGS)

.PHONY: test
test: ## Run unit tests with the race detector
	$(GO) test -count=1 -race $(PKG)

.PHONY: test-norace
test-norace: ## Run the suite without the race detector (no C toolchain needed)
	TIGER_NO_RACE=1 $(GO) test -count=1 $(PKG)

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

# The uncovered SDK methods are asserted exactly, not counted: a new gap fails,
# and so does a method that gained a call site.
#
# The check itself is Go, not shell. It was a 40-line grep/sed/comm pipeline
# needing mktemp, grep -E, sed -E, cmp and comm, and so could only run where
# those exist. It now lives in internal/sdkcoverage, where the reasoning it
# used to carry in comments here is recorded in the doc comments of the code
# that enforces it — next to the behaviour, where it cannot drift from it.
.PHONY: coverage-check
coverage-check: ## Fail unless the uncovered SDK methods are exactly the allow-list
	@$(GO) test -count=1 -run TestSDKCoverage -v ./test/

.PHONY: docs-check
docs-check: ## Fail unless the README's test counts and coverage table match a real run
	@GO=$(GO) ./scripts/check-docs

.PHONY: verify
verify: fmt-check vet test build coverage-check docs-check ## Everything CI should run

.PHONY: demo-dry-run
demo-dry-run: ## Prove the safety gate refuses a real order (no credentials needed beyond dummy)
	$(GO) run ./cmd/trade -command place -symbol AAPL -quantity 1 -limit-price 100 ; \
	echo "exit=$$?"

.PHONY: clean
clean: ## Remove build output
	rm -rf $(BIN)
	$(GO) clean -testcache
