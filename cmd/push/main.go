// Command push subscribes to Tiger's real-time push feed (TCP + TLS +
// Protobuf) and prints the callbacks it receives.
//
// This command is read-only: push is a subscription feed and never writes
// orders. It is included because it is the only way to observe order, asset
// and position changes in real time.
//
// # A nil error from Subscribe* is not an acceptance
//
// The protocol has a subscribe acknowledgement, but the Go SDK discards it:
// handleMessage (push/push_client.go) acts only on CONNECTED, HEARTBEAT,
// MESSAGE, ERROR and DISCONNECT. A subscription the server refuses therefore
// produces neither an error nor a callback — it simply never delivers data,
// and the only evidence is that nothing arrived. So nothing here claims
// success from a Subscribe* return value: a feedTracker counts arrivals in the
// callbacks and reportDelivery says, at shutdown, which feeds delivered
// nothing and why that is often expected (a closed market is not a failure).
//
// # The 1-minute unsubscribe cooldown
//
// Tiger requires at least a minute between the last subscribe and any
// unsubscribe, and a run that unsubscribes at shutdown is always inside that
// window. -unsubscribe therefore refuses to send an unsubscribe it knows the
// server will reject, rather than sending it and hoping (see sendUnsubscribe).
//
// Run it with a timeout; it blocks until interrupted:
//
//	go run ./cmd/push -symbols AAPL,MSFT -subscribe quote,tick,account -duration 60s
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	sdkpush "github.com/tigerfintech/openapi-go-sdk/push"
	sdkpb "github.com/tigerfintech/openapi-go-sdk/push/pb"

	"github.com/shing1211/tiger-go-demo/internal/config"
	"github.com/shing1211/tiger-go-demo/internal/logging"
	"github.com/shing1211/tiger-go-demo/internal/tigersdk"
)

// pushClient is the subset of the SDK push client this command uses, kept as
// an interface so it can be faked in tests.
//
// It exists for the same reason cmd/trade has a tradeClient: widening the
// -subscribe vocabulary must be a visible, reviewable change here, at the
// interface, rather than an invisible one at a call site buried in a switch.
// The trade client (sdktrade.TradeClient) and the order methods are
// deliberately absent from this file entirely — a push command that could
// reach PlaceOrder/ModifyOrder/CancelOrder would need a new import and a new
// line on this interface, and TestPackageCannotReachOrderWrites checks that
// neither exists.
//
// The unsubscribes mirror the API's asymmetry rather than smoothing it over:
// the market-data ones take the symbols to drop (nil = all of them), the two
// ranking ones take (market, indicators), and the four account ones —
// UnsubscribeOrder, UnsubscribePosition, UnsubscribeAsset and
// UnsubscribeTransaction — take no arguments at all, because they can only
// drop the whole subject for the account.
type pushClient interface {
	SetCallbacks(sdkpush.Callbacks)
	Connect() error
	Disconnect() error

	SubscribeQuote([]string) error
	SubscribeTick([]string) error
	SubscribeDepth([]string) error
	SubscribeOption([]string) error
	SubscribeFuture([]string) error
	SubscribeKline([]string) error
	SubscribeCc([]string) error
	SubscribeMarket(string) error
	SubscribeStockTop(string, []string) error
	SubscribeOptionTop(string, []string) error
	SubscribeOrder(string) error
	SubscribePosition(string) error
	SubscribeAsset(string) error
	SubscribeTransaction(string) error

	UnsubscribeQuote([]string) error
	UnsubscribeTick([]string) error
	UnsubscribeDepth([]string) error
	UnsubscribeOption([]string) error
	UnsubscribeFuture([]string) error
	UnsubscribeKline([]string) error
	UnsubscribeCc([]string) error
	UnsubscribeMarket(string) error
	UnsubscribeStockTop(string, []string) error
	UnsubscribeOptionTop(string, []string) error
	UnsubscribeOrder() error
	UnsubscribePosition() error
	UnsubscribeAsset() error
	UnsubscribeTransaction() error
}

var _ pushClient = (*sdkpush.PushClient)(nil)

// unsubscribeCooldown is the minimum gap Tiger allows between the last
// subscribe and an unsubscribe. See the package comment for why -unsubscribe
// is built around refusing rather than hoping.
const unsubscribeCooldown = time.Minute

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "\nerror: %v\n", err)
		if config.MissingCredentialErrorIs(err) {
			os.Exit(2)
		}
		os.Exit(1)
	}
}

// options is the command line, already parsed. It exists so the flag
// definitions can be built without running the command: the defaults are
// behaviour, and -unsubscribe defaulting to on would be a behaviour change
// that only a test of the flag set itself can catch.
type options struct {
	symbols     string
	subscribe   string
	account     bool
	market      string
	indicators  string
	unsubscribe bool
	duration    time.Duration
	configPath  string
	verbose     bool

	// Resolved from the flags above, once the whole line is known.
	feeds []string
	syms  []string
	inds  []string
}

// newFlagSet declares every flag, with its default, and binds it to o.
func newFlagSet(o *options) *flag.FlagSet {
	fs := flag.NewFlagSet("push", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	fs.StringVar(&o.symbols, "symbols", "AAPL,MSFT", "comma-separated symbols to subscribe to")
	fs.StringVar(&o.subscribe, "subscribe", "quote", "comma-separated feeds: "+strings.Join(acceptedFeeds(), ", "))
	fs.BoolVar(&o.account, "account", false, "subscribe to the account feeds (order/position/asset/transaction); implies -subscribe account")
	fs.StringVar(&o.market, "market", "", "market for the market, stock_top and option_top feeds (market=HK; stock_top=US|HK; option_top=US)")
	fs.StringVar(&o.indicators, "indicators", defaultIndicators, "comma-separated indicators for stock_top/option_top")
	fs.BoolVar(&o.unsubscribe, "unsubscribe", false, "unsubscribe before disconnecting (off by default: see the 1-minute cooldown below)")
	fs.DurationVar(&o.duration, "duration", 30*time.Second, "how long to listen before exiting cleanly (0 = until Ctrl-C)")
	fs.StringVar(&o.configPath, "config", "", "path to a YAML config file (also $TIGER_CONFIG)")
	fs.BoolVar(&o.verbose, "v", false, "verbose logging")

	fs.Usage = func() {
		fmt.Fprint(os.Stderr, usage)
		fs.PrintDefaults()
	}
	return fs
}

// resolve validates the flags that can be checked from the command line alone,
// which is every one of them, and fills in the derived fields. It runs before
// the config is loaded so a typo does not present as a missing credential.
func (o *options) resolve() error {
	feeds, err := parseFeeds(o.subscribe, o.account)
	if err != nil {
		return err
	}
	inds, err := parseIndicators(o.indicators)
	if err != nil {
		return err
	}
	if err := validateFeedOptions(feeds, o.market); err != nil {
		return err
	}
	if err := validateIndicators(feeds, inds); err != nil {
		return err
	}
	if err := checkUnsubscribePlan(o.unsubscribe, o.duration); err != nil {
		return err
	}
	o.feeds = feeds
	o.inds = inds
	o.syms = splitSymbols(o.symbols)
	return nil
}

func run(args []string) error {
	var o options
	fs := newFlagSet(&o)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil // help needs no credentials
		}
		return err
	}
	if err := o.resolve(); err != nil {
		return err
	}

	logging.SilenceSDKNoise()
	log := logging.NewFromConfig(os.Stderr, level(o.configPath, o.verbose))

	cfg, err := config.Load(config.Options{ConfigPath: o.configPath})
	if err != nil {
		return err
	}
	tigersdk.WarnStrayProperties(".", os.Stderr)
	log.Info("config loaded:\n%s", cfg.Redacted())

	real, err := tigersdk.Push(cfg, tigersdk.PushOptions{Log: log})
	if err != nil {
		return err
	}
	pc := pushClient(real)

	// Counting has to start before the first Subscribe*, because a feed that
	// delivers nothing is the only signal that the server refused it.
	tracker := newFeedTracker(o.feeds)
	pc.SetCallbacks(callbacks(log, tracker))

	subscribedAt := time.Now()
	if err := pc.Connect(); err != nil {
		return fmt.Errorf("connect to push server %s: %w", cfg.PushURL, err)
	}
	// Safety net for the early returns above and below. Disconnect is
	// idempotent in the SDK, so the orderly path in shutdown can disconnect
	// first without this firing a second time.
	defer func() {
		if err := pc.Disconnect(); err != nil {
			log.Warn("disconnect: %v", err)
		}
	}()

	if err := subscribeFeeds(pc, o.feeds, o.syms, o.market, o.inds, log); err != nil {
		return err
	}

	// Block until the duration elapses or the user interrupts.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var timeout <-chan time.Time
	if o.duration > 0 {
		t := time.NewTimer(o.duration)
		defer t.Stop()
		timeout = t.C
		fmt.Fprintf(os.Stderr, "listening for %s (Ctrl-C to stop early)\n", o.duration)
	}

	select {
	case <-ctx.Done():
		fmt.Fprintln(os.Stderr, "\ninterrupted, shutting down")
	case <-timeout:
		fmt.Fprintln(os.Stderr, "\nduration elapsed, shutting down")
	}

	// What actually arrived is the verdict on every Subscribe* above.
	reportDelivery(log, tracker, o.feeds)

	// Unsubscribe, or explain why not, before the deferred Disconnect runs.
	if err := sendUnsubscribe(log, pc, o.feeds, o.syms, o.market, o.inds, o.unsubscribe, time.Since(subscribedAt)); err != nil {
		return err
	}
	if err := pc.Disconnect(); err != nil {
		log.Warn("disconnect: %v", err)
	}
	return nil
}

// ---- feed vocabulary ----

// canonicalFeeds is every feed the dispatcher understands, in the order the
// flag help lists them. account is a composite: it also subscribes the
// transaction feed, because a fill is only interpretable next to the order it
// fills, and -account must mean the same thing as -subscribe account.
var canonicalFeeds = []string{
	"quote", "tick", "depth", "option", "future", "kline",
	"cc", "market", "stock_top", "option_top",
	"account", "transaction",
}

// feedAliases maps an accepted -subscribe spelling onto the feed it selects.
// crypto is the word people reach for; cc is the SDK's own SubjectType.
var feedAliases = map[string]string{"crypto": "cc"}

// acceptedFeeds is every spelling -subscribe accepts, sorted, for the flag
// help and for the error that names the valid set. Sorting is what makes the
// help stable, and the aliases come from a map whose iteration order is not.
func acceptedFeeds() []string {
	out := make([]string, 0, len(canonicalFeeds)+len(feedAliases))
	out = append(out, canonicalFeeds...)
	for alias := range feedAliases {
		out = append(out, alias)
	}
	sort.Strings(out)
	return out
}

// parseFeeds validates the -subscribe list, canonicalises aliases and expands
// the account composite. It returns feeds in the order given, de-duplicated.
func parseFeeds(s string, forceAccount bool) ([]string, error) {
	known := map[string]bool{}
	for _, f := range canonicalFeeds {
		known[f] = true
	}
	for alias := range feedAliases {
		known[alias] = true
	}

	var picked []string
	seen := map[string]bool{}
	for _, p := range strings.Split(s, ",") {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		if !known[p] {
			return nil, fmt.Errorf("unknown feed %q (want one of: %s)", p, strings.Join(acceptedFeeds(), ", "))
		}
		if canon, isAlias := feedAliases[p]; isAlias {
			p = canon
		}
		if !seen[p] {
			seen[p] = true
			picked = append(picked, p)
		}
	}
	if forceAccount && !seen["account"] {
		seen["account"] = true
		picked = append(picked, "account")
	}

	// Expanding after de-duplication is what makes "-subscribe account,transaction"
	// and "-subscribe account" the same request.
	var out []string
	for _, f := range picked {
		out = append(out, f)
		if f == "account" {
			if !seen["transaction"] {
				seen["transaction"] = true
				out = append(out, "transaction")
			}
		}
	}
	if len(out) == 0 {
		return nil, errors.New("-subscribe selected no feeds")
	}
	return out, nil
}

// validateFeedOptions checks -market against the feed that consumes it. Tiger
// serves the whole-market quote stream for HK only, the stock ranking for US
// and HK, and the option ranking for US only.
func validateFeedOptions(feeds []string, market string) error {
	need := map[string][]string{
		"market":     {"HK"},
		"stock_top":  {"US", "HK"},
		"option_top": {"US"},
	}
	for _, f := range feeds {
		markets, ok := need[f]
		if !ok {
			continue
		}
		if market == "" {
			return fmt.Errorf("-subscribe %s needs -market (want one of: %s)", f, strings.Join(markets, ", "))
		}
		m := strings.ToUpper(strings.TrimSpace(market))
		ok = false
		for _, want := range markets {
			if m == want {
				ok = true
			}
		}
		if !ok {
			return fmt.Errorf("-subscribe %s does not serve -market %s (want one of: %s)", f, market, strings.Join(markets, ", "))
		}
	}
	return nil
}

// ---- indicators ----

// defaultIndicators is the -indicators default. volume and amount are the only
// two names that are valid for both ranking feeds, so one default works for
// -subscribe stock_top and -subscribe option_top alike.
const defaultIndicators = "volume,amount"

var (
	stockTopIndicators  = []string{"changeRate", "changeRate5Min", "turnoverRate", "amount", "volume", "amplitude"}
	optionTopIndicators = []string{"bigOrder", "volume", "amount", "openInt"}
)

// parseIndicators splits -indicators and canonicalises each name, so
// CHANGERATE and changeRate both reach the server as changeRate. The empty
// list is returned as-is; whether that is an error depends on the feed.
func parseIndicators(s string) ([]string, error) {
	canonical := map[string]string{}
	for _, group := range [][]string{stockTopIndicators, optionTopIndicators} {
		for _, name := range group {
			canonical[strings.ToLower(name)] = name
		}
	}
	var out []string
	seen := map[string]bool{}
	for _, p := range strings.Split(s, ",") {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		name, ok := canonical[p]
		if !ok {
			return nil, fmt.Errorf("unknown indicator %q (stock_top wants: %s; option_top wants: %s)",
				p, strings.Join(stockTopIndicators, ", "), strings.Join(optionTopIndicators, ", "))
		}
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	return out, nil
}

// validateIndicators checks the parsed list against the feed that will consume
// it. Both ranking feeds document their indicator list as required, and the
// two valid sets overlap only on volume and amount.
func validateIndicators(feeds []string, inds []string) error {
	for _, f := range feeds {
		var valid []string
		switch f {
		case "stock_top":
			valid = stockTopIndicators
		case "option_top":
			valid = optionTopIndicators
		default:
			continue
		}
		if len(inds) == 0 {
			return fmt.Errorf("-subscribe %s needs at least one -indicators (want one of: %s)", f, strings.Join(valid, ", "))
		}
		for _, name := range inds {
			ok := false
			for _, v := range valid {
				if name == v {
					ok = true
				}
			}
			if !ok {
				return fmt.Errorf("-indicators %q is not valid for -subscribe %s (want one of: %s)", name, f, strings.Join(valid, ", "))
			}
		}
	}
	return nil
}

// ---- dispatch ----

// subscribeFeeds sends one subscribe per feed. A nil error here means only
// that the request was written to the socket, so the log line says "sent"
// rather than "subscribed"; reportDelivery is what confirms delivery.
func subscribeFeeds(pc pushClient, feeds []string, syms []string, market string, inds []string, log *logging.Logger) error {
	for _, f := range feeds {
		var err error
		switch f {
		case "quote":
			err = pc.SubscribeQuote(syms)
		case "tick":
			err = pc.SubscribeTick(syms)
		case "depth":
			err = pc.SubscribeDepth(syms)
		case "option":
			err = pc.SubscribeOption(syms)
		case "future":
			err = pc.SubscribeFuture(syms)
		case "kline":
			err = pc.SubscribeKline(syms)
		case "cc":
			// Tiger's own documents disagree on the crypto symbol form
			// (BTC, BTC.USD, BTCUSD and BTC/USD all appear), so the symbols
			// go out exactly as given and feedHint tells the user which
			// forms to try if nothing arrives.
			err = pc.SubscribeCc(syms)
		case "market":
			// Not market status. The wire sends dataType=Quote with the
			// market field set and no symbols, so the payloads come back
			// through OnQuote and are printed as [QUOTE] like any other
			// quote — the wire does not say which feed a quote came from.
			err = pc.SubscribeMarket(market)
		case "stock_top":
			err = pc.SubscribeStockTop(market, inds)
		case "option_top":
			err = pc.SubscribeOptionTop(market, inds)
		case "account":
			// The three account feeds, in the order the pre-existing command
			// used. Empty string means "use the account from config", so the
			// account never travels on the wire from this command.
			if err = pc.SubscribeOrder(""); err == nil {
				err = pc.SubscribePosition("")
			}
			if err == nil {
				err = pc.SubscribeAsset("")
			}
		case "transaction":
			err = pc.SubscribeTransaction("")
		}
		if err != nil {
			return fmt.Errorf("subscribe %s: %w", f, err)
		}
		log.Info("subscribe sent: feed=%s symbols=%v market=%s indicators=%v (delivery is confirmed below, not by this line)", f, syms, market, inds)
	}
	return nil
}

// unsubscribeFeeds is subscribeFeeds' mirror, used only when the cooldown in
// sendUnsubscribe has been satisfied.
func unsubscribeFeeds(pc pushClient, feeds []string, syms []string, market string, inds []string, log *logging.Logger) error {
	for _, f := range feeds {
		var err error
		switch f {
		case "quote":
			err = pc.UnsubscribeQuote(syms)
		case "tick":
			err = pc.UnsubscribeTick(syms)
		case "depth":
			err = pc.UnsubscribeDepth(syms)
		case "option":
			err = pc.UnsubscribeOption(syms)
		case "future":
			err = pc.UnsubscribeFuture(syms)
		case "kline":
			err = pc.UnsubscribeKline(syms)
		case "cc":
			err = pc.UnsubscribeCc(syms)
		case "market":
			err = pc.UnsubscribeMarket(market)
		case "stock_top":
			err = pc.UnsubscribeStockTop(market, inds)
		case "option_top":
			err = pc.UnsubscribeOptionTop(market, inds)
		case "account":
			// These three take no argument: the API can only drop the whole
			// subject, not one of its parts.
			if err = pc.UnsubscribeOrder(); err == nil {
				err = pc.UnsubscribePosition()
			}
			if err == nil {
				err = pc.UnsubscribeAsset()
			}
		case "transaction":
			err = pc.UnsubscribeTransaction()
		}
		if err != nil {
			return fmt.Errorf("unsubscribe %s: %w", f, err)
		}
		log.Info("unsubscribe sent: feed=%s symbols=%v market=%s indicators=%v", f, syms, market, inds)
	}
	return nil
}

// checkUnsubscribePlan refuses a -unsubscribe/-duration pair that could not
// possibly work. A run shorter than the cooldown can only end by asking the
// server to undo a subscription it made seconds ago, so the request is
// rejected here — before any connection is made — rather than sent and
// hoped for.
func checkUnsubscribePlan(unsubscribe bool, duration time.Duration) error {
	if !unsubscribe {
		return nil
	}
	if duration > 0 && duration < unsubscribeCooldown {
		return fmt.Errorf("-unsubscribe needs -duration of at least %s: Tiger rejects an unsubscribe sent less than %s after subscribing",
			unsubscribeCooldown, unsubscribeCooldown)
	}
	return nil
}

// sendUnsubscribe honours -unsubscribe once the cooldown has elapsed and says
// so plainly when it has not.
//
// The refusal is the honest option. The alternative — send it anyway and warn
// that the server will probably ignore it — reports a clean-up that did not
// happen, and a second connection is a way to make things worse: Tiger allows
// one push connection per Tiger ID, and a rejected unsubscribe leaves this
// connection's subscriptions exactly where they were. Silence plus a warning
// reads as "it worked"; an explicit "not sent, and here is why" cannot.
func sendUnsubscribe(log *logging.Logger, pc pushClient, feeds []string, syms []string, market string, inds []string, requested bool, since time.Duration) error {
	if !requested {
		return nil
	}
	if since < unsubscribeCooldown {
		log.Warn("not unsubscribing: only %s since the last subscribe, and Tiger rejects an unsubscribe inside the %s cooldown (nothing was sent; the connection is simply closing)",
			since.Round(time.Second), unsubscribeCooldown)
		return nil
	}
	return unsubscribeFeeds(pc, feeds, syms, market, inds, log)
}

// ---- delivery accounting ----

// feedTracker counts arrivals per feed.
//
// The wire does not always say which subscription a payload belongs to: the
// cc and market feeds both reuse QuoteData and arrive through OnQuote, so a
// quote marks every subscribed feed that OnQuote can serve. That over-counts
// rather than under-counts, which is the right way to be wrong here — the
// cost of a false negative is a loud warning nobody can act on.
type feedTracker struct {
	mu     sync.Mutex
	feeds  map[string]bool
	counts map[string]int
}

func newFeedTracker(feeds []string) *feedTracker {
	t := &feedTracker{feeds: map[string]bool{}, counts: map[string]int{}}
	for _, f := range feeds {
		t.feeds[f] = true
	}
	return t
}

// arrive records one delivery against every named feed that is subscribed.
func (t *feedTracker) arrive(names ...string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, n := range names {
		if t.feeds[n] {
			t.counts[n]++
		}
	}
}

func (t *feedTracker) count(feed string) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.counts[feed]
}

// reportDelivery is the verdict on the subscribe calls: for each feed, how
// much arrived, and for a silent feed the reason that is most often benign.
func reportDelivery(log *logging.Logger, t *feedTracker, feeds []string) {
	log.Info("delivery report: %s", strings.Join(feeds, ", "))
	for _, f := range feeds {
		if n := t.count(f); n > 0 {
			log.Info("  delivered: feed=%s messages=%d", f, n)
		} else {
			log.Warn("  NO DATA: feed=%s — %s", f, feedHint(f))
		}
	}
}

// feedHint explains a silent feed. Every line names the three server-side
// causes that produce silence with no error at all, because the SDK cannot
// distinguish them: a refusal carrying a business code (3xxx), a per-data-type
// subscription quota (code 4, "you can only subscribe to xxx symbols"), and a
// market that is closed. The feed-specific reason leads because it is the one
// that is actually likely.
func feedHint(feed string) string {
	const common = "the server may have refused it (business code 3xxx), the per-data-type subscription quota may be exhausted (code 4), or the market may be closed"
	switch feed {
	case "account":
		return "account feeds are event-driven and only move when something happens on the account; " + common
	case "transaction":
		return "fills only arrive when an order fills; " + common
	case "stock_top", "option_top":
		return "rankings are pushed only during market hours, about every 30s, and US pre/post sessions push only changeRate and changeRate5Min; " + common
	case "market":
		return "the whole-market quote stream serves HK only and follows the HK session; " + common
	case "cc":
		return "Tiger's docs disagree on the crypto symbol form, so try BTC, BTC.USD, BTCUSD or BTC/USD in -symbols; the payload reuses QuoteData and prints as [QUOTE]; " + common
	default:
		return common
	}
}

// ---- callbacks ----

func callbacks(log *logging.Logger, t *feedTracker) sdkpush.Callbacks {
	return sdkpush.Callbacks{
		OnConnect:    func() { log.Info("push connected") },
		OnDisconnect: func() { log.Warn("push disconnected") },
		OnKickout:    func(m string) { log.Error("kicked out: %s", m) },
		OnError:      func(err error) { log.Error("push error: %v", err) },

		OnQuote: func(d *sdkpb.QuoteData) {
			// Also serves the cc and market feeds: the SDK routes all three
			// through OnQuote, so crypto and whole-market quotes are
			// visually identical to equity quotes.
			t.arrive("quote", "cc", "market")
			printQuote("QUOTE", d)
		},
		OnOption: func(d *sdkpb.QuoteData) {
			t.arrive("option")
			printQuote("OPTION", d)
		},
		OnFuture: func(d *sdkpb.QuoteData) {
			t.arrive("future")
			printQuote("FUTURE", d)
		},
		OnTick: func(d sdkpush.PushTradeTick) {
			t.arrive("tick")
			for _, tick := range d.Ticks {
				fmt.Printf("[TICK]  %-10s sn=%d price=%.4f vol=%d type=%s cond=%s venue=%s\n",
					d.Symbol, tick.Sn, tick.Price, tick.Volume, tick.TickType, tick.Cond, tick.PartCode)
			}
		},
		OnDepth: func(d *sdkpb.QuoteDepthData) {
			t.arrive("depth")
			// The order book arrives as parallel slices (price/volume/orderCount),
			// not as a list of structs, so index them in lockstep.
			fmt.Printf("[DEPTH] %-10s\n", d.GetSymbol())
			printBookSide("  bid", d.GetBid())
			printBookSide("  ask", d.GetAsk())
		},
		OnKline: func(d *sdkpb.KlineData) {
			t.arrive("kline")
			printKline(d)
		},
		OnStockTop: func(d *sdkpb.StockTopData) {
			t.arrive("stock_top")
			printStockTop(d)
		},
		OnOptionTop: func(d *sdkpb.OptionTopData) {
			t.arrive("option_top")
			printOptionTop(d)
		},

		OnOrder: func(d *sdkpb.OrderStatusData) {
			t.arrive("account")
			fmt.Printf("[ORDER] id=%d %-10s %s %s %d/%d @%.4f status=%s%s\n",
				d.GetId(), d.GetSymbol(), d.GetAction(), d.GetOrderType(),
				d.GetFilledQuantity(), d.GetTotalQuantity(), d.GetAvgFillPrice(),
				d.GetStatus(), errorSuffix(d.GetErrorMsg()))
		},
		OnPosition: func(d *sdkpb.PositionData) {
			t.arrive("account")
			fmt.Printf("[POS]   %-10s qty=%d avg_cost=%.4f mkt_value=%.2f unreal_pnl=%.2f\n",
				d.GetSymbol(), d.GetPosition(), d.GetAverageCost(), d.GetMarketValue(), d.GetUnrealizedPnl())
		},
		OnAsset: func(d *sdkpb.AssetData) {
			t.arrive("account")
			fmt.Printf("[ASSET] account=%s ccy=%s cash=%.2f net_liq=%.2f buying_power=%.2f\n",
				d.GetAccount(), d.GetCurrency(), d.GetCashBalance(), d.GetNetLiquidation(), d.GetBuyingPower())
		},
		OnTransaction: func(d *sdkpb.OrderTransactionData) {
			t.arrive("transaction")
			fmt.Printf("[FILL]  order_id=%d %-10s %d @%.4f\n",
				d.GetId(), d.GetSymbol(), d.GetFilledQuantity(), d.GetFilledPrice())
		},
	}
}

// printBookSide renders up to five levels of one side of the book.
func printBookSide(label string, book *sdkpb.QuoteDepthData_OrderBook) {
	if book == nil {
		fmt.Printf("%s (none)\n", label)
		return
	}
	prices, volumes, counts := book.GetPrice(), book.GetVolume(), book.GetOrderCount()
	n := len(prices)
	if len(volumes) < n {
		n = len(volumes)
	}
	if n == 0 {
		fmt.Printf("%s (empty)\n", label)
		return
	}
	fmt.Printf("%s %d level(s):\n", label, len(prices))
	for i := 0; i < n && i < 5; i++ {
		count := "-"
		if i < len(counts) {
			count = fmt.Sprintf("%d", counts[i])
		}
		fmt.Printf("      %10.4f  vol=%-10d orders=%s\n", prices[i], volumes[i], count)
	}
	if len(prices) > 5 {
		fmt.Printf("      ... %d more level(s)\n", len(prices)-5)
	}
}

func errorSuffix(msg string) string {
	if msg == "" {
		return ""
	}
	return " err=" + msg
}

func splitSymbols(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, strings.ToUpper(p))
		}
	}
	return out
}

func level(configPath string, verbose bool) string {
	if verbose {
		return "debug"
	}
	if p := config.ResolveConfigPath(configPath, os.Getenv); p != "" {
		if raw, err := os.ReadFile(p); err == nil {
			for _, line := range strings.Split(string(raw), "\n") {
				if t := strings.TrimSpace(line); strings.HasPrefix(t, "log_level:") {
					return strings.TrimSpace(strings.TrimPrefix(t, "log_level:"))
				}
			}
		}
	}
	return "info"
}

const usage = `push — Tiger real-time push feed (read-only)

Connects to Tiger's push server over TCP + TLS and prints decoded Protobuf
callbacks. Push is a subscription feed only; it never writes orders.

Feeds (comma-separated, -subscribe):
  quote tick depth option future kline   market data for -symbols
  cc (or crypto)                        digital-currency quotes
  market                                whole-market quote stream, HK only
  stock_top                             US/HK ranking, needs -market + -indicators
  option_top                            US ranking, needs -market + -indicators
  account                               order + position + asset + transaction

A nil error from a subscribe is not an acceptance: the SDK discards the
server's acknowledgement, so a refused subscription is silent. This command
counts what arrives and reports, per feed, at shutdown. Silence is usually a
closed market, not a failure.

Option symbols contain spaces — quote them, or your shell will eat them:
  -symbols "AAPL  260619C00200000"   (OCC, the two spaces are padding)
  -symbols "AAPL 260619C00200000"    (readable form)

-limitations worth knowing:
  one push connection per Tiger ID; a second connection kicks the first (4001)
  subscribing consumes per-data-type quota; exceeding it is code 4
  unsubscribe only works >=1m after subscribing, so -unsubscribe is off by
  default and refuses a -duration shorter than that

No credentials are needed for -h.

Examples:
  go run ./cmd/push -symbols AAPL,MSFT -subscribe quote
  go run ./cmd/push -symbols AAPL -subscribe tick -duration 2m
  go run ./cmd/push -subscribe account -account -duration 5m
  go run ./cmd/push -symbols AAPL -subscribe quote,depth -v
  go run ./cmd/push -subscribe kline -symbols AAPL -duration 1m
  go run ./cmd/push -subscribe stock_top -market US -indicators changeRate,volume
  go run ./cmd/push -subscribe market -market HK -duration 2m

Flags:
`
