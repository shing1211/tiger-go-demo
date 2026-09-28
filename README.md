# tiger-go-demo

A self-contained, safe-by-default Go demo of the **Tiger Brokers OpenAPI**,
built on the official SDK [`github.com/tigerfintech/openapi-go-sdk`](https://github.com/tigerfintech/openapi-go-sdk) v0.5.2.

Three commands:

| Command | What it does | Writes orders? |
|---|---|---|
| `cmd/quote` | Real-time briefs, historical bars, intraday timeline, market state, order-book depth | **No** — read-only |
| `cmd/trade` | Account assets, positions, order list; place / modify / cancel behind a hard gate | **Yes**, behind two independent gates |
| `cmd/push` | Real-time push feed (TCP + TLS + Protobuf): quotes, ticks, depth, order/position/asset updates | **No** — read-only |

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
  tests pass, all three binaries run, `-h` works without credentials, missing
  credentials produce a precise actionable error, and the dry-run gate provably
  blocks order writes. The configuration loader, redaction, and the
  request-building path are exercised by tests.
- **Also proven:** the HTTP path reaches Tiger's *real* production gateway and
  returns a *real* API error (`code=1000 common param error(tigerId … is
  illegal)`) with deliberately fake credentials. The push client likewise opens
  a real TLS connection to Tiger's push server.
- **NOT proven:** no request has ever been made with valid credentials. Field
  names, order-state values, k-line periods, option fields, and push callback
  payloads are taken from the SDK's own types and documentation, not from a
  live account. Expect to fix small things on first contact with the real API.

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
route to a write. Read-only operations (`assets`, `positions`, `orders`,
`active-orders`, `preview`) ignore the gate, since `preview` asks Tiger to
validate an order without placing it.

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

### `trade` — account and orders

```bash
# Read-only queries (no gate needed)
go run ./cmd/trade -command assets
go run ./cmd/trade -command positions
go run ./cmd/trade -command orders -limit 20
go run ./cmd/trade -command active-orders -states NEW,HELD

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
│   ├── quote/main.go      market data (read-only)
│   ├── quote/output.go    table formatting
│   ├── trade/main.go      account + gated order writes
│   └── push/main.go       real-time push subscriptions
├── internal/
│   ├── config/            env + YAML loader, validation, redaction, write gate
│   │   └── config_test.go
│   ├── logging/           leveled logger implementing the SDK logger interface
│   └── tigersdk/          SDK client construction; defeats properties-file override
├── .env.example
├── config.example.yaml
├── Makefile
└── README.md
```

---

## Development

```bash
make help          # list targets
make verify        # gofmt check + go vet + go test -race + build
make build         # binaries into ./bin
make test          # unit tests
```

Or directly:

```bash
gofmt -l .         # must print nothing
go build ./...
go vet ./...
go test -race ./...
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
