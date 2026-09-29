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

# The uncovered set is asserted exactly, not counted: a new gap fails, and so
# does a method that gained a call site (then you delete its line here).
# Reasons are one word each; see README "What the 14 uncovered methods are".
.PHONY: coverage-check
coverage-check: ## Fail unless the uncovered SDK methods are exactly the allow-list
	@set -eu; \
	sdk=$$($(GO) list -m -f '{{.Version}}' github.com/tigerfintech/openapi-go-sdk); \
	base="$$($(GO) env GOMODCACHE)/github.com/tigerfintech/openapi-go-sdk@$$sdk"; \
	if [ ! -d "$$base" ]; then echo "SDK not in module cache: $$base"; exit 1; fi; \
	tmp=$$(mktemp -d); \
	trap 'rm -rf "$$tmp"' EXIT; \
	all=$$tmp/all; got=$$tmp/got; want=$$tmp/want; \
	for pair in quote:Quote trade:Trade; do \
		f=$${pair%%:*}; c=$${pair##*:}; \
		grep -hoE "^func \(c \*$${c}Client\) [A-Z][A-Za-z0-9]*" "$$base/$$f"/*.go \
		  | sed -E 's/.*\) //'; \
	done | sort -u > $$all; \
	while read -r m; do \
		grep -rqE "\.$$m\(" cmd/ internal/ || echo "$$m"; \
	done < $$all > $$got; \
	for e in GetBrief:deprecated GetBars:deprecated GetBarsByPage:deprecated \
	         GetOptionBrief:deprecated GetWarrantBriefs:deprecated \
	         GetStockDelayBriefs:deprecated GrabQuotePermission:mutating \
	         PlaceForexOrder:mutating TransferSegmentFund:mutating \
	         CancelSegmentFund:mutating TransferPosition:mutating \
	         OptionExerciseSubmit:mutating OptionExerciseCancel:mutating \
	         SetSecretKey:not-a-call; do echo "$$e"; done > $$tmp/allowed; \
	sed 's/:.*//' $$tmp/allowed | sort > $$want; \
	total=$$(wc -l < $$all | tr -d ' '); \
	left=$$(wc -l < $$got | tr -d ' '); \
	echo "sdk coverage: $$((total - left))/$$total methods covered, $$left uncovered"; \
	if cmp -s $$got $$want; then \
		echo "uncovered set matches the allow-list:"; \
		sed 's/^/  /' $$tmp/allowed; \
		exit 0; \
	fi; \
	echo "FAIL: the uncovered set is not the allow-list."; \
	newgap=$$(comm -13 $$want $$got); \
	fixed=$$(comm -23 $$want $$got); \
	if [ -n "$$newgap" ]; then \
		echo "not on the allow-list — cover it, or justify it and add a line above:"; \
		for n in $$newgap; do echo "  UNCOVERED: $$n"; done; \
	fi; \
	if [ -n "$$fixed" ]; then \
		echo "now covered — delete from the allow-list above:"; \
		for n in $$fixed; do grep -E "^$$n:" $$tmp/allowed | sed 's/^/  /'; done; \
	fi; \
	exit 1

.PHONY: verify
verify: fmt-check vet test build coverage-check ## Everything CI should run

.PHONY: demo-dry-run
demo-dry-run: ## Prove the safety gate refuses a real order (no credentials needed beyond dummy)
	$(GO) run ./cmd/trade -command place -symbol AAPL -quantity 1 -limit-price 100 ; \
	echo "exit=$$?"

.PHONY: clean
clean: ## Remove build output
	rm -rf $(BIN)
	$(GO) clean -testcache
