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
# Reasons are one word each; see README "What the uncovered SDK methods are".
#
# "internal" is a claim about the SDK, so it is only earned by a call site inside
# the SDK's library packages. Execute/QueryToken/SecretKey/StartTokenAutoRefresh
# have one (quote/quote_client.go, client/http_client.go, trade/trade_client.go).
# ExecuteRaw does NOT, and is therefore "not-used" like the user-facing escape
# hatch it is: it has zero callers at all, inside or outside the module. The
# other three methods that once sat here as "not-used" — RefreshToken,
# SetCurrentToken and GetAccountSubscriptions — are now reached, from
# cmd/token. They were "not-used" rather than "internal" for as long as they
# were uncovered, because labelling them "internal" would assert something the
# SDK source does not support: nothing in the SDK's own library packages calls
# them, only the example programs in examples/manual_test/ and
# cmd/integ_token_refresh/. That is still true of the SDK, and is still why
# labelling them "internal" would be wrong if they ever became uncovered again;
# but a covered method carries no allow-list line, so the question no longer
# arises for them.
#
# Scanned: the four client types the SDK ships — QuoteClient 78, TradeClient 39,
# PushClient 34, HttpClient 8 = 159. The receiver pattern is deliberately
# receiver-name-agnostic ("\([a-zA-Z_][A-Za-z0-9_]* \*?[A-Za-z]+Client\)" rather
# than the hardcoded "\(c \*"). Pinning "c" is the latent bug the sibling
# longbridge-go-demo README documents: the day the SDK renames one receiver, the
# pattern matches nothing for that type and the whole type silently drops out of
# the denominator — no failure, just a smaller, greener number.
#
# "*?" is insurance, NOT a fix for a live bug: none of the four scanned client
# packages declares a value receiver today (measured — 0 of 82/42/46/14 receiver
# decls in quote/trade/push/client are value receivers). Value receivers do exist
# elsewhere in the module (model/, logger/), so the pattern must not forbid them
# outright: a future SDK that adds one to a client type would otherwise drop that
# method out of the denominator silently, the same failure as pinning a receiver
# name. Keep the "*?"; do not cite it as covering a present-day value receiver.
#
# KNOWN FALSE POSITIVES — this matcher is by METHOD NAME, not by receiver type.
# "HttpClient.Close" is reported covered by nine ".Close(" sites under cmd/ and
# internal/, but only two of those are the SDK's: internal/tigersdk/tigersdk.go
# calls s.HTTP.Close() and s.QuoteHTTP.Close(). The other seven are our own
# Session.Close (cmd/quote/main.go:112, cmd/trade/main.go:235), env.Close
# (cmd/options, cmd/corporate, cmd/futures, cmd/reference — 4) and
# internal/rocli/rocli.go:113 e.Session.Close(). A green run is therefore
# NOT evidence that HttpClient.Close is exercised — it is only evidence that
# the word "Close(" appears somewhere in the tree. Every other method on the
# list is a quote/trade/push name, so the collision is harmless today; it would
# not be if a future SDK method shared a name with a helper of ours.
#
# The glob stays FLAT ("$base/$pkg"/*.go, not -r) on purpose. push/pb/ is
# generated protobuf: 436 exported method declarations over 32 receiver types
# (43 declared types), 278 of them GetX() field accessors and 87 of them
# Reset/String/ProtoReflect — hundreds of call sites no SDK user would ever
# write. A flat glob cannot reach push/pb/ at all, so that cost is structural
# rather than a flag that might be dropped.
#
# --exclude-dir=pb states that structurally-guaranteed exclusion explicitly.
# It is honest documentation, NOT a load-bearing guard, and the distinction was
# measured rather than assumed: making the glob recursive while keeping this
# flag leaves the count at 159, and making it recursive while DROPPING the flag
# ALSO leaves it at 159 — because no pb receiver type ends in "Client" (0 of 32)
# and the string "Client" never appears in push/pb at all, so the *Client
# suffix already filters pb out. The flag only starts to matter if the receiver
# pattern is ever widened off the Client suffix; with a widened pattern, -r alone
# inflates the scan to 320 methods / 107 uncovered, and -r plus this flag holds
# it to 163. So: keep the flat glob (that is the real guard), keep the flag (it
# is the correct intent and covers the widened-pattern case), and do not read a
# green run as evidence that pb is being excluded by the flag.
#
# --exclude='*_test.go' keeps the SDK's own tests out of the scan. Twelve test
# files live under push/ and client/; measured, none declares a method on the
# four client types today, so the counts are identical either way — the flag is
# insurance, not arithmetic. push/coverage_extra_test.go (43KB) and
# client/coverage_extra_test.go (26KB) are precisely the kind of file that grows
# a helper method on the type under test, and a test helper is not a call site
# this target is claiming coverage for.
.PHONY: coverage-check
coverage-check: ## Fail unless the uncovered SDK methods are exactly the allow-list
	@set -eu; \
	sdk=$$($(GO) list -m -f '{{.Version}}' github.com/tigerfintech/openapi-go-sdk); \
	base="$$($(GO) env GOMODCACHE)/github.com/tigerfintech/openapi-go-sdk@$$sdk"; \
	if [ ! -d "$$base" ]; then echo "SDK not in module cache: $$base"; exit 1; fi; \
	tmp=$$(mktemp -d); \
	trap 'rm -rf "$$tmp"' EXIT; \
	all=$$tmp/all; got=$$tmp/got; want=$$tmp/want; \
	for pair in quote:QuoteClient trade:TradeClient push:PushClient client:HttpClient; do \
		f=$${pair%%:*}; c=$${pair##*:}; \
		grep -hoE "^func \([a-zA-Z_][A-Za-z0-9_]* \*?[A-Za-z]+Client\) [A-Z][A-Za-z0-9]*" \
		  "$$base/$$f"/*.go --exclude-dir=pb --exclude='*_test.go' \
		  | sed -E 's/.*\) //'; \
	done | LC_ALL=C sort -u > $$all; \
	while read -r m; do \
		grep -rqE "\.$$m\(" cmd/ internal/ || echo "$$m"; \
	done < $$all > $$got; \
	for e in GetBrief:deprecated GetBars:deprecated GetBarsByPage:deprecated \
	         GetOptionBrief:deprecated GetWarrantBriefs:deprecated \
	         GetStockDelayBriefs:deprecated GrabQuotePermission:mutating \
	         PlaceForexOrder:mutating TransferSegmentFund:mutating \
	         CancelSegmentFund:mutating TransferPosition:mutating \
	         OptionExerciseSubmit:mutating OptionExerciseCancel:mutating \
	         SetSecretKey:not-a-call \
	         ExecuteRaw:not-used \
	         Execute:internal QueryToken:internal \
	         SecretKey:internal \
	         StartTokenAutoRefresh:internal; do echo "$$e"; done > $$tmp/allowed; \
	sed 's/:.*//' $$tmp/allowed | LC_ALL=C sort > $$want; \
	total=$$(wc -l < $$all | tr -d ' '); \
	left=$$(wc -l < $$got | tr -d ' '); \
	echo "sdk coverage: $$((total - left))/$$total methods covered, $$left uncovered"; \
	if LC_ALL=C cmp -s $$got $$want; then \
		echo "uncovered set matches the allow-list:"; \
		sed 's/^/  /' $$tmp/allowed; \
		exit 0; \
	fi; \
	echo "FAIL: the uncovered set is not the allow-list."; \
	newgap=$$(LC_ALL=C comm -13 $$want $$got); \
	fixed=$$(LC_ALL=C comm -23 $$want $$got); \
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
