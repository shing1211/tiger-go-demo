# tiger-go-demo

A self-contained, safe-by-default Go demo of the **Tiger Brokers OpenAPI**,
built on the official SDK [`github.com/tigerfintech/openapi-go-sdk`](https://github.com/tigerfintech/openapi-go-sdk) v0.5.2.

Eight commands. Seven are read-only; one can write, and only behind two
independent gates:

| Command | What it does | Writes orders? |
|---|---|---|
| `cmd/quote` | Real-time briefs, historical bars, intraday timeline, market state, order-book depth, addon-plan entitlement | **No** — read-only |
| `cmd/options` | Option expiries, chains (plain and with Greeks), quotes, k-lines, depth, ticks, timeline, symbols, implied vol | **No** — read-only |
| `cmd/futures` | Contract metadata (exchanges, current/all/continuous contracts, trading times) plus quotes, k-lines, depth, ticks | **No** — read-only |
| `cmd/reference` | Symbol lists and names, stock details, fundamentals, financial series, FX, short interest, trading calendar, market scanner, industries | **No** — read-only |
| `cmd/corporate` | Dividends, splits, earnings calendar, IPOs, symbol changes, delistings, capital flow, HK warrants, fund NAVs | **No** — read-only |
| `cmd/push` | Real-time push feed (TCP + TLS + Protobuf): quotes, ticks, depth, k-lines, crypto, whole-market and rankings, order/position/asset/fill updates | **No** — read-only |
| `cmd/token` | Bearer token in memory, and this process's local record of which account feeds it subscribed to | **No** — read-only |
| `cmd/trade` | 29 read commands — account state, orders, contracts, funds, transfers, option exercise — plus place / modify / cancel behind a hard gate | **Yes**, behind two independent gates |

Only `cmd/trade` has a write path at all. The other seven binaries never construct
a trade client, so there is no code in them that *could* place an order.

---

## ⚠️ Requirements, before you can run any of this

**Tiger OpenAPI has no paper or simulated mode.** There is no sandbox. Every
order you submit goes to a **real, funded brokerage account**. Read this
section before typing anything.

To use this project you need all of the following:

1. **A real Tiger account with a funded balance.** A Tiger *simulated* account
   **will not work** — it fails authentication against the OpenAPI gateway. If
   you only have a simulated account, stop here; this project cannot help you
   test against it.
2. **OpenAPI access enabled** on that account by Tiger, with your app approved.
3. **Credentials generated from the Tiger OpenAPI developer portal**:
   <https://quant.itigerup.com/openapi/en/>

   | Credential | What it is | Where it comes from |
   |---|---|---|
   | **Developer ID** (App Key) | Your numeric developer id | Portal → My OpenAPI → App management |
   | **RSA Private Key** | PEM private key used to sign every request | Portal → App management → download private key |
   | **Trading account** | e.g. `DU1234567` | Your Tiger account / statement |
   | **App Secret** | *Institutional accounts only* | Portal → App management |

   Leave `TIGER_SECRET_KEY` empty for a normal retail account.

### 🔒 This project has NOT been validated against the live Tiger API

**The rule these three bullets exist to obey is normative in
[`openspec/specs/verification-honesty/`](openspec/specs/verification-honesty/spec.md):**
an artifact describes what was observed and what was tested, a rejected request
proves only that a request was built, signed, delivered and refused, and a
record of past runs is not a statement about the present. The accounting itself
is kept here in full, because it is the evidence the rule is about:

- **Proven:** the code compiles, `go vet` is clean, `gofmt` is clean, unit
  tests pass (14 packages, 730 cases — see [Test suite](#test-suite)), all eight
  binaries build, `-h` works without credentials, missing credentials produce a
  precise actionable error, and the dry-run gate provably blocks order writes.
  The configuration loader, redaction, and the request-building path are
  exercised by tests. `scripts/verify` runs all of that plus the SDK coverage
  check.
- **Also proven — historically, and not re-runnable:** the HTTP path reaches
  Tiger's *real* production gateway and returns a *real* API error
  (`code=1000 common param error(tigerId … is illegal)`) with deliberately fake
  credentials. The push client likewise opened a real TLS connection to Tiger's
  push server. Every `-op` of the four read-only data commands was run this way
  at the time: 77 invocations, each building a request, signing it, sending it
  and decoding a genuine API error response. That proved the wiring, the flag
  parsing and the request construction — and nothing more. It is a record of a
  past run, not a current state: it needs a valid RSA key to reproduce, and
  `cmd/quote -op addon-entitlement` is **not** part of those 77. Do not read the
  count as covering today's endpoint list.
- **NOT proven:** no request has ever been made with valid credentials. Field
  names, order-state values, k-line periods, option expiry/strike encoding,
  corporate-action type strings, financial `-fields` values and push callback
  payloads are taken from the SDK's own types and documentation, not from a live
  account. Expect to fix small things on first contact with the real API. In
  particular, the scanner filter JSON, the financial field names and the
  corporate-action type strings are Tiger-side contracts this project can only
  pass through.

Note the shape of that last bullet: it is a list of Tiger-side contracts this
project passes through unchanged, which is precisely the class of thing a green
test suite cannot speak for. `openspec/specs/sdk-coverage/` records the same
distinction on the coverage side — a method is covered when it is *referenced*,
not when it has been *exercised*.

Use small orders, and check your positions, on your first live run.

---

## Safety model

Because there is no sim mode, the write path is defended twice. **Both** gates
must be satisfied before a single byte reaches Tiger, and neither alone is
enough. The normative statement of that — including what a refusal does to the
method call — is
[`openspec/specs/write-gate/`](openspec/specs/write-gate/spec.md). At a glance:

| Gate | Default | How to satisfy it |
|---|---|---|
| `TIGER_DRY_RUN` | `true` | Set `TIGER_DRY_RUN=false` |
| `--confirm-live` flag | absent | Pass `--confirm-live` on the command line |

If either is missing, the command prints the exact request it **would** have
sent and exits **3** without contacting Tiger:

```
$ go run ./cmd/trade -command place -symbol AAPL -quantity 1 -limit-price 100

--- WOULD HAVE SENT ---
api:   place_order
payload: {
  "account": "DU0000000",
  "action": "BUY",
  "order_type": "LMT",
  "total_quantity": 1,
  "limit_price": 100,
  "time_in_force": "DAY",
  "symbol": "AAPL",
  "sec_type": "STK",
  "market": "US",
  "currency": "USD"
}
--- END (nothing was sent) ---

error: REFUSED: dry run is enabled (TIGER_DRY_RUN=true, the default).
No order was sent to Tiger. Set TIGER_DRY_RUN=false AND pass --confirm-live to submit one.
```

All three write paths — `place`, `modify`, `cancel` — funnel through the single
function `config.Writable()` in `internal/config/config.go`. There is no second
route to a write.

The 29 other `trade` commands are reads and ignore the gate. `preview` is the
easy case to reason about: it asks Tiger to validate an order without placing
it, so it is a read with respect to the book even though it costs a round trip.
The other 28 are ordinary queries — account state, order history, contracts,
funds, transfers, option-exercise previews — and none of them can mutate
anything.

That separation is **structural, not conventional**. `cmd/trade` is split across
two files: `main.go` holds the flag set, the gate and the three write paths;
`reads.go` holds the read-only endpoints. No function in `reads.go` takes a
`*config.Config`, so the value that owns the gate is never passed in and the
gate is not reachable from that file. Adding a write there would require
changing a handler signature, which is exactly the review signal you want.
`cmd/trade/dispatch_test.go` pins this down — see
[Tests](#tests-for-the-write-gate).

The seven read-only commands (`quote`, `options`, `futures`, `reference`,
`corporate`, `push`, `token`) do not participate in the gate at all, and that is
deliberate rather than an oversight. Four of them — `options`, `futures`,
`reference`, `corporate` — build their SDK client through `internal/rocli`,
which only ever calls `Session.Quote()`. `cmd/quote` skips `rocli` too and
builds its own session with `tigersdk.NewSession`, then calls `Session.Quote()`
directly.

`cmd/push` is the odd one out and does **not** go through `rocli`: it never
imports it. It builds its client by calling `tigersdk.Push`, which returns a
`*sdkpush.PushClient` value from the SDK's own `push` package — constructed by
`sdkpush.NewPushClient` against the same re-asserted `*sdkconfig.ClientConfig`
every other client is built from, so the credential defence is identical. What
makes it safe is not `rocli`: it is that `cmd/push` does not import the SDK's
`trade` package at all, and its `pushClient` interface exposes only connection
setup, subscribes and unsubscribes. A test parses the package's own source and
fails if `openapi-go-sdk/trade` or any of `PlaceOrder`, `ModifyOrder`,
`CancelOrder`, `PreviewOrder`, `NewTradeClient` or `TradeClient` appears in an
import or an identifier.

`cmd/token` is the same shape as `cmd/push` and for the same reason: it
imports no trade package, and the `tokenClient` and `subClient` interfaces it
declares expose only the token and local-subscription methods. The same source
check covers it. Note that `-set` on that command deliberately does bypass one
defence — the clearing of the SDK's discovered token file — which is an
authentication choice an operator makes on purpose, not an order-write path, and
which is stated on stderr before the value is used.

So there is no trade client in any of their processes, and no flag combination
can make them write. The gate logic itself is unchanged by the read-only
commands.

`cmd/trade` also prints a warning banner whenever dry-run is disabled:

```
WARN  DRY RUN IS DISABLED — orders submitted from here are REAL and will be filled.
```

**Keep `TIGER_DRY_RUN=true` unless you are deliberately placing a real order.**

---

## Setup

Requires Go 1.24+.

```bash
cd ~/github/tiger-go-demo
go build ./...
```

`go` may not be on `PATH` on this machine. If `go: command not found`:

```bash
export PATH=$PATH:/usr/local/go/bin
```

### Credentials — pick one method

**Option A: environment variables (recommended for a quick start)**

```bash
cp .env.example .env
$EDITOR .env          # fill in TIGER_ID, TIGER_PRIVATE_KEY, TIGER_ACCOUNT
set -a; . ./.env; set +a
```

For the private key, it is safer to keep it in a file than in your shell
history:

```bash
chmod 600 ~/.config/tiger/private_key.pem
export TIGER_PRIVATE_KEY_FILE=~/.config/tiger/private_key.pem
```

**Option B: YAML file**

```bash
cp config.example.yaml config.local.yaml
$EDITOR config.local.yaml
go run ./cmd/quote -config config.local.yaml
```

**Precedence: environment variables > YAML file > built-in defaults.**

A YAML file that was explicitly requested but is missing is a hard error, not a
silent fallback — you never accidentally trade the wrong account. Unknown YAML
keys are also errors, so typos surface immediately.

### Verify your credentials

```bash
go run ./cmd/quote -symbols AAPL
```

**Exit statuses are normative in
[`openspec/specs/exit-codes/`](openspec/specs/exit-codes/spec.md):** `0`
success, `1` an ordinary failure, `2` missing credentials, `3` a safety refusal,
with no two meanings sharing a status. Two consequences worth knowing up front,
because they are the ones that are easy to mis-script:

- **`2` means missing credentials and nothing else.** An unusable flag, an
  unparseable value, an unknown `-op` and an unreadable named config file are
  all `1`. A script that reads `2` knows only that the environment needs fixing.
- **Only `cmd/trade` can ever exit `3`.** The `quote` and `push` binaries
  contain no branch that produces it, because neither has a write path to
  refuse. The shared read-only plumbing in `internal/rocli` *does* define `3`,
  so the status is reachable in the code but not in those two commands' output.

With no credentials you get a precise, actionable error and exit code 2 —
reproduced here on the built binary with all five `TIGER_*` variables unset:

```console
$ env -u TIGER_ID -u TIGER_PRIVATE_KEY -u TIGER_PRIVATE_KEY_FILE \
      -u TIGER_ACCOUNT -u TIGER_SECRET_KEY ./bin/quote -symbols AAPL
error: incomplete Tiger OpenAPI configuration

Missing required setting(s):
  - TIGER_ID  (developer id / App Key)
  - TIGER_PRIVATE_KEY or TIGER_PRIVATE_KEY_FILE  (RSA private key)

Set them in any of these ways:
  export TIGER_ID=...            # or put them in a .env / shell profile
  cp config.example.yaml config.local.yaml   # then edit it
  go run ./cmd/quote --config config.local.yaml

Where each credential comes from (https://quant.itigerup.com/openapi/en/):
  TIGER_ID            Developer ID (App Key)   -> portal: My OpenAPI / App management
  TIGER_PRIVATE_KEY   RSA private key (PEM)    -> portal: App management -> Download private key
  TIGER_ACCOUNT       Funded trading account   -> your Tiger statement / portal account page
  TIGER_SECRET_KEY    App Secret, institutional accounts only -> portal: App management

Note: a Tiger SIMULATED account will NOT authenticate against OpenAPI.
You need a real, funded account with OpenAPI access enabled.

exit=2
```

Note what that message does *not* ask for: `TIGER_ACCOUNT` is not listed, because
`cmd/quote` reads no account state. A trading command with the same environment
does ask for it, and also exits `2`.

`--help` works with no credentials at all and exits `0`.

---

## Running the commands

### `quote` — market data (read-only)

```bash
# Real-time briefs
go run ./cmd/quote -symbols AAPL,MSFT

# Historical daily bars
go run ./cmd/quote -symbols AAPL -klines -period day -limit 10

# Intraday timeline
go run ./cmd/quote -symbols AAPL -timeline

# Order-book depth
go run ./cmd/quote -symbols AAPL -depth -market US

# Market state
go run ./cmd/quote -market-state US

# Addon-plan entitlement: the plan in force and the quota it leaves behind
go run ./cmd/quote -op addon-entitlement

# Combine
go run ./cmd/quote -symbols AAPL,0700.HK -klines -depth -market HK -v
```

K-line periods: `day`, `week`, `month`, `year`, `1m`, `5m`, `15m`, `30m`, `60m`.

`-op` selects a single endpoint instead of the default flow. `quote` has one:
`addon-entitlement` (`QuoteClient.GetAddonEntitlement`, wire method
`addon_entitlements`, a genuine read). It follows the same shape as the `-op` of
the other four read-only data commands: one `ops` slice in `cmd/quote/main.go`
feeds the flag help, the usage text and the dispatcher, so the three cannot
drift apart. Three tests hold that together — the usage text mentions every op
in `ops`, no op is listed twice, and every op in `ops` actually reaches a
printer rather than falling through to the "unknown `-op`" error. With no `-op`,
`quote` runs the default flow: briefs for `-symbols` plus whatever extras the
boolean flags ask for.

### What `addon-entitlement` prints, and what a `0` there does not mean

```text
== addon entitlement ==
  user_level  pro
  active_plan plan_type=standard expire=2026-01-01 00:00:00
  addons (1)
    PLAN_TYPE            ACTIVE  START                EXPIRE
    market_data          true    2025-12-01 00:00:00  2026-01-01 00:00:00
  effective_entitlement
    QUOTA                       LIMIT  REMAINING
    history_stock                 100         40
    history_future                 50         50
    history_option                 20          5
    subscribe                      30         12
    subscribe_depth                10          7
    high_freq_limit               200
    mid_freq_limit                100
    low_freq_limit                 20
    rate_multiple                   4
```

(That block is real output from `printEntitlement` against a hand-built
`model.AddonEntitlement`. It is *not* a live response — see the "NOT proven"
section above. The last four have no limit/remaining split in the SDK's model,
so only one column is printed for them.)

The output distinguishes absent from zero exactly as far as the SDK's types
allow, and no further:

- `EffectiveEntitlement` and `ActivePlan` are **pointers**. `nil` means the
  server sent no such block, and prints `(absent)` / `(none)`.
- The **fourteen quota fields** inside `effective_entitlement` are plain `int`
  with `omitempty`. A field the server omits arrives as `0`, and **the type
  gives no way to tell that apart from a real zero** — `{}` and
  `{"historyStockLimit":0}` decode to the same struct. So an omitted field and a
  genuine measurement of zero both print as `0`. `cmd/quote/output_test.go`
  pins this against the real `encoding/json` decoder rather than against
  hand-built structs, and the demo does not pretend otherwise: a `0` in that
  table means "the type could not tell us", not "you have nothing left".

### `options` — option market data (read-only)

Option identifiers are `UNDERLYING YYMMDDC|P STRIKE`, e.g. `"AAPL 250117C00200000"`
(AAPL call, 2025-01-17, strike 200). Endpoints that take a bare identifier accept
`-ids`; the rest are driven from the same flag and converted internally.

```bash
# Which expiries are listed for an underlying
go run ./cmd/options -op expiration -symbols AAPL,TSLA

# Option chain. With no -expiry the nearest listed expiry is used.
go run ./cmd/options -op chain -symbols AAPL -expiry 2025-01-17
go run ./cmd/options -op chain -symbols AAPL -greeks -itm in

# Real-time quotes, k-lines, depth, ticks, timeline
go run ./cmd/options -op quote    -ids "AAPL 250117C00200000,AAPL 250117P00200000"
go run ./cmd/options -op kline    -ids "AAPL 250117C00200000" -period day -limit 20
go run ./cmd/options -op kline-plain -ids "AAPL 250117C00200000" -period day -limit 20
go run ./cmd/options -op depth    -ids "AAPL 250117C00200000"
go run ./cmd/options -op ticks    -ids "AAPL 250117C00200000" -limit 5
go run ./cmd/options -op timeline -ids "AAPL 250117C00200000"

# Symbol list and implied-volatility analytics
go run ./cmd/options -op symbols  -market US -limit 20
go run ./cmd/options -op analysis -symbols AAPL -volatility-list
```

`-greeks` adds delta/gamma/theta/vega/rho and implied vol to the chain. `-itm`
filters to `in`, `out` or `all`.

### `futures` — futures market data (read-only)

```bash
# Contract metadata
go run ./cmd/futures -op exchange
go run ./cmd/futures -op current         -code CL
go run ./cmd/futures -op contract        -code CLmain
go run ./cmd/futures -op contracts       -exchange COMEX
go run ./cmd/futures -op all-contracts   -exchange COMEX -type ALL
go run ./cmd/futures -op continuous      -type ALL
go run ./cmd/futures -op trading-times   -code CLmain
go run ./cmd/futures -op history-main    -codes CLmain

# Market data
go run ./cmd/futures -op quote      -codes CLmain,ESmain
go run ./cmd/futures -op kline      -codes CLmain -period day -limit 10
go run ./cmd/futures -op kline-page -code CLmain -period day -page-size 50
go run ./cmd/futures -op depth      -codes CLmain
go run ./cmd/futures -op ticks      -code CLmain -limit 5
```

`-op current` resolves the front month of a product; `-op contract` looks one
contract up by its code. `-op kline-page` walks the paged endpoint and returns a
flat bar list, which is what you want for a series longer than one page.

### `reference` — reference and fundamental data (read-only)

```bash
# Symbol reference
go run ./cmd/reference -op symbols        -market US -limit 20
go run ./cmd/reference -op symbol-names   -market HK -limit 20
go run ./cmd/reference -op stock-details  -symbols AAPL,MSFT
go run ./cmd/reference -op stock-industry -symbols AAPL
go run ./cmd/reference -op stock-broker   -symbols AAPL

# Fundamentals
go run ./cmd/reference -op stock-fundamental  -symbols AAPL
go run ./cmd/reference -op financial-daily     -symbols AAPL -fields revenue,eps
go run ./cmd/reference -op financial-report    -symbols AAPL -fields revenue -period-type annual
go run ./cmd/reference -op financial-currency  -symbols AAPL
go run ./cmd/reference -op exchange-rate       -currencies USD,HKD
go run ./cmd/reference -op short-interest      -symbols AAPL,TSLA

# Calendars and screens
go run ./cmd/reference -op calendar -market US -begin-date 2025-01-01 -end-date 2025-01-31
go run ./cmd/reference -op scanner   -market US -page 1 -page-size 10
go run ./cmd/reference -op scanner   -market US -base-filters '[{"field":"market_cap","min":10000000000}]'
go run ./cmd/reference -op scanner-tags    -market US
go run ./cmd/reference -op industry-list   -industry-level 1
go run ./cmd/reference -op industry-stocks -industry-id 1001

# Other market data shapes and Tiger extras
go run ./cmd/reference -op ticks           -symbols AAPL -limit 5
go run ./cmd/reference -op timeline        -symbols AAPL
go run ./cmd/reference -op delayed         -symbols AAPL
go run ./cmd/reference -op kline-page      -symbols AAPL -period day -page-size 50
go run ./cmd/reference -op overnight       -symbols AAPL
go run ./cmd/reference -op trade-metas     -symbols AAPL
go run ./cmd/reference -op kline-quota
go run ./cmd/reference -op quote-permission
go run ./cmd/reference -op trade-metas -symbols AAPL
go run ./cmd/reference -op trade-rank -market US
go run ./cmd/reference -op timeline-history -symbols AAPL
```

`stock-fundamental` and `scanner-tags` return server-defined payloads and are
printed as JSON rather than forced into invented columns. Scanner filters are
open-ended server-side structures, so they arrive as JSON on the command line and
are validated locally before the request is sent.

Which `-fields` values the financial endpoints accept is a **Tiger-side
contract**. The ones in the examples come from the SDK's documentation, not from
a live response.

### `corporate` — corporate actions and instrument reference (read-only)

```bash
# Corporate actions
go run ./cmd/corporate -op dividend      -symbols AAPL -market US
go run ./cmd/corporate -op split         -symbols AAPL -market US
go run ./cmd/corporate -op earnings      -symbols AAPL -market US
go run ./cmd/corporate -op actions       -symbols AAPL -market US -action-type dividend
go run ./cmd/corporate -op ipo           -symbols BABA -market US
go run ./cmd/corporate -op symbol-change -symbols AAPL -market US
go run ./cmd/corporate -op delisting     -symbols AAPL -market US

# Capital flow
go run ./cmd/corporate -op capital-flow         -symbol AAPL -market US -period 5d
go run ./cmd/corporate -op capital-distribution -symbol AAPL -market US

# Hong Kong warrants and mutual funds
go run ./cmd/corporate -op warrant-filter -underlying 700 -market HK
go run ./cmd/corporate -op warrant-quote  -symbols 12345.HK
go run ./cmd/corporate -op fund-symbols
go run ./cmd/corporate -op fund-contracts -symbols 00001.HK
go run ./cmd/corporate -op fund-quote     -symbols 00001.HK
go run ./cmd/corporate -op fund-history   -symbols 00001.HK -limit 20
```

`-op actions` is the combined query and requires `-action-type`; the other
corporate-action endpoints set the type themselves.

### Shared flags

The four data commands (`options`, `futures`, `reference`, `corporate`) share
these, alongside their own:

| Flag | Default | Description |
|---|---|---|
| `-op` | varies | Which endpoint to call (listed in each command's `-h`) |
| `-market` | `US` | Market filter: `US`, `HK`, `SG`, `AU` |
| `-lang` | `en_US` | Response language: `en_US`, `zh_CN`, `zh_TW` |
| `-sec-type` | `STK` | Security type: `STK`, `OPT`, `FUT` |
| `-limit` | `20` | Max rows printed **and** requested |
| `-page` / `-page-size` | `0` | Paging for the paged endpoints |
| `-begin` / `-end` | `0` | Time range in epoch milliseconds (`0` = server default) |
| `-config` | — | YAML config path (also `$TIGER_CONFIG`) |
| `-v` | — | Debug logging |

`-limit` caps both what is requested and what is printed, and a truncation note
tells you when rows were dropped, so a broad query never looks complete.
`-h` works with no credentials and exits 0.

`cmd/quote` is the odd one out: it has its own boolean flags (`-klines`,
`-timeline`, `-depth`, `-market-state`) and its own `-limit` default of 5, but it
now also has `-op`, with a single value (`addon-entitlement`). The flag name and
the `ops`-slice shape are the same as the four above; the list is just shorter.

### `trade` — account, orders and 3 gated writes

`-command` selects one of **32** operations: **29 reads** that ignore the write
gate, and **3 writes** (`place`, `modify`, `cancel`) that cannot run without it.

| Group | `-command` values |
|---|---|
| account | `assets`, `positions`, `prime-assets`, `aggregate-assets`, `analytics-asset`, `managed-accounts` |
| orders | `orders`, `active-orders`, `inactive-orders`, `filled-orders`, `get-order`, `order-transactions`, `preview` |
| contracts | `contract`, `contract3`, `contracts`, `quote-contract`, `derivative-contracts` |
| estimates | `estimate-tradable-quantity` |
| funds | `segment-fund-available`, `segment-fund-history`, `fund-details`, `funding-history` |
| transfers | `position-transfer-records`, `position-transfer-detail`, `position-transfer-external-records` |
| options | `option-exercise-check`, `option-exercise-positions`, `option-exercise-records` |
| **writes** | `place`, `modify`, `cancel` — gated, see [Safety model](#safety-model) |

Every read is in `cmd/trade/reads.go` and takes a `tradeClient` and an `options`
value but **not** a `*config.Config`, which is what makes "a read cannot reach
the write gate" a property of the code rather than a promise in a comment.

```bash
# Read-only queries (no gate needed)
go run ./cmd/trade -command assets
go run ./cmd/trade -command positions
go run ./cmd/trade -command orders -limit 20
go run ./cmd/trade -command active-orders -states NEW,HELD
go run ./cmd/trade -command filled-orders
go run ./cmd/trade -command get-order -order-id 123456
go run ./cmd/trade -command order-transactions -symbol AAPL

# Aggregated balances and P&L
go run ./cmd/trade -command prime-assets
go run ./cmd/trade -command aggregate-assets -base-currency USD
go run ./cmd/trade -command analytics-asset -since-date 2025-01-01
go run ./cmd/trade -command managed-accounts

# Contract lookups. The derivative ones need -sec-type OPT/WAR/IOPT and an expiry.
go run ./cmd/trade -command contract -symbol AAPL
go run ./cmd/trade -command contract3 -symbol AAPL
go run ./cmd/trade -command contracts -symbols AAPL,MSFT
go run ./cmd/trade -command quote-contract -symbol AAPL -sec-type OPT -expiry 20260619
go run ./cmd/trade -command derivative-contracts -symbols AAPL -sec-type OPT

# Sizing: what could this account actually trade right now?
go run ./cmd/trade -command estimate-tradable-quantity -symbol AAPL -action BUY

# Funds, including the read side of the segment-fund transfer endpoints
go run ./cmd/trade -command segment-fund-available
go run ./cmd/trade -command segment-fund-history
go run ./cmd/trade -command fund-details -limit 10
go run ./cmd/trade -command funding-history

# Position-transfer records (the transfer itself is a write, and is absent)
go run ./cmd/trade -command position-transfer-records
go run ./cmd/trade -command position-transfer-detail -transfer-id t1
go run ./cmd/trade -command position-transfer-external-records

# Option exercise: the pre-flight check and the history, but not submit/cancel
go run ./cmd/trade -command option-exercise-positions -type Exercise
go run ./cmd/trade -command option-exercise-check -contract-id 123456
go run ./cmd/trade -command option-exercise-records

# Validate an order with Tiger without placing it (allowed in dry-run)
go run ./cmd/trade -command preview -symbol AAPL -quantity 1 -limit-price 100

# See what WOULD be sent (default dry-run)
go run ./cmd/trade -command place -symbol AAPL -quantity 1 -limit-price 100
go run ./cmd/trade -command modify -order-id 12345 -symbol AAPL -quantity 1 -limit-price 105
go run ./cmd/trade -command cancel -order-id 12345

# ACTUALLY submit — both gates satisfied. THIS SENDS A REAL ORDER.
TIGER_DRY_RUN=false go run ./cmd/trade -command place \
    -symbol AAPL -quantity 1 -limit-price 100 --confirm-live

TIGER_DRY_RUN=false go run ./cmd/trade -command cancel -order-id 12345 --confirm-live
```

Order types: `LMT`, `MKT`, `STP`, `STP_LMT`. Time in force: `DAY`, `GTC`, `OPG`.

**Deliberately not exposed.** Six `TradeClient` methods mutate the account and
are therefore not wired up at all: `PlaceForexOrder`, `TransferSegmentFund`,
`CancelSegmentFund`, `TransferPosition`, `OptionExerciseSubmit` and
`OptionExerciseCancel`. `SetSecretKey` is a local struct-field assignment with
no HTTP call at all, so there is nothing to gate and nothing to route. The
*read* side of five of those six operations is available above
(`segment-fund-available` and `segment-fund-history` sit on the same
`SegmentFundRequest` shape as the transfer and cancel; `funding-history` shares
the `transfer_fund` endpoint with `TransferSegmentFund`;
`position-transfer-records` is the read side of `TransferPosition`; and
`option-exercise-check` / `option-exercise-records` are the read side of the
exercise submits). The sixth, `PlaceForexOrder`, has no read counterpart in the
SDK.

### `push` — real-time feed (read-only)

```bash
# Quotes for 30 seconds (default)
go run ./cmd/push -symbols AAPL,MSFT -subscribe quote

# Trade ticks for 2 minutes
go run ./cmd/push -symbols AAPL -subscribe tick -duration 2m

# Account feed: order, position and asset changes
go run ./cmd/push -subscribe account -account -duration 5m

# Several feeds at once
go run ./cmd/push -symbols AAPL -subscribe quote,depth -v

# Option quotes. OCC symbols carry padding spaces, so quote the whole argument
go run ./cmd/push -subscribe option -symbols "AAPL  260619C00200000" -duration 1m

# Whole-market quote stream — HK only, and no symbols
go run ./cmd/push -subscribe market -market HK -duration 2m

# US stock ranking, two indicators
go run ./cmd/push -subscribe stock_top -market US -indicators changeRate,volume

# Crypto. The symbol form is undocumented, so pass what you have and read the
# delivery report if nothing arrives
go run ./cmd/push -subscribe crypto -symbols BTC -duration 1m

# Unsubscribing. Needs at least a minute of runtime, and refuses a shorter one
go run ./cmd/push -subscribe quote -unsubscribe -duration 2m
```

`Ctrl-C` exits cleanly. `-h` works with no credentials.

#### Feeds

`-subscribe` takes a comma-separated list. Values are case-folded, trimmed,
de-duplicated and kept in the order given; a blank segment is dropped, and a
list that selects nothing is an error that names the valid set.

| Feed | SDK method | What arrives |
|---|---|---|
| `quote` | `SubscribeQuote` | `[QUOTE]` |
| `tick` | `SubscribeTick` | `[TICK]`, one line per tick |
| `depth` | `SubscribeDepth` | `[DEPTH]`, up to five levels a side |
| `option` | `SubscribeOption` | `[OPTION]` |
| `future` | `SubscribeFuture` | `[FUTURE]` |
| `kline` | `SubscribeKline` | `[KLINE]`, one minute bar |
| `cc` (alias `crypto`) | `SubscribeCc` | `[QUOTE]` — the payload is the same as a quote's |
| `market` | `SubscribeMarket` | `[QUOTE]` — the whole HK market |
| `stock_top` | `SubscribeStockTop` | `[STOPTOP]`, grouped by indicator |
| `option_top` | `SubscribeOptionTop` | `[OPTTOP]`, ranked items and big orders |
| `account` | `SubscribeOrder` + `SubscribePosition` + `SubscribeAsset` | `[ORDER]`, `[POS]`, `[ASSET]` |
| `transaction` | `SubscribeTransaction` | `[FILL]` |

Two entries are not one method each, and both are deliberate:

- **`account` is a composite of three subscribes**, and it also pulls in
  `transaction`, because a fill is only interpretable next to the order that
  fills. So `-subscribe account` and `-subscribe account,transaction` are the
  same request, and the `-account` flag means exactly the same thing. The three
  account subscribes are sent with an empty account string, which means "the
  account from your config" — the account never travels on the wire from this
  command.
- **`crypto` is an alias for `cc`**, which is the SDK's own subject-type name.
  `crypto` is the word people reach for, so it is accepted; `cc` is what the
  dictionary stores.

`market` is a whole-market *quote* stream, not market status. The wire sends
`dataType=Quote` with the market field set and no symbols, so its payloads come
back through the same callback as a per-symbol quote and print as `[QUOTE]`.
The SDK's docstring calls this feed "market status"; the wire is unambiguous and
the command follows the wire, so nothing is labelled "market status" anywhere.

`cc` and `market` are also why the delivery counts can over-count — see below.

#### Flags

| Flag | Default | Description |
|---|---|---|
| `-subscribe` | `quote` | Comma-separated feeds; the help lists the accepted spellings |
| `-symbols` | `AAPL,MSFT` | Comma-separated symbols, upper-cased. Split on commas only, so OCC padding survives |
| `-account` | off | Subscribe the account feeds; implies `-subscribe account` |
| `-market` | empty | `HK` for `market`; `US`\|`HK` for `stock_top`; `US` for `option_top`. Required by those three, and refused for any other market |
| `-indicators` | `volume,amount` | For `stock_top` / `option_top`. Case-folded, so `CHANGERATE` reaches the server as `changeRate` |
| `-unsubscribe` | **off** | Unsubscribe before disconnecting — see the cooldown below |
| `-duration` | `30s` | How long to listen; `0` means until `Ctrl-C` |
| `-config` | — | YAML config path (also `$TIGER_CONFIG`) |
| `-v` | — | Debug logging |

`stock_top` accepts `changeRate`, `changeRate5Min`, `turnoverRate`, `amount`,
`volume`, `amplitude`; `option_top` accepts `bigOrder`, `volume`, `amount`,
`openInt`. The two sets overlap only on `volume` and `amount`, which is why
those two are the default — one default is then valid for both feeds. An
indicator from the wrong feed is refused with a message naming the right set.
That catches the mistake most likely to be made here: the SDK's own tests
(`push/coverage_extra_test.go`) subscribe with `top_gainer`, `top_loser`,
`top_volume` and `top_oi`, so those are the values a reader copying out of the
SDK source will reach for, and they are not in the sets this command accepts.

**Those two indicator lists are not verified against the live server.** They
come from Tiger's published indicator names, not from the SDK — the SDK's own
docstrings for the two ranking subscribes take a list of strings and enumerate
nothing — and this project has never run them against Tiger. If a ranking feed
delivers nothing, the delivery report is where that would show up.

Every flag is validated **before** the config is loaded and before any
connection is made, so a typo reports itself as a typo rather than as a missing
credential.

#### Delivery accounting, and why a subscribe that returns nil proves nothing

**The Go SDK exposes no subscribe-acknowledgement path.** A nil error from any
`Subscribe*` means only that the frame was written to the socket. The protocol
has an acknowledgement, but the SDK discards it: `handleMessage` in the SDK's
push client acts only on `CONNECTED`, `HEARTBEAT`, `MESSAGE`, `ERROR` and
`DISCONNECT`. A subscription the server refuses therefore produces neither an
error nor a callback — it simply never delivers, and the only evidence is that
nothing arrived.

So this command counts arrivals in the callbacks and reports the verdict at
shutdown, per feed. Counting starts *before* the first subscribe, because a
silent feed is the only signal a refusal leaves. The subscribe line says
`sent`, never `subscribed`:

```
15:04:05 INFO  subscribe sent: feed=quote symbols=[AAPL MSFT] market= indicators=[volume amount] (delivery is confirmed below, not by this line)
15:04:05 INFO  push connected
15:04:05 INFO  listening for 30s (Ctrl-C to stop early)
[QUOTE] AAPL       last=187.2500 bid=187.2400/100 ask=187.2600/200 vol=1234 status=Trading
```

and at the end:

```
15:04:35 INFO  delivery report: quote, depth
15:04:35 INFO    delivered: feed=quote messages=412
15:04:35 WARN    NO DATA: feed=depth — the server may have refused it (business code 3xxx), the per-data-type subscription quota may be exhausted (code 4), or the market may be closed
```

Payload lines go to **stdout**; log lines, including the delivery report, go to
**stderr**. So `go run ./cmd/push … > ticks.txt` keeps the data and leaves the
accounting on your terminal.

A `NO DATA` line is a warning, never an error: a closed market and a refused
subscription are indistinguishable from the client side, so the hint names all
three causes the SDK cannot tell apart — a refusal carrying a business code
(`3xxx`), a per-data-type subscription quota (code 4, "you can only subscribe to
xxx symbols"), and a closed market — with the feed-specific reason first,
because that is the one that is actually likely. A quiet `account` feed means
nothing happened on the account; a quiet `stock_top` means it is outside market
hours (and that US pre/post sessions push only `changeRate` and
`changeRate5Min`).

**The counts over-count on purpose.** The wire does not always say which
subscription a payload belongs to: `cc` and `market` both reuse `QuoteData` and
arrive through the quote callback, so one quote credits `quote`, `cc` and
`market` alike when all three are subscribed. The opposite error would raise a
`NO DATA` warning about a feed that is plainly delivering — and a warning
nobody can act on is worse than a count that is too generous.

#### Output shapes

The lines below are what the renderers emit, read off the format strings and
pinned by tests. **They are not a transcript from a live run** — nothing in this
repo has been run against Tiger with working credentials, so no sample here can
be. What is verifiable is the shape, and the shape is the same shape you would
get.

```
[QUOTE] AAPL       last=187.2500 bid=187.2400/100 ask=187.2600/200 vol=1234 status=Trading
[OPTION] AAPL      last=1.4250 bid=1.4200/50 ask=1.4300/60 vol=812 status=Trading
[FUTURE] ES        last=5480.2500 bid=5479.7500/3 ask=5480.5000/4 vol=145220 status=Trading
[TICK]  AAPL       sn=41822 price=187.2600 vol=100 type=+ cond=Regular venue=NASDAQ
```

`cc` and `market` print as `[QUOTE]` — the payload is identical and the wire
does not distinguish them. That is also why neither is ever labelled `[CC]` or
`[MARKET]`: there is no field in the payload that would make the label honest.

The book arrives as parallel slices rather than a list of structs, so it renders
as a block, truncated at five levels a side with the remainder counted:

```
[DEPTH] AAPL
  bid 7 level(s):
          187.2400  vol=100       orders=4
          187.2300  vol=250       orders=11
          ...
          ... 4 more level(s)
  ask 5 level(s):
          187.2600  vol=200       orders=8
```

A missing level count prints `orders=-` rather than `0`, because "the server
sent no count" and "the count is zero" are different facts.

```
[KLINE] AAPL       o=187.1000 h=187.3100 l=186.9800 c=187.2500 avg=187.1400 vol=48310 n=274 amt=9045120.75
```

Rankings are grouped by indicator, because the server groups the rows that way
and the name is a group header, not a per-row field. Option rows carry
symbol, expiry, strike and right in separate fields, reassembled into one cell
so a row can be matched against the symbol you subscribed to:

```
[STOPTOP] market=US indicator=changeRate rows=10
           NVDA       last=141.3200 value=8.4200
           ...
           ... 4 more row(s)
[OPTTOP] market=US indicator=volume rows=10 big_orders=2
          AAPL 20260619 200 CALL      vol=10.00 amt=2000.00 oi=500.00 vol/oi=0.0200
          AAPL 20260619 200 PUT       BUY  vol=1500.00 price=3.5000 amt=5250.00
```

The account shapes:

```
[ORDER] id=88123 AAPL       BUY LMT 100/200 @187.2500 status=FILLED
[ORDER] id=88124 AAPL       BUY LMT 0/100 @187.2500 status=REJECTED err=insufficient funds
[POS]   AAPL       qty=100 avg_cost=185.1200 mkt_value=18725.00 unreal_pnl=213.00
[ASSET] account=DU1234567 ccy=USD cash=125340.55 net_liq=402118.90 buying_power=201059.45
[FILL]  order_id=88123 AAPL       100 @187.2500
```

A server error message rides on the `[ORDER]` line as an `err=` suffix, and
appears on no other line because no other line has somewhere to put it.

#### The one-minute unsubscribe cooldown

The vendor requires at least a minute between the last subscribe and any
unsubscribe, and a run that unsubscribes at shutdown is always inside that
window unless it ran long enough. `-unsubscribe` is therefore **off by
default**, and the code refuses rather than hopes:

- `checkUnsubscribePlan` rejects `-unsubscribe -duration <1m` **before any
  connection is made**, with a message naming both flags. `-duration 0` is
  allowed, because a run until `Ctrl-C` cannot be judged up front.
- `sendUnsubscribe` re-checks the elapsed time at shutdown. If the run ended
  inside the cooldown, **nothing is sent** and the refusal is logged loudly:

  ```
  15:04:35 WARN  not unsubscribing: only 29.997s since the last subscribe, and Tiger rejects an unsubscribe inside the 1m0s cooldown (nothing was sent; the connection is simply closing)
  ```

  If the cooldown has passed, the unsubscribes are sent, in the same order as
  the subscribes, and always before the disconnect — after the socket closes
  there is nothing to write to.

The unsubscribes mirror the API's asymmetry rather than smoothing it over: the
seven market-data ones take the symbols to drop, the two ranking ones take
`(market, indicators)`, and the four account ones —
`UnsubscribeOrder`, `UnsubscribePosition`, `UnsubscribeAsset`,
`UnsubscribeTransaction` — take **no arguments at all**, because the API can
only drop the whole subject, never one part of it.

The alternative to refusing was to send the request anyway and warn that the
server will probably ignore it. That was rejected: a rejected unsubscribe
leaves this connection's subscriptions exactly where they were, Tiger allows
one push connection per Tiger ID, and a second connection is a way to make
things worse. Silence plus a warning reads as "it worked"; an explicit "not
sent, and here is why" cannot.

#### Two vendor-side things the code does not smooth over

- **The crypto symbol format is undocumented and Tiger's own documents
  disagree** — `BTC`, `BTC.USD`, `BTCUSD` and `BTC/USD` all appear. The command
  does not guess and does not rewrite: the symbols go out exactly as supplied,
  and if nothing arrives the `cc` hint names the four forms to try. Crypto
  subscription is therefore **unverified** — this repo has never confirmed which
  form, if any, the server accepts.
- **One push connection per Tiger ID.** A second connection kicks the first,
  which arrives as a kickout callback, and the command logs that as an error
  because it is the one event an operator has to act on.

### `token` — bearer token and local subscription state (read-only)

```bash
# Ask the gateway for a new token. In memory only; no file is written.
go run ./cmd/token -refresh

# Put a token you already hold into this process. In memory only.
go run ./cmd/token -set "$TOKEN" -refresh

# Print this process's LOCAL record of account subscriptions
go run ./cmd/token -show
```

Three flags, all off by default, so a bare invocation is a usage error rather
than a silent no-op. The order they run in is fixed and the order is the point:

- `-set` is applied first, then `-refresh`, then `-show`. A refresh is a request
  authenticated with the token currently in hand, so `-set` seeds that token and
  `-refresh` then rotates it. The reverse order would make `-set` a no-op,
  because the refresh would overwrite it immediately. **The value you passed to
  `-set` is not the token in effect when the process exits** — the gateway's is,
  and the output says so at the point where it stops being true.
- `-show` reads the push client's own in-memory map. It opens no connection and
  sends nothing, so **the server is not asked**. The banner says LOCAL, and says
  the server was not asked, before any list is printed — an empty list with no
  context is exactly the output that reads like "the account has no
  subscriptions". This command never subscribes, so the list is empty by
  construction; `cmd/push` is where subscriptions are made.

**`-set` is the one place the token-file defence is deliberately bypassed, and
the cost is printed on stderr before the value is used.**
`internal/tigersdk.NewClientConfig` clears `ClientConfig.Token` after the SDK
builds it, because the SDK copies that field into the `Authorization` header of
every request. That clearing still runs for every command in this project, this
one included — `-set` does not undo it and does not route around it. It is a
separate input, applied after construction, to a different field, in this one
process, only when a human typed it. The value came from a command line, so it is
in that user's shell history and readable by any other user on the machine
through `ps`. Only its **length** is ever printed, through the same redaction the
private key and the app secret get; the value is not.

Two inputs are refused rather than applied: `-set` with an empty value (which
would silently *clear* the token rather than set one — often a shell expanding an
unset variable), and `-set` with a value containing `PRIVATE KEY`, which is the
shape of a credential that must never reach a command line. If your token starts
with `-`, write `-set=TOKEN`.

**`-refresh` is unverified.** It asks the real gateway for a new token, and this
project has never made a request with valid credentials — so its behaviour is
established only by "it compiles, it is wired, and the error path is tested
against a fake". It is called with a nil token manager on purpose: that is what
keeps the new token in memory. Passing a manager would write it to a file, which
is the one thing this project refuses to do behind an operator's back. The token
is gone when the process exits, and there is nowhere to persist it.

---

## Configuration reference

| Env var | YAML key | Required | Default | Description |
|---|---|---|---|---|
| `TIGER_ID` | `tiger_id` | **yes** | — | Developer ID / App Key |
| `TIGER_PRIVATE_KEY` | `private_key` | **yes** | — | RSA private key (PEM body) |
| `TIGER_PRIVATE_KEY_FILE` | — | — | — | Path to the PEM file (alternative) |
| `TIGER_ACCOUNT` | `account` | for trading | — | Trading account number |
| `TIGER_SECRET_KEY` | `secret_key` | institutional only | — | App Secret |
| `TIGER_LICENSE` | `license` | no | `TBNZ` | `TBNZ`/`TBSG`/`TBAU`/`TBIH`/`TBHK` |
| `TIGER_LANGUAGE` | `language` | no | `en_US` | `zh_CN`/`zh_TW`/`en_US` |
| `TIGER_TIMEZONE` | `timezone` | no | `US/Eastern` | Response timestamp timezone |
| `TIGER_TIMEOUT` | `timeout` | no | `15s` | HTTP timeout (Go duration) |
| `TIGER_DEVICE_ID` | `device_id` | no | auto MAC | Device identifier |
| `TIGER_SERVER_URL` | `server_url` | no | production gateway | Trade/account/common endpoint |
| `TIGER_QUOTE_SERVER_URL` | `quote_server_url` | no | = `server_url` | Quote endpoint. **Now honoured** — see below |
| `TIGER_PUSH_URL` | `push_url` | no | `openapi.tigerfintech.com:8887` | Push server |
| `TIGER_DRY_RUN` | `dry_run` | no | **`true`** | Order-write kill switch |
| `TIGER_CONFIG` | — | no | — | Path to the YAML config |
| `TIGER_LOG_LEVEL` | `log_level` | no | `info` | `debug`/`info`/`warn`/`error` |

### `quote_server_url` was inert, and is not any more

An earlier version of this README listed `quote_server_url` as the quote
endpoint without qualification, which was **not true**. The field was parsed,
stored, passed to the SDK config and re-asserted like every other field, and had
no effect whatsoever: `Session.Quote()` handed `NewQuoteClient` the session's
*single* `*HttpClient`, and the SDK substitutes `QuoteServerURL` for `ServerURL`
in exactly one place — inside `NewQuoteHttpClient` — so every market-data request
was posted to `server_url`.

The consequence was not cosmetic. Anyone who set `quote_server_url` to keep
market data off production — a sandbox, a recording proxy, a second gateway —
would have had every quote request go to the **production** gateway instead,
silently. That was verified with two loopback listeners on the old code: with
`server_url` and `quote_server_url` pointing at different ports, the quote
request arrived on the main port.

It is now honoured, and the fix is structural rather than a reordering:

- The quote client is built through the SDK's `NewQuoteHttpClient`, which is the
  only place `QuoteServerURL` is substituted, with an explicit fallback to
  `server_url` when it is unset. So a `Config` built by hand — a test, a library
  caller — cannot produce an ambiguous endpoint either.
- The quote client **shares token storage** with the trade client
  (`WithSharedTokenFrom`), so a token can never differ between the two. That is
  also why no second token-refresh goroutine is started.
- `Session` now holds **two** HTTP clients, and `Close` releases both. They are
  two closes of two objects, not two closes of one: `NewQuoteHttpClient` builds a
  fresh client, so closing the trade gateway's client says nothing about the
  quote gateway's. A test gives each client a token-refresh goroutine and counts
  them, so "neither leaks" is observed rather than assumed.
- `TestQuoteServerURLIsHonoured` swaps the SDK's quote-client constructor for a
  recorder, because `HttpClient` holds its config in an unexported field and
  exports no accessor. That invisibility is precisely how a knob that does
  nothing survives a change: the endpoint quote traffic will actually use is
  otherwise unobservable. A second test asserts that pointing the quote gateway
  somewhere does not move the trade gateway.

---

## Security notes

- **Secrets are never logged.** `private_key` and `secret_key` are rendered as
  `<redacted:N bytes>` everywhere, including via `fmt.Stringer`, so an
  accidental `fmt.Printf("%v", cfg)` cannot leak one. Covered by tests. The
  rule, and the fact that the *string* form of the configuration is the redacted
  one, are normative in
  [`openspec/specs/secret-redaction/`](openspec/specs/secret-redaction/spec.md).
- **`secret_key` is omitted, never empty.** Tiger rejects an empty
  `secret_key` in `biz_content` with `biz_param_error(1010)`. This project
  leaves the field unset unless you actually configured one — an unset app
  secret is left *out* of the request, not sent as `""`.
- **The SDK's own `TIGEROPEN_*` env vars are deliberately unused**, so this
  project's loader stays the single source of truth. Note that "unused" is
  enforced, not assumed: see the stray-input table below.

### The complete set of inputs the SDK honours, and what this project does with them

**The normative statement is
[`openspec/specs/credential-defence/`](openspec/specs/credential-defence/spec.md).**
The table is the evidence, and it is kept whole because the last three rows are the
ones a reader should be able to check for themselves:

The SDK accepts credentials and endpoints from five places this project does not
configure: three files and two groups of environment variables. **All five are
neutralised.** Four of the five are also **named in a warning** — the three files,
plus the file `$TIGEROPEN_TOKEN_FILE` points at. `$TIGEROPEN_TOKEN` and the
credential variables are not. That asymmetry is deliberate, and it is narrower
than it was: the one input that is file-shaped but not a file in a scanned
directory has joined the files, and the reason it did is stated below rather than
papered over.

| Input | Neutralised? | Warned? | Notes |
|---|---|---|---|
| `./tiger_openapi_config.properties` | yes | **yes** | Re-asserted after the SDK builds its config. Long-standing. |
| `~/.tigeropen/tiger_openapi_config.properties` | yes | **yes** (new) | Found from *any* working directory. Was defended but silent. |
| `./tiger_openapi_token.properties` | yes (new) | **yes** (new) | The significant gap. See below. |
| `$TIGEROPEN_TOKEN` | yes (tested) | no | A value, not a path. `warnStrayFile` prints a path, so it could never name one. |
| `$TIGEROPEN_TOKEN_FILE` | yes (tested) | **yes** (new) | File-shaped, and it can name a path anywhere on disk — outside both the working directory and the home directory. |
| `$TIGEROPEN_TIGER_ID` and friends | yes | no | Same treatment as `$TIGEROPEN_TOKEN`: values, not paths. |

`internal/tigersdk.NewClientConfig` re-asserts **every** field of the
`ClientConfig` after the SDK has built it, so a stray file cannot redirect your
orders, your gateway or your account — including into fields this project
legitimately leaves empty, which is exactly the ones a file gets to fill.

**`~/.tigeropen/tiger_openapi_config.properties` was defended but silent.** A
user could plant a file there, run a command from any directory, watch it have no
effect, and get no explanation. The values were never wrong; the *feedback* was.
It is now named in the warning like the working-directory copy.

**`./tiger_openapi_token.properties` was the significant gap.** The SDK reads it
*independently* of the config file — a different file, found by a different
mechanism, feeding a different field — and `client/http_client.go:381` copies
its contents into the `Authorization` header of **every** request. So a file that
has nothing to do with your credentials could authenticate your session as a
different account. `sc.Token` is now cleared explicitly, and the file is named
in the warning. The `Authorization` half was proved on the wire, not just in the
config: a loopback listener saw `Authorization: STRAY-TOKEN-ABC` from the raw
SDK built with no token option at all, and an empty header from a session this
project built with the same file sitting in the working directory.

**`$TIGEROPEN_TOKEN` is neutralised but silent; `$TIGEROPEN_TOKEN_FILE` is now
named.** Both are covered by tests and both lose to the same `sc.Token` clearing,
but only one of them is reported. The rest of the asymmetry is intact and still
deliberate: an environment variable is something you can see in your own shell,
while a file you may not know exists is not — so the warning budget goes to
files.

`$TIGEROPEN_TOKEN_FILE` is the carve-out, and it earns the exception twice over.
It is **file-shaped** — what it names is a file, and a file the SDK would read a
token from. And it **escapes both directory scans**: the working directory and
`~/.tigeropen` are the only places the three file checks look, and this variable
can name a path anywhere on disk. "We warn you about files" would otherwise be
an untrue rule rather than a narrow one, and this is the input that breaks it.
Its payload is also the highest-consequence of the four: the bearer token the SDK
copies into the `Authorization` header of every request.

It is printed **last** anyway, because it is the least likely of the four — it
takes a deliberate export, where the three files are things somebody left lying
around. Being last costs a line of output; not warning at all would have cost a
silent token source.

What is still silent is `$TIGEROPEN_TOKEN` and the credential variables. That
remains a recorded gap rather than an oversight, and it is recorded here instead
of left for someone to discover.

`WarnStrayProperties` keeps the message format it always had — name the file,
say it is being ignored, explain the consequence, say to delete it. The body was
factored so each new detection reuses the same text. It runs **four** detections
now — three files, then the environment variable — and four limits are worth
knowing:

- It **degrades silently** when `HOME` is unset or unreadable (a bare container,
  a cron job with no login shell). No home directory means the file cannot be
  there, so this is not an error, but it is a detection that did not run.
- **One broken lookup disables one detection, not all of them.** A missing
  `HOME` still leaves the working-directory and token-file warnings intact, and
  that is asserted by a test.
- **The fourth check is existence-based**, like the other three. A
  `$TIGEROPEN_TOKEN_FILE` naming a file that is not there produces nothing, and an
  empty or whitespace-only value is silent too. There is no discovered file to
  report in either case, and an empty value is not even a redirection — the SDK
  falls back to its default token file name, which is the third check.
- **Its path is echoed verbatim, not normalised.** `sub/../token.properties` comes
  back as `sub/../token.properties`, deliberately: the SDK hands the raw
  environment value to its token manager, so the raw string is the one it would
  actually open, and the only form you can match against your own shell history or
  `.env`. The cost is a value whose `$HOME` was never expanded: the existence
  check then fails and nothing is named, even though the variable is set. The
  benefit is that this is the one warning whose path is a real, actionable path as
  typed, where the working-directory ones are not (below).

Every `WarnStrayProperties` call site passes `"."`, so the working-directory
name in the warning is the bare filename, not `./tiger_…`. The absolute form
appears when an absolute directory is passed. Adequate for a human reading a
terminal; not a path a script could delete from. `$TIGEROPEN_TOKEN_FILE` is the
one exception, and by construction: its value is never joined onto a directory,
so what the warning reads back is exactly what you exported.

- **`.gitignore` covers `.env`, `*.properties`, `config.local.yaml`,
  `config.yaml`, `*.pem` and `*.key`.** Never commit real credentials.

---

## Project layout

```
tiger-go-demo/
├── cmd/
│   ├── quote/
│   │   ├── main.go         flags, -op dispatch, the default flow
│   │   ├── output.go       table formatting, addon-entitlement rendering
│   │   └── output_test.go  absent-vs-zero rendering, decoded against real JSON
│   ├── options/            option market data (read-only)
│   │   ├── main.go         flags + endpoint dispatch
│   │   ├── requests.go     option-identifier parsing
│   │   └── requests_test.go
│   ├── futures/            futures market data (read-only)
│   ├── reference/          reference + fundamental data (read-only)
│   ├── corporate/          corporate actions, warrants, funds (read-only)
│   ├── trade/
│   │   ├── main.go         flag set, write gate, the 3 write paths
│   │   ├── reads.go        the 29 read-only endpoints
│   │   └── dispatch_test.go  reads bypass the gate; writes do not
│   └── push/
│       ├── main.go         flags, the 12 feeds / 13 spellings, dispatch, delivery accounting
│       ├── output.go       the per-payload renderers
│       └── main_test.go    vocabulary-vs-dispatch drift, cooldown policy, output shapes
│   └── token/
│       ├── main.go         flags, the fixed action order, -set / -refresh / -show
│       ├── output.go       the renderers
│       └── main_test.go    resolve, ordering, and "never print the value"
├── internal/
│   ├── config/             env + YAML loader, validation, redaction, write gate
│   │   └── config_test.go
│   ├── logging/            leveled logger implementing the SDK logger interface
│   │   └── logging_test.go
│   ├── rocli/              shared read-only plumbing: session, output, exit codes
│   │   ├── rocli.go
│   │   ├── output.go
│   │   └── rocli_test.go
│   ├── sdkcoverage/        the SDK coverage check: scan, allow-list, reason derivation
│   │   ├── scan.go         the SDK's client surface, and this repo's call sites
│   │   ├── allowlist.go    the 19 exclusions, and the closed reason vocabulary
│   │   ├── check.go        runs both halves and reports
│   │   ├── verify.go       derives the reasons that are claims about the SDK
│   │   └── *_test.go       unit tests, control tests, mutation-checked
│   └── tigersdk/           SDK client construction; defeats every SDK-discovered input
│       ├── tigersdk.go
│       └── tigersdk_test.go
├── test/
│   ├── readonly_test.go    read-only classification, both directions, plus a control test
│   └── coverage_test.go    the SDK coverage enforcement point
├── openspec/
│   └── specs/              the normative requirements; see "Specification layer"
├── docs/
│   └── superpowers/        the design and plan for the verification work
├── scripts/
│   └── verify              the gate: gofmt, vet, test, coverage-check
├── .env.example
├── .gitattributes         LF for all text — gofmt rejects CRLF
├── config.example.yaml
├── Makefile                a convenience layer over scripts/verify
└── README.md
```

`internal/rocli` is what keeps the four read-only data commands small: it owns
flag registration, credential loading, session setup, table formatting and the
exit-code convention, so each command file is just a list of endpoints. It
exposes no trade client, which is why those commands cannot place an order.
`cmd/quote`, `cmd/push` and `cmd/token` do not use it — `cmd/quote` builds its own
session, and `cmd/push` and `cmd/token` build a push client directly; see
[Safety model](#safety-model) for why none of them can write.

`internal/sdkcoverage` holds the check's logic so it is unit-testable, and
`test/coverage_test.go` is the enforcement point. The split is deliberate: the
check's scan roots are `cmd/` and `internal/`, so a check living in `test/` is
outside its own scan. A checker that could see itself would be able to satisfy the
check it performs.

---

## SDK coverage

**The normative statement is
[`openspec/specs/sdk-coverage/`](openspec/specs/sdk-coverage/spec.md):** the
current figure, that every remaining gap is excluded for a stated structural
reason rather than forgotten, and that a rejected request is not a mutation in
this API. This section is the evidence for all three.

#### How the check works, and where it lives

The check is Go, in [`internal/sdkcoverage`](internal/sdkcoverage), and it is
enforced by `test/coverage_test.go`. Three properties of that arrangement are
load-bearing, and each is silent when broken:

- **It walks declarations, not a hardcoded directory list.** `ClientMethods`
  parses the module cache and matches `FuncDecl` whose receiver *type* ends in
  `Client`. A client type the SDK ships later is therefore counted automatically.
  It matches the type and never the receiver variable's name: the previous
  implementation was a `grep` for a literal `(c \*`, and pinning a receiver name
  means that the day the SDK renames one receiver, that whole type drops out of
  the denominator — a smaller, greener number, and no failure.
- **`push/pb` is excluded structurally.** That package declares 32 receiver types
  over 43 declared types, 278 of them field accessors, and none ends in
  `Client` — the string does not appear in the package at all. A walk keyed on
  client-typed declarations cannot reach it. The old `--exclude-dir=pb` flag was
  honest documentation rather than a guard, and there is no flag here to drop.
- **The check is outside its own scan.** The scan roots are `cmd/` and
  `internal/`. A checker living in `test/` cannot see itself, so it cannot
  satisfy the check it performs. That is structural, not a comment about an
  exclusion list.

Call sites are matched on `*ast.CallExpr` whose callee is a selector, not on
text. The previous implementation was `grep -E "\.Name("`, which cannot tell a
call from a comment or a string literal — both of which would report a method as
referenced when nothing in the program calls it. The switch is
behaviour-preserving and that was measured rather than assumed: all 174 textual
hits under `cmd/` and `internal/` were re-examined with literals stripped and
comment boundaries respected, and every one was a real call. What it removes is
the possibility of losing one later.

**The `internal` reason is derived, not asserted.** It is the only reason that
makes a claim about the vendor rather than about this project, so it is the only
one `verify.go` can check — and it does, against the module source. A call site
in a package the SDK ships for its callers to import (`client`, `quote`, `trade`,
`push`, `config`, `model`, `signer`, `logger`) earns it; a call site in `cmd/`,
`examples/`, `integtest/` or `scripts/` does not, because those are programs
shipped for a human to run and nothing in the library's own call graph depends
on them. Relabelling a method `internal` without a qualifying call site now fails
the build instead of becoming a comment nobody re-reads.

**Two methods are covered only from a test file.** `GetSubscriptions` and `State`
are referenced by `internal/tigersdk/tigersdk_test.go` and by nothing else. Test
files are in scope for the call-site scan by decision: excluding them would move
the published figure from 140/159 to 138/159, and a change to a published number
belongs in this README with a reason, not in a silent scope change. What it
means is narrower than it may look — those two are exercised in a test, so their
shapes are known, but neither is called by any command.

The SDK ships **four** client types, and the check covers all of them: **159**
exported methods — **78** on `QuoteClient`, **39** on `TradeClient`, **34** on
`PushClient`, **8** on `HttpClient`. This project references **140** of them,
leaving **19** uncovered, and `make coverage-check` fails the build if that set
is not *exactly* these nineteen, so the number cannot drift quietly:

```console
$ go test -count=1 -run TestSDKCoverage -v ./test/
sdk coverage: 140/159 methods covered, 19 uncovered
uncovered set matches the allow-list:
  CancelSegmentFund:mutating
  Execute:internal
  ExecuteRaw:not-used
  GetBars:deprecated
  GetBarsByPage:deprecated
  GetBrief:deprecated
  GetOptionBrief:deprecated
  GetStockDelayBriefs:deprecated
  GetWarrantBriefs:deprecated
  GrabQuotePermission:mutating
  OptionExerciseCancel:mutating
  OptionExerciseSubmit:mutating
  PlaceForexOrder:mutating
  QueryToken:internal
  SecretKey:internal
  SetSecretKey:not-a-call
  StartTokenAutoRefresh:internal
  TransferPosition:mutating
  TransferSegmentFund:mutating
```

| Client type | Total | Covered | Uncovered |
|---|---|---|---|
| `QuoteClient` | 78 | 71 | 7 |
| `TradeClient` | 39 | 32 | 7 |
| `PushClient` | 34 | 34 | 0 |
| `HttpClient` | 8 | 3 | 5 |
| **All four** | **159** | **140** | **19** |

Read the `HttpClient` row with the caveat in
[the `Close` false positive](#the-close-false-positive) below: one of the three
counted as covered there is not evidence of anything.

#### History — the earlier 103 / 117 figure

**This section is history, not the current claim.** Before the check was widened
from two client types to four, the denominator was the `QuoteClient` +
`TradeClient` subset and the figure was **103 / 117 with 14 uncovered**. Those
fourteen are a strict subset of the nineteen above — the check did not lose
gaps when it widened, it gained five. Kept because the progression is the
evidence for how the number moved:

| | Initial commit | After the 4 data commands | After `cmd/trade/reads.go` | `-op addon-entitlement` | Widened to all four clients |
|---|---|---|---|---|---|
| Overall | 13 / 117 | 79 / 117 | 102 / 117 | 103 / 117 | **140 / 159** |
| `QuoteClient` (read-only) | 5 / 78 | 71 / 78 | 70 / 78 | 71 / 78 | **71 / 78** |
| `TradeClient` | 8 / 39 | 8 / 39 | 32 / 39 | 32 / 39 | **32 / 39** |
| `PushClient` | — | — | — | — | **34 / 34** |
| `HttpClient` | — | — | — | — | **3 / 8** |
| Uncovered | 104 | 38 | 15 | 14 | **19** |

(The first three columns are commits `0ebcbf4`, `83e12a6` and `7239660`. They
are re-derivable with the two-client loop in the git history of this section,
against a `git worktree` of the relevant commit — which is how the first two
were checked when the table was extended. The last column is a different
denominator, not a bigger version of the same one: 159 is not 117 plus 42, it
is 117 plus the 42 methods on `PushClient` and `HttpClient`, 8 of which this
project leaves uncovered.)

The old two-client derivation loop, kept because it is the one that produced
those historical columns:

```console
$ TG=$(go env GOMODCACHE)/github.com/tigerfintech/openapi-go-sdk@v0.5.2
$ for f in quote trade; do
    grep -hoE "^func \(c \*${f^}Client\) [A-Z][A-Za-z0-9]*" $TG/$f/*.go \
      | sed -E 's/.*\) //' | sort -u
  done > /tmp/methods          # 117 lines
$ wc -l < /tmp/methods
117
```

The current one adds the other two types and matches on the receiver pattern
rather than a hardcoded name — see [how the scan works](#how-the-scan-works)
for why that second part is load-bearing.

`QuoteClient` went **down** by one, not up, when `reads.go` landed.
`cmd/reference` used to call `GetStockDelayBriefs`, a deprecated alias, behind a
`-delay-mins` flag that existed only to select it. Both the call and the flag
are gone; `-op delayed` now calls the non-deprecated `GetDelayedQuote`. That is
a fix, not a regression, and it is the reason the quote count dropped while the
overall count rose by 24. `GetAddonEntitlement` then brought the quote count
back up to where it was, from the other direction.

### What the uncovered SDK methods are

**7 on `QuoteClient`:**

- **6 deprecated aliases** — `GetBrief`, `GetBars`, `GetBarsByPage`,
  `GetOptionBrief`, `GetWarrantBriefs` and `GetStockDelayBriefs`. Each carries a
  `// Deprecated:` line in `quote/quote_client.go` pointing at a replacement
  (`GetRealTimeQuote`, `GetKline`, `GetKlineByPage`, `GetOptionQuote`,
  `GetWarrantQuote`, `GetDelayedQuote`). The commands call the replacements.
  Every deprecated symbol reachable from this repo — those 6 methods plus
  `tigeropen.Version`, `model.MarketScannerTags`, `model.BarsRequest`,
  `model.BarsByPageRequest` and `OrderRequest.IsQuantityByAmount` — now has
  **zero** call sites, which is checkable:

  ```console
  $ for s in GetBrief GetBars GetBarsByPage GetOptionBrief GetWarrantBriefs \
             GetStockDelayBriefs tigeropen.Version MarketScannerTags \
             BarsRequest BarsByPageRequest IsQuantityByAmount; do
      printf '%-24s %s\n' "$s" "$(grep -rn "\b$s\b" --include=*.go . | wc -l)"
    done
  GetBrief                 0
  GetBars                  0
  GetBarsByPage            0
  GetOptionBrief           0
  GetWarrantBriefs         0
  GetStockDelayBriefs      0
  tigeropen.Version        0
  MarketScannerTags        0
  BarsRequest              0
  BarsByPageRequest        0
  IsQuantityByAmount       0
  ```

- **`GrabQuotePermission`** — *claims* a market-data permission, so it changes
  account state. It is not a read, and it is deliberately left out.

There is **no** open gap left on the quote client: every one of the 7 is either
a deprecated alias superseded by a method this project does call, or something
that mutates the account.

**7 on `TradeClient`:**

- **6 account-mutating methods** — `PlaceForexOrder`, `TransferSegmentFund`,
  `CancelSegmentFund`, `TransferPosition`, `OptionExerciseSubmit`,
  `OptionExerciseCancel`. Each would need the write gate in front of it, and
  that is exactly why none is wired up. Their *read* counterparts are covered
  where the SDK has any.
- **`SetSecretKey`** — not an API call at all. It is a single in-memory
  assignment (`c.secretKey = key`) on the client struct. There is no HTTP
  request, nothing to gate, and nothing to route.

So: of the 7 uncovered trade methods, **6 write and 1 is not a call at all**.
None of them is a read that was skipped for being awkward — the read side of
every one of those operations is now covered by `reads.go`.

**0 on `PushClient`:** all 34 are covered. `cmd/token` supplies the last one,
`GetAccountSubscriptions`.

**5 on `HttpClient`:** the interesting ones, because `HttpClient` is the SDK's
own transport rather than an endpoint client, and the reason vocabulary splits.

#### The reason vocabulary, and the `internal` vs `not-used` distinction

Every allow-list line carries exactly one reason word, and the two that are easy
to confuse are not interchangeable:

| Reason | What it asserts | On the list |
|---|---|---|
| `deprecated` | A non-deprecated replacement is what the commands call | 6 |
| `mutating` | Would change account state, so it needs the write gate | 7 |
| `not-a-call` | Not an API call at all — a local assignment | 1 |
| `not-used` | A user-facing escape hatch this repo has not reached yet | 1 |
| `internal` | **A call site exists inside the SDK's own library packages** | 4 |

**`internal` is a claim about the SDK, not about this repo.** It is the only
reason that asserts something the vendor's source has to support, so it is the
only one that can be checked against the module cache, and it is the reason it
is easy to get wrong: a caller inside the module is *not* automatically a
caller inside the library.

Only **four** methods earn it. Each has a call site in a package that ships as
library code:

| Method | Library call site in `openapi-go-sdk@v0.5.2` |
|---|---|
| `Execute` | `quote/quote_client.go` (×3), `client/http_client.go`, `trade/trade_client.go` (×2) |
| `QueryToken` | `client/http_client.go` (×2) |
| `SecretKey` | `trade/trade_client.go` |
| `StartTokenAutoRefresh` | `client/http_client.go` |

The other three methods on `HttpClient` are **`not-used`, not `internal`**, and
the distinction is the whole point:

- **`ExecuteRaw` has no caller anywhere in the SDK's own code** — zero call
  sites outside its own test files. It is a raw escape hatch — hand it a method
  name and a JSON body and it posts them — and no shipped package reaches for
  it. Check it yourself:

  ```console
  $ TG=$(go env GOMODCACHE)/github.com/tigerfintech/openapi-go-sdk@v0.5.2
  $ grep -rn "\.ExecuteRaw(" $TG --include=*.go | grep -v _test.go || echo "no non-test caller"
  no non-test caller
  $ grep -rn "func (c \*HttpClient) ExecuteRaw" $TG --include=*.go
  .../client/http_client.go:300:func (c *HttpClient) ExecuteRaw(apiMethod string, requestJSON string) (string, error) {
  ```

- **`RefreshToken` and `SetCurrentToken` are called only from example programs
  that ship inside the module.** `RefreshToken` has exactly one non-test caller,
  `examples/manual_test/integ_token_and_order.go`; `SetCurrentToken` has
  exactly one, `cmd/integ_token_refresh/main.go`. Both are programs the SDK's
  authors ship for a human to run — they are not code the library runs, and
  nothing in the library's own call graph depends on them.

  ```console
  $ grep -rn "\.RefreshToken("  $TG --include=*.go | grep -v _test.go
  .../examples/manual_test/integ_token_and_order.go:84:		if err := hc2.RefreshToken(nil); err != nil {
  $ grep -rn "\.SetCurrentToken(" $TG --include=*.go | grep -v _test.go
  .../cmd/integ_token_refresh/main.go:133:	primary3.SetCurrentToken(simulatedToken)
  ```

**So the SDK does not call any of `ExecuteRaw`, `RefreshToken` or
`SetCurrentToken`.** They are the same species of thing — a user-facing escape
hatch — and labelling any of them `internal` would assert something the SDK
source does not support.

`cmd/token` now calls all three, so they are covered rather than justified.
That is a statement about *this repo*, and it is worth being precise about what
it did and did not change: `internal` was never the right word for any of them
and still is not. The distinction mattered while they were uncovered, because an
uncovered method needs a reason and `internal` was available to misuse. A
covered method carries no allow-list line at all, so the question no longer
arises — but the fact about the SDK above is still the fact, and would still
settle it if they ever became uncovered again. `GetAccountSubscriptions` is the
same case with an even stronger claim behind it: it has no caller anywhere,
inside or outside the module.

#### The `Close` false positive

`HttpClient.Close` is the one method the check counts as covered, and **a green
run is not evidence that it was exercised.**

The matcher searches by **method name**, not by receiver type: for each method
name it asks whether `\.Name(` appears anywhere under `cmd/` or `internal/`.
There are **nine** `.Close(` sites in non-test code there, and only **two** of
them are the SDK's method:

```go
// internal/tigersdk/tigersdk.go — the two that are the SDK's
s.HTTP.Close()        // *HttpClient
s.QuoteHTTP.Close()   // *HttpClient
```

The other seven are our own `Session.Close` (`cmd/quote`, `cmd/trade`,
`internal/rocli`) and `env.Close` (`cmd/options`, `cmd/corporate`,
`cmd/futures`, `cmd/reference`) — different types, different methods, same
word. A green run therefore proves only that the string `Close(` appears
somewhere in the tree. It does **not** prove `HttpClient.Close` ran, and this
README does not claim it did.

The collision is harmless today, and only by luck: every other method on the
list is a `quote`, `trade` or `push` name, and none of our own helpers is called
`GetBrief`, `GetKline` or `SubscribeQuote`. It would not be harmless if a future
SDK method shared a name with a helper of ours — which is exactly why the
limitation is written down here instead of being left for someone to
rediscover. The rule this section follows: **`HttpClient.Close` is not claimed
as covered.**

#### How the scan works

The receiver pattern is **receiver-name-agnostic** — it matches
`\([a-zA-Z_][A-Za-z0-9_]* \*?[A-Za-z]+Client\)`, not a hardcoded `(c *`. Pinning
the receiver name is a latent bug with no failure attached: the day the SDK
renames one receiver, the pattern matches nothing for that type, the whole type
silently drops out of the denominator, and the run is simply smaller and
greener.

The optional `*?` is the same class of insurance, and worth being precise about
what it is worth: value receivers do exist elsewhere in the module (`model` and
`logger` have them), but **none of the four scanned client types declares one
today**, so the `*?` changes nothing in the current count. It is there so that
introducing one is a zero-change event rather than a silent 151-method shrink.

The glob is **flat** (`$base/$pkg/*.go`, not `-r`), which is what keeps the
generated Protobuf package out: `push/pb/` declares 436 exported methods over
32 receiver types, 278 of them field accessors and 87 of them
`Reset`/`String`/`ProtoReflect` — hundreds of call sites no SDK user would
write. A flat glob cannot reach `push/pb/` at all, so the exclusion is
structural rather than a flag that might be dropped. `--exclude-dir=pb` is
honest documentation of that intent, not the guard: making the glob recursive
*while keeping the flag* leaves the count at 159, and making it recursive
*while dropping the flag* also leaves it at 159, because no `pb` receiver type
ends in `Client` and the string `Client` never appears in `push/pb` at all. Do
not read a green run as evidence that the flag is doing the work.

`--exclude='*_test.go'` keeps the SDK's own tests out. Measured, none of them
declares a method on the four client types today, so the counts are identical
either way — the flag is insurance, not arithmetic. The SDK's
`push/coverage_extra_test.go` and `client/coverage_extra_test.go` are exactly the
kind of file that grows a helper method on the type under test, and a test
helper is not a call site this target is claiming coverage for.

#### History — the trade-client correction

An earlier version of this README claimed all 31 previously-uncovered trade
methods were account-mutating and would each need the write gate. That was
wrong. Exactly **24 of the 31 were plain reads**, and all 24 are now covered:

```
AggregateAssets      AnalyticsAsset       Contract             Contract3
Contracts            DerivativeContracts  EstimateTradableQuantity
FilledOrders         FundDetails          FundingHistory       GetOrder
InactiveOrders       ManagedAccounts      OptionExerciseCheck
OptionExercisePositions  OptionExerciseRecords  OrderTransactions
PositionTransferDetail   PositionTransferExternalRecords
PositionTransferRecords  PrimeAssets    QuoteContract
SegmentFundAvailable SegmentFundHistory
```

It also listed `GrantQuotePermission` as skipped — **no such method exists in
SDK v0.5.2**; only `GrabQuotePermission` does.

### How to read the coverage claim

This is a **static** check. It proves a method is *referenced from `cmd/` or
`internal/`*, not that it was *exercised against a live Tiger account*. Nothing
in this repo has run with valid credentials — see "This project has NOT been
validated against the live Tiger API" near the top of this README.

Two consequences worth stating separately, because they are different strengths
of claim:

- **The counts are a reference search.** `GetBars` gaining a call site changes a
  number; it does not mean a bar was ever fetched.
- **A name-based match can be the wrong method.** `HttpClient.Close` is the
  known instance, and it is documented above rather than quietly counted.

### There is no REST here, and no HTTP verbs

Tiger OpenAPI is **not** a REST API, and the SDK does not pretend otherwise.
Every `QuoteClient` and `TradeClient` method is a `POST` to a single gateway
URL with a JSON `"method"` field naming the operation:

```go
// client/http_client.go:373 in tigerfintech/openapi-go-sdk@v0.5.2
req, err := http.NewRequest("POST", c.config.ServerURL, strings.NewReader(string(jsonData)))
```

There is no URL path to get wrong, no `GET`/`POST`/`DELETE` distinction to
respect, and no 404-vs-200 signal. The endpoint is selected by a string —
`quote_real_time`, `quote_contract`, `addon_entitlements`, `place_order` — which
is why this README talks about *wire methods* rather than paths and verbs. The
only place the SDK uses a different verb at all is the push feed, which is a
TCP + TLS + Protobuf socket rather than HTTP.

**A `POST` is not a mutation**, and the coverage split above is drawn on "does
this change account state", not on the HTTP verb. `EstimateTradableQuantity`
costs a real round trip against the live account and sits with the reads
precisely because it cannot place anything.

---

## Development

**The gate is `scripts/verify`, and it is four commands.** The `make` targets are
a convenience layer over the same four, not the gate itself — so the checks run on
a machine with no GNU make installed, which is the point. A gate that only runs on
one contributor's machine is not a gate.

```bash
scripts/verify                 # the gate: gofmt, vet, test, coverage-check
TIGER_NO_RACE=1 scripts/verify # the same, without the race detector
```

**And it now runs on CI too**, which is what the line above was asking for.
[`.github/workflows/verify.yml`](.github/workflows/verify.yml) calls
`scripts/verify` itself rather than re-listing the four commands, so a check added
to the script is enforced automatically and the two cannot drift apart. It runs on
`ubuntu-latest`, `windows-latest` and `macos-latest`, on every push to `main` and
every pull request. The matrix is over operating systems rather than Go versions on
purpose: `go.mod` declares `go 1.24`, so an older runner would either refuse the
module or quietly download 1.24 and test the same thing repeatedly. The platforms
differ in the ways that have actually bitten this project — CRLF against `gofmt`
(what the `.gitattributes` `eol=lf` pin is for), and whether a C compiler is
present for `-race`.

Or the four commands directly, which is what the script runs:

```bash
gofmt -l .                                    # must print nothing
go vet ./...
go test -count=1 ./...
go test -count=1 -run TestSDKCoverage ./test/
```

Two environment details, both learned the hard way on Windows:

- **`-race` needs cgo, which needs a C compiler.** Go's Windows installer does not
  bundle one, so on a stock Windows box `go test -race` exits 2 with
  `-race requires cgo`. That is a *missing toolchain*, not a property of Windows:
  install MinGW-w64 and put it on `PATH` and the race detector works. Verified on
  this project's own machine, where a MinGW-w64 `gcc` sits in `~/mingw64/bin` and
  the whole suite passes clean under `-race` — including `cmd/push`, which is where
  the concurrency actually is (delivery tracker, cooldown policy). An earlier
  version of this file claimed Windows "provides neither [cgo nor a C toolchain] by
  default" and that claim was never tested; it was used as the reason to run every
  gate in this project with `TIGER_NO_RACE=1`. The race detector is the better
  default, so the script uses it unless `TIGER_NO_RACE=1`; that is the opt-out, not
  the rule. `make test-norace` is the same escape hatch, for machines with no C
  toolchain at all.
- **`gofmt` rejects CRLF**, and `core.autocrlf=true` — the Windows git default —
  turns every checked-out text file into one. `.gitattributes` now pins `eol=lf`
  for all text, which is why `gofmt -l .` prints nothing on a fresh clone. Without
  it, that check fails on all 32 Go files for a reason that has nothing to do with
  formatting.

`GO` defaults to `go` on `PATH`; override it if your install is elsewhere
(`make verify GO=/usr/local/go/bin/go`).

```bash
make help          # list targets
make verify        # fmt-check + vet + test -race + build + coverage-check
make build         # binaries into ./bin (all eight)
make test          # unit tests with the race detector
make test-norace   # the same, without it
make coverage-check  # the coverage assertion on its own

make run-quote     ARGS="-symbols AAPL"
make run-options   ARGS="-op expiration"
make run-futures   ARGS="-op exchange"
make run-reference ARGS="-op stock-details -symbols AAPL"
make run-corporate ARGS="-op dividend -symbols AAPL -market US"
make run-token     ARGS="-show"
```

`coverage-check` is the step worth knowing about. It does not merely *count*
coverage; it asserts the uncovered set is **exactly** the nineteen methods listed
above, so a new gap fails and so does a method that quietly gained a call site.
It is Go — `internal/sdkcoverage`, enforced by `test/coverage_test.go` — and it was
a 40-line `grep`/`sed`/`comm` pipeline until recently, which meant it could only
run where those exist.

```console
$ go test -count=1 -run TestSDKCoverage -v ./test/
=== RUN   TestSDKCoverage
    coverage_test.go:26:
        sdk coverage: 140/159 methods covered, 19 uncovered
        uncovered set matches the allow-list:
          CancelSegmentFund:mutating
          Execute:internal
          ExecuteRaw:not-used
          GetBars:deprecated
          GetBarsByPage:deprecated
          GetBrief:deprecated
          GetOptionBrief:deprecated
          GetStockDelayBriefs:deprecated
          GetWarrantBriefs:deprecated
          GrabQuotePermission:mutating
          OptionExerciseCancel:mutating
          OptionExerciseSubmit:mutating
          PlaceForexOrder:mutating
          QueryToken:internal
          SecretKey:internal
          SetSecretKey:not-a-call
          StartTokenAutoRefresh:internal
          TransferPosition:mutating
          TransferSegmentFund:mutating
--- PASS: TestSDKCoverage (0.15s)
PASS
ok  	github.com/shing1211/tiger-go-demo/test	0.531s
```

### Test suite

`go test ./...` puts **14** packages behind tests and passes. The rough case
count — every `=== RUN` and every subtest `--- PASS` line — is **730**, across
287 top-level test functions:

```console
$ go test -count=1 -v ./... 2>&1 | grep -cE '^(=== RUN|    --- PASS)'
730
$ go test -count=1 ./... | grep -c '^ok'
14
```

Per-package statement coverage:

| Package | Coverage | What it covers |
|---|---|---|
| `internal/logging` | **100.0%** | Level parsing, filtering, formatting, `Discard`, concurrency, SDK-noise silencing |
| `internal/tigersdk` | **~98.7%** | Every SDK-input defence, session construction, both gateways, `Close`, error rendering, push client |
| `cmd/push` | **86.0%** | The feed vocabulary against both dispatch switches, the flag defaults, the `-market` and `-indicators` validators, the cooldown policy, the delivery tracker, every renderer |
| `internal/config` | **81.4%** | Env/YAML precedence, validation, redaction, the write gate, stray-file lookups |
| `cmd/token` | **62.9%** | Flag resolution, the fixed action order, the nil token manager, and "the value is never printed" |
| `internal/sdkcoverage` | **61.7%** | The client-surface walk, the AST call-site match, the allow-list's shape, and the reason derivation — each with a control test |
| `cmd/trade` | **57.9%** | The write-gate dispatch table (the load-bearing part) |
| `internal/rocli` | **48.2%** | Exit codes, row formatting, truncation, and the five helpers consolidated from five commands |
| `cmd/options` | **48.8%** | Every renderer, plus the option-identifier parsing, the expiry conversion and the `-op chain` request validation. The residual is handler plumbing, which needs a live account |
| `cmd/quote` | **43.4%** | Every renderer, including entitlement, market state, briefs, k-lines, timeline and depth. The residual is handler plumbing |
| `cmd/reference` | **41.3%** | Every renderer, including the four-way scanner group dispatch. The residual is handler plumbing |
| `cmd/corporate` | **31.9%** | Every renderer. The residual is handler plumbing |
| `cmd/futures` | **31.7%** | Every renderer. The residual is handler plumbing |
| `test` | no statements | Holds only tests: the read-only classification and the coverage enforcement point |

```console
$ go test -cover ./...
ok  	cmd/corporate	coverage: 31.9% of statements
ok  	cmd/futures	coverage: 31.7% of statements
ok  	cmd/options	coverage: 44.7% of statements
ok  	cmd/push	coverage: 86.0% of statements
ok  	cmd/quote	coverage: 43.4% of statements
ok  	cmd/reference	coverage: 41.3% of statements
ok  	cmd/token	coverage: 62.9% of statements
ok  	cmd/trade	coverage: 57.9% of statements
ok  	internal/config	coverage: 81.4% of statements
ok  	internal/logging	coverage: 100.0% of statements
ok  	internal/rocli	coverage: 48.2% of statements
ok  	internal/sdkcoverage	coverage: 61.7% of statements
ok  	internal/tigersdk	coverage: 98.7% of statements
ok  	test	coverage: [no statements]
```

Four of those numbers need a caveat, because a percentage can mean more than it
does:

- **`cmd/push` at 86.0% tests the command's own decisions, not the wire.** The
  push client is reached through a `pushClient` interface that the test suite
  fakes, so every subscribe, the delivery accounting, the cooldown policy and
  every renderer are exercised hermetically — with no connection, no
  credentials and no network. What that cannot cover is the 14% that needs a
  live server: the callbacks the SDK invokes, and therefore the payload values
  the renderers format. The shapes are pinned; the values are not, because no
  run in this repo has produced any.
- **`cmd/quote` is now at 43.4%, and no renderer in the project holds an SDK
  client any more.** Its entitlement, market-state, brief, k-line, timeline and
  depth printers all take a `*sdkquote.QuoteClient`, a concrete SDK struct with no
  interface, no injectable constructor and no transport seam — so they used to be
  exercisable only against a live account, which this project does not have. All
  six have been split the same way as the rest: the handler keeps the SDK call and
  hands plain data to a `print*` renderer. `printAddonEntitlement` calls and
  `printEntitlement` renders was the first of these; the other five
  (`printMarketState`, `printBriefs`, `printKlines`, `printTimeline`,
  `printDepth`) were split later, each with a test. What is still uncovered across
  every data command is the handler half — argument validation, the SDK call, and
  error wrapping.
- **`cmd/quote`'s two display caps are literals, and both now say what they hide.**
  `printIntradayTimelines` and `printQuoteDepths` cap at 10 by literal, so
  `-limit` does not reach them — they are named constants
  (`timelinePointsShown`, `depthLevelsShown`) so the bound and the notice text
  cannot drift apart. `printQuoteDepths` used to stop *silently*, which was the
  one place in the project claiming a completeness it did not have; it now prints
  `... N more level(s) not shown`, worded for price/size pairs rather than
  reusing `rocli.Truncate`'s "row(s)". A book exactly at the cap prints no notice,
  and that boundary is pinned.
- **The four `cmd/quote` renderers that had no empty-result branch now have one.**
  They printed a heading and column header over nothing, on the reasoning that an
  empty table is self-evident. That was the only place relying on a reader
  noticing an empty table, and it is a weaker signal than the other 60 renderers
  give, so they now print `  (no rows returned)` and suppress the heading. This
  changed output for an empty response and was a deliberate decision, not a
  side-effect of the split.
- **`cmd/corporate` (31.9%), `cmd/futures` (31.7%), `cmd/reference` (41.3%) and
  `cmd/options` (48.8%) had the same blocker and the same fix.** Their handlers
  no longer render inline; each delegates to a tested `print*` renderer, which is
  what moved them off 2-5% and 16.3%. The pure validation half has been tested
  where it could be reached without a client — identifier parsing, the expiry
  conversion, the `-op chain` request builder. The residual is the handler
  plumbing that genuinely needs an account: the SDK call and its error wrapping.
- **`-limit 0` now means "no rows" in every command, and that was a behaviour
  change.** Two sites — `opExpiration` and `printChains` — used to guard with
  `if o.Limit > 0 && i >= o.Limit`, so `-limit 0` printed *every* row there while
  the bare `if i >= limit` sites printed none. This divergence used to be
  documented here rather than fixed. It is now resolved: both sites use the bare
  form, so no `Limit > 0 &&` guard remains anywhere in the tree. The cost is that
  `-limit 0` on `-op chain` prints headings and no rows, where it used to print
  the whole chain. `printChains`' behaviour under `-limit 0` is pinned by a test,
  and `truncation_guard_test.go` in each of the four packages now fails if any
  limit guard loses its `rocli.Truncate` call — the notice that says how many rows
  a truncated result hid, which is easy to drop silently because the rows are
  still capped either way.
- **`internal/sdkcoverage` at 61.7% is the number that matters least, and it is
  worth saying why.** It covers the walk, the AST match, the allow-list's shape
  and the reason derivation, each with a control test. The uncovered remainder is
  `Check`'s error plumbing, which is exercised by the same code path every time
  `test/coverage_test.go` runs — and that test *does* run on every `go test
  ./...`. The package's own tests were additionally checked by mutation: three
  deliberate breakages (treat every directory as library, include `_test.go`
  files, return nothing unconditionally) each fail the suite, so the tests are
  not passing for a reason nobody checked.
- **`internal/tigersdk` at ~98.7% is the meaningful number, because of the
  control tests.** Each defence in that package has a *companion* test that
  proves the hazard is actually reachable, by handing the raw SDK the same input
  and asserting it **does** get adopted:

  | Defence test | Control test |
  |---|---|
  | `TestNewClientConfigBeatsStrayPropertiesFile` | `TestStrayPropertiesFileIsActuallyReachable` |
  | `TestNewClientConfigBeatsHomePropertiesFile` | `TestHomeFileIsActuallyReachable` |
  | `TestNewClientConfigBeatsStrayTokenFile` | `TestStrayTokenFileIsActuallyReachable` |
  | `TestNewClientConfigBeatsStrayFileWhenFieldsAreUnset` | `TestNewClientConfigEmptyFieldsSurviveTheSDKWithoutAFile` |
  | `TestWarnStrayPropertiesNamesTheHomeFile` | `TestHomeFileIsActuallyReachable` |

  A defence test on its own proves nothing: the defence could be removed
  entirely and the test would still pass, if the hazard had quietly gone away
  upstream. The control test fails the moment the hazard stops being real — the
  SDK stops reading the file, and the defence test is then measuring nothing.
  That is the property that makes this suite worth having rather than a green
  tick, and it is why `TestSDKConfigErrorIsUnreachable` — one test that
  documents a branch as *provably* unreachable rather than leaving it silently
  untested — is worth as much as a covered one.

### Tests for the write gate

`cmd/trade/dispatch_test.go` is the test that makes the safety claim checkable
rather than merely asserted. It runs with dry-run **on** and `--confirm-live`
**off** — the configuration in which `config.Writable` refuses every write — and
asserts four things:

- **Every one of the 29 reads is in the table**, and the table's length is
  checked against `readCommands` at run time, so a command added to
  `readCommands` without a test fails the build rather than silently going
  untested.
- **Each read returns `nil`** against a fake client under that refused-write
  config. A read that has been through the gate could not do this.
- **Each read calls exactly the one SDK method it is supposed to.** The fake
  records the method name; a read that quietly grew a second SDK call fails.
- **Each of the 3 writes returns a `config.IsSafetyError` and the fake's
  `called` field is still empty** — i.e. the refusal happened before any SDK
  call, not after a failed one.

Two further tests pin the edges: an unknown `-command` names itself in the error
and is not classified as a credential or safety failure, and a read missing an
input it cannot fill in (`-symbol`, `-expiry`, `-transfer-id`, `-contract-id`,
a non-derivative `-sec-type`, …) refuses locally, naming the flag, without
reaching the SDK.

The structural half of the guarantee is not something a test can assert at
runtime — a function that took a `*config.Config` would still work. It is
enforced by the signatures: **no handler in `reads.go` accepts a
`*config.Config`**, so the gate's owner is never passed in and the gate is
unreachable from that file. The test proves the consequence; the signature is
the cause.

```console
$ go test -race -count=1 -v ./cmd/trade/
--- PASS: TestReadCommandsBypassTheGate (0.05s)      # 29 subtests, one per read
--- PASS: TestWritesAreRefusedByDefault (0.01s)      # 3 subtests, one per write
--- PASS: TestUnknownCommandIsRejected (0.00s)
--- PASS: TestMissingInputsAreRejectedLocally (0.01s) # 9 subtests
PASS
ok  	github.com/shing1211/tiger-go-demo/cmd/trade	1.162s
```

(The four `--- PASS` lines and the `ok` line are verbatim output; the `# N
subtests` notes are counts, `grep -cE '^    --- PASS'` per test.)

---

## Specification layer

**`openspec/specs/` holds the normative requirements. This README holds the
evidence and the instructions.** Where the two used to say the same thing, the
spec now owns the rule and the README keeps the transcript, the control-test
table and the caveat — because a rule with no recorded observation behind it is
a claim, and a claim is what this project is trying not to make.

The layer is validated, so a spec that drifts out of shape fails rather than
lingering:

```console
$ openspec validate --specs --strict
- Validating...
✓ spec/credential-defence
✓ spec/exit-codes
✓ spec/sdk-coverage
✓ spec/secret-redaction
✓ spec/verification-honesty
✓ spec/write-gate
Totals: 6 passed, 0 failed (6 items)
```

Seven capabilities, 42 requirements:

| Spec | What it owns |
|---|---|
| `write-gate` | the two independent conditions that must both hold before a byte is sent, and that a refusal stops the method call |
| `command-classification` | that every command is classified read-only or write in **both** directions, that exactly one command writes, that a read-only command cannot reach an order write or produce a refusal, and that the classification is deliberate rather than inherited |
| `exit-codes` | the four statuses, the two classifications that are easiest to get wrong, and the seven read-only binaries that can never produce a refusal |
| `credential-defence` | all five SDK-discovered credential inputs neutralised, and the deliberate asymmetry in what is warned about |
| `secret-redaction` | the exact form a secret takes in any printable rendering, and that an unset app secret is omitted rather than sent empty |
| `sdk-coverage` | the coverage figure over all four client types, that every gap is a stated exclusion, the meaning of each exclusion reason, the known name-collision false positive, and that the split is drawn on account state rather than the HTTP verb |
| `verification-honesty` | that a rejected request proves a request was built, signed, delivered and refused — that a record of past runs is not a statement about the present, that a nil from a subscribe is not an acceptance, that nothing here is verified against live data, and that an undocumented vendor format is passed through and flagged rather than guessed |

Read a spec when you want to know **what must be true**. Read this README when
you want to know **how it behaves, how to run it, and how far it has been
checked**.

---

## Troubleshooting

**`error: incomplete Tiger OpenAPI configuration` (exit 2)**
Credentials are missing. The message names every missing variable and the
portal page it comes from. See the example above.

**`code=1000 common param error(tigerId … is illegal)`**
The developer id is wrong, not yet approved for OpenAPI, or the licence is
mismatched. Confirm `TIGER_ID` and `TIGER_LICENSE` in the portal. A simulated
account also fails here.

**`签名失败 … 不是有效的 PEM 或 Base64 格式` (signature failed, not valid PEM)**
The private key is malformed. Check it begins with `-----BEGIN RSA PRIVATE KEY-----`
or `-----BEGIN PRIVATE KEY-----` and that line breaks survived your copy/paste.
Using `TIGER_PRIVATE_KEY_FILE` avoids most whitespace damage.

**`biz_param_error(1010)` on an order**
Usually an empty `secret_key` in the payload. If you are a retail account, make
sure `TIGER_SECRET_KEY` is unset rather than set to `""`.

**`error: REFUSED: dry run is enabled` (exit 3)**
Working as designed. Pass `--confirm-live` **and** set `TIGER_DRY_RUN=false`.

**`warning: found <path> … being IGNORED`**
A file the SDK would otherwise have discovered on its own —
`./tiger_openapi_config.properties`, `~/.tigeropen/tiger_openapi_config.properties`,
`./tiger_openapi_token.properties`, or whatever `$TIGEROPEN_TOKEN_FILE` points at.
It is being ignored on purpose, and the path in the warning is the one to delete.
If you expected its values to be in effect, they were not: this project loads
credentials from env/YAML only. If the last one is the surprise, `unset
TIGEROPEN_TOKEN_FILE` is the fix — the variable belongs to the SDK, and this
project reads it for nothing except the warning above.

**Push: `等待 CONNECTED 响应超时` (timed out waiting for CONNECTED)**
The TLS connection opened but authentication was rejected — same credential
problems as above. It is not a firewall issue if you got this far.

**Changes to your env vars seem to do nothing**
Confirm the variable is actually exported in the shell running `go run`, and
remember YAML is lower precedence than env.

---

## Links

- Tiger OpenAPI docs: <https://docs.itigerup.com/docs/> (EN: <https://docs-en.itigerup.com/docs/>)
- Developer portal: <https://quant.itigerup.com/openapi/en/>
- Go SDK: <https://github.com/tigerfintech/openapi-go-sdk> (MIT)
- pkg.go.dev: <https://pkg.go.dev/github.com/tigerfintech/openapi-go-sdk>

## License

This demo is provided as-is for educational use. The SDK is MIT-licensed.
**You are responsible for any orders you submit.**
