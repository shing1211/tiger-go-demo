# tiger-go-demo

A self-contained, safe-by-default Go demo of the **Tiger Brokers OpenAPI**,
built on the official SDK [`github.com/tigerfintech/openapi-go-sdk`](https://github.com/tigerfintech/openapi-go-sdk) v0.5.2.

Three commands, plus four read-only data commands:

| Command | What it does | Writes orders? |
|---|---|---|
| `cmd/quote` | Real-time briefs, historical bars, intraday timeline, market state, order-book depth | **No** — read-only |
| `cmd/options` | Option expiries, chains (plain and with Greeks), quotes, k-lines, depth, ticks, timeline, symbols, implied vol | **No** — read-only |
| `cmd/futures` | Contract metadata (exchanges, current/all/continuous contracts, trading times) plus quotes, k-lines, depth, ticks | **No** — read-only |
| `cmd/reference` | Symbol lists and names, stock details, fundamentals, financial series, FX, short interest, trading calendar, market scanner, industries | **No** — read-only |
| `cmd/corporate` | Dividends, splits, earnings calendar, IPOs, symbol changes, delistings, capital flow, HK warrants, fund NAVs | **No** — read-only |
| `cmd/trade` | 29 read commands — account state, orders, contracts, funds, transfers, option exercise — plus place / modify / cancel behind a hard gate | **Yes**, behind two independent gates |
| `cmd/push` | Real-time push feed (TCP + TLS + Protobuf): quotes, ticks, depth, order/position/asset updates | **No** — read-only |

Only `cmd/trade` has a write path at all. The other six binaries never construct a
trade client, so there is no code in them that *could* place an order.

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

Be clear-eyed about what is and is not proven:

- **Proven:** the code compiles, `go vet` is clean, `gofmt` is clean, unit
  tests pass, all seven binaries run, `-h` works without credentials, missing
  credentials produce a precise actionable error, and the dry-run gate provably
  blocks order writes. The configuration loader, redaction, and the
  request-building path are exercised by tests.
- **Also proven:** the HTTP path reaches Tiger's *real* production gateway and
  returns a *real* API error (`code=1000 common param error(tigerId … is
  illegal)`) with deliberately fake credentials. The push client likewise opens
  a real TLS connection to Tiger's push server. Every `-op` of the four
  read-only data commands has been run this way: 77 invocations, each building a
  request, signing it, sending it and decoding a genuine API error response.
  That proves the wiring, the flag parsing and the request construction — and
  nothing more.
- **NOT proven:** no request has ever been made with valid credentials. Field
  names, order-state values, k-line periods, option expiry/strike encoding,
  corporate-action type strings, financial `-fields` values and push callback
  payloads are taken from the SDK's own types and documentation, not from a live
  account. Expect to fix small things on first contact with the real API. In
  particular, the scanner filter JSON, the financial field names and the
  corporate-action type strings are Tiger-side contracts this project can only
  pass through.

Use small orders, and check your positions, on your first live run.

---

## Safety model

Because there is no sim mode, the write path is defended twice. **Both** gates
must be satisfied before a single byte reaches Tiger:

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

The six read-only commands (`quote`, `options`, `futures`, `reference`,
`corporate`, `push`) do not participate in the gate at all, and that is
deliberate rather than an oversight: they build their SDK client through
`internal/rocli`, which only ever calls `Session.Quote()`. There is no trade
client in their process, so no flag combination can make them write. The gate
logic itself is unchanged by the read-only commands.

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

With no credentials you get a precise, actionable error and exit code 2:

```
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
```

`--help` works with no credentials at all and exits 0.

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

# Combine
go run ./cmd/quote -symbols AAPL,0700.HK -klines -depth -market HK -v
```

K-line periods: `day`, `week`, `month`, `year`, `1m`, `5m`, `15m`, `30m`, `60m`.

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

All four read-only data commands share these, alongside their own:

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
```

Feeds: `quote`, `tick`, `depth`, `account`. `Ctrl-C` exits cleanly.

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
| `TIGER_SERVER_URL` | `server_url` | no | production gateway | Trade/common endpoint |
| `TIGER_QUOTE_SERVER_URL` | `quote_server_url` | no | = `server_url` | Quote endpoint |
| `TIGER_PUSH_URL` | `push_url` | no | `openapi.tigerfintech.com:8887` | Push server |
| `TIGER_DRY_RUN` | `dry_run` | no | **`true`** | Order-write kill switch |
| `TIGER_CONFIG` | — | no | — | Path to the YAML config |
| `TIGER_LOG_LEVEL` | `log_level` | no | `info` | `debug`/`info`/`warn`/`error` |

---

## Security notes

- **Secrets are never logged.** `private_key` and `secret_key` are rendered as
  `<redacted:N bytes>` everywhere, including via `fmt.Stringer`, so an
  accidental `fmt.Printf("%v", cfg)` cannot leak one. Covered by tests.
- **`secret_key` is omitted, never empty.** Tiger rejects an empty
  `secret_key` in `biz_content` with `biz_param_error(1010)`. This project
  leaves the field unset unless you actually configured one.
- **Stray properties files are neutralised.** The SDK auto-discovers
  `./tiger_openapi_config.properties` and `~/.tigeropen/…` and will silently
  override credentials you passed explicitly. `internal/tigersdk` re-asserts
  every field after the SDK builds its config, so a stray file cannot redirect
  your orders. You get a warning telling you the file is being ignored.
- **The SDK's own `TIGEROPEN_*` env vars are deliberately unused**, so this
  project's loader stays the single source of truth.
- **`.gitignore` covers `.env`, `*.properties`, `config.local.yaml`,
  `config.yaml`, `*.pem` and `*.key`.** Never commit real credentials.

---

## Project layout

```
tiger-go-demo/
├── cmd/
│   ├── quote/main.go       market data (read-only)
│   ├── quote/output.go     table formatting
│   ├── options/            option market data (read-only)
│   │   ├── main.go         flags + endpoint dispatch
│   │   ├── requests.go     option-identifier parsing
│   │   └── requests_test.go
│   ├── futures/            futures market data (read-only)
│   ├── reference/          reference + fundamental data (read-only)
│   ├── corporate/          corporate actions, warrants, funds (read-only)
│   ├── trade/
│   │   ├── main.go         flag set, write gate, the 3 write paths
│   │   ├── reads.go        the 24 read-only endpoints
│   │   └── dispatch_test.go  reads bypass the gate; writes do not
│   └── push/main.go        real-time push subscriptions
├── internal/
│   ├── config/             env + YAML loader, validation, redaction, write gate
│   │   └── config_test.go
│   ├── logging/            leveled logger implementing the SDK logger interface
│   ├── rocli/              shared read-only plumbing: session, output, exit codes
│   │   ├── rocli.go
│   │   ├── output.go
│   │   └── rocli_test.go
│   └── tigersdk/           SDK client construction; defeats properties-file override
├── .env.example
├── config.example.yaml
├── Makefile
└── README.md
```

`internal/rocli` is what keeps the four read-only data commands small: it owns
flag registration, credential loading, session setup, table formatting and the
exit-code convention, so each command file is just a list of endpoints. It
exposes no trade client, which is why those commands cannot place an order.

---

## SDK coverage

Of the SDK's **117** exported client methods (**78** on `QuoteClient`, **39**
on `TradeClient`), this project now exercises **102**:

| | Initial commit | After the 4 data commands | After `cmd/trade/reads.go` |
|---|---|---|---|
| Overall | 13 / 117 | 79 / 117 | **102 / 117** |
| `QuoteClient` (read-only) | 5 / 78 | 71 / 78 | **70 / 78** |
| `TradeClient` | 8 / 39 | 8 / 39 | **32 / 39** |

(The middle column is the state at commit `83e12a6`; the right-hand column is
the working tree. Both are re-derivable with the loop below against a
`git worktree` of the earlier commit.)

You can re-derive all three numbers without trusting this table:

```console
$ TG=$(go env GOMODCACHE)/github.com/tigerfintech/openapi-go-sdk@v0.5.2
$ grep -rhoE '^func \([a-z] \*(Quote|Trade)Client\) [A-Z][A-Za-z0-9]*' "$TG" --include=*.go \
    | sed -E 's/.*\) //' | sort -u \
  | while read -r m; do
      grep -rqE "\.$m\(" cmd/ || echo "UNCOVERED: $m"
    done
UNCOVERED: GetAddonEntitlement
UNCOVERED: GetBars
UNCOVERED: GetBarsByPage
UNCOVERED: GetBrief
UNCOVERED: GetOptionBrief
UNCOVERED: GetStockDelayBriefs
UNCOVERED: GetWarrantBriefs
UNCOVERED: GrabQuotePermission
UNCOVERED: CancelSegmentFund
UNCOVERED: OptionExerciseCancel
UNCOVERED: OptionExerciseSubmit
UNCOVERED: PlaceForexOrder
UNCOVERED: SetSecretKey
UNCOVERED: TransferPosition
UNCOVERED: TransferSegmentFund
$ # 15 uncovered -> 102 of 117 covered
```

`QuoteClient` goes **down** by one, not up, when `reads.go` lands.
`cmd/reference` used to call `GetStockDelayBriefs`, a deprecated alias, behind a
`-delay-mins` flag that existed only to select it. Both the call and the flag
are gone; `-op delayed` now calls the non-deprecated `GetDelayedQuote`. That is
a fix, not a regression, and it is the reason the quote count drops while the
overall count rises by 24.

### What the 15 uncovered methods actually are

**8 on `QuoteClient`:**

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
  ```

- **`GrabQuotePermission`** — *claims* a market-data permission, so it changes
  account state. It is not a read, and it is deliberately left out.

- **`GetAddonEntitlement`** (`quote/quote_client.go:418`, wire method
  `addon_entitlements`) — a genuine read that returns the account's addon plan
  entitlements. It is **still uncovered**, and that is an open gap, not a
  principled omission. It is the one method in this repo that could be added
  without any safety argument against it; nobody wired it up.

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

This is a **static** check. It proves a method is *referenced from `cmd/`*, not
that it was *exercised against a live Tiger account*. Nothing in this repo has
run with valid credentials — see the "This project has NOT been validated
against the live Tiger API" section near the top of this README.

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

```bash
make help          # list targets
make verify        # gofmt check + go vet + go test -race + build
make build         # binaries into ./bin (all seven)
make test          # unit tests

make run-quote     ARGS="-symbols AAPL"
make run-options   ARGS="-op expiration"
make run-futures   ARGS="-op exchange"
make run-reference ARGS="-op stock-details -symbols AAPL"
make run-corporate ARGS="-op dividend -symbols AAPL -market US"
```

Or directly:

```bash
gofmt -l .         # must print nothing
go build ./...
go vet ./...
go test -race ./...
```

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
$ go test -race ./cmd/trade/ -v
--- PASS: TestReadCommandsBypassTheGate (0.02s)      # 29 subtests, one per read
--- PASS: TestWritesAreRefusedByDefault (0.00s)       # 3 subtests, one per write
--- PASS: TestUnknownCommandIsRejected (0.00s)
--- PASS: TestMissingInputsAreRejectedLocally (0.01s) # 9 subtests
PASS
ok  	github.com/shing1211/tiger-go-demo/cmd/trade	1.089s
```

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

**`warning: found ./tiger_openapi_config.properties … being IGNORED`**
The SDK's auto-discovery file. It is being ignored on purpose. Delete it.

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
