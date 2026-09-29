package sdkcoverage

// Reason is why a method is deliberately not referenced.
//
// The vocabulary is closed, and it is closed in the type system: this is a
// defined type with five constants and no other value, so a typo or an
// invented sixth reason does not compile. That inverts the previous failure
// mode, where the allow-list was a shell literal and a mistyped reason was a
// word in a comment nobody re-read.
//
// A note on the two hyphenated values, because "every reason is a single word"
// is a scenario in the sdk-coverage spec and it cannot mean "contains no
// hyphen" without rejecting the spec's own vocabulary. It is read as what it
// is reaching for: a reason carries no free text after it. There is no field
// in which commentary could ride along, which is the property worth having, and
// the words themselves stay as they are so that renaming them does not churn
// the Makefile, the README and the spec at once.
type Reason string

const (
	// ReasonDeprecated: a non-deprecated replacement is what the commands call.
	ReasonDeprecated Reason = "deprecated"
	// ReasonMutating: would change account state, so it would need the write
	// gate in front of it, and nothing in this project puts a gate in front of
	// an unwired method.
	ReasonMutating Reason = "mutating"
	// ReasonNotACall: not an API call at all — a local in-memory assignment on
	// the client struct, with no HTTP request to route or gate.
	ReasonNotACall Reason = "not-a-call"
	// ReasonNotUsed: a user-facing escape hatch this project has not reached.
	ReasonNotUsed Reason = "not-used"
	// ReasonInternal: the library's own shipped code calls it. This is the only
	// reason that asserts something about the vendor rather than about this
	// project, so it is the only one that can be checked — and it is, against
	// the module source. See verify.go.
	ReasonInternal Reason = "internal"
)

// Entry pairs an unreferenced method with its structural reason.
type Entry struct {
	Method string
	Reason Reason
}

// AllowList is the exact set of methods this project does not reference.
//
// It is asserted, not counted, and in both directions. A method that is
// neither covered nor listed fails, and so does a listed method that gained a
// call site. Both directions are load-bearing: the first stops a gap appearing,
// the second stops a line outliving the reason it was written for. Comparing
// the set rather than the count matters too, because a substitution of one
// method for another leaves the count identical.
//
// The entries are sorted by method, bytewise as Go sorts strings, so a diff
// shows only the line that changed. That is not the same as alphabetical
// order to a reader: every uppercase name precedes every lowercase one, so
// SetSecretKey sits between SecretKey and StartTokenAutoRefresh. The grouping
// comments below therefore sit at the ends of their runs rather than
// introducing a break in the order.
//
// The reason breakdown is 6 deprecated, 7 mutating, 1 not-a-call, 1 not-used,
// 4 internal — 19 in total, which is the figure the sdk-coverage spec pins.
var allowList = []Entry{
	// --- account-mutating ---
	//
	// Each would need the write gate in front of it, which is exactly why none
	// is wired up. The read side of every one of these operations is covered in
	// cmd/trade/reads.go wherever the SDK provides one. The split is drawn on
	// whether a method changes account state, not on the HTTP verb: every
	// method in this library is a POST, including reads that cost a real round
	// trip.
	{Method: "CancelSegmentFund", Reason: ReasonMutating},

	// --- called by the library's own shipped packages ---
	//
	// verify.go derives this from the module source rather than trusting the
	// claim: Execute from client/, quote/ and trade/; QueryToken and
	// StartTokenAutoRefresh from client/; SecretKey from trade/.
	{Method: "Execute", Reason: ReasonInternal},

	// A raw escape hatch: hand it a method name and a JSON body and it posts
	// them. It has zero callers anywhere in the module, including the library's
	// own code and its tests.
	//
	// Three methods once sat here and two of them are now covered, from
	// cmd/token: RefreshToken, SetCurrentToken and GetAccountSubscriptions were
	// "not-used" rather than "internal" for as long as they were uncovered,
	// because labelling them "internal" would assert something the SDK source
	// does not support — nothing in the SDK's own importable packages calls
	// them, only the example programs in examples/manual_test/ and
	// cmd/integ_token_refresh/. That is still true of the SDK and is still why
	// labelling them "internal" would be wrong if they became uncovered again.
	// A covered method carries no allow-list line, so the question does not
	// arise for them today; the record matters if it ever does.
	{Method: "ExecuteRaw", Reason: ReasonNotUsed},

	// --- deprecated aliases ---
	//
	// Each carries a Deprecated: line in the SDK pointing at a replacement, and
	// the commands call the replacement: GetRealTimeQuote, GetKline,
	// GetKlineByPage, GetOptionQuote, GetWarrantQuote, GetDelayedQuote. A
	// deprecated alias gaining a call site is a failure, not progress, so these
	// lines disappear the moment that happens.
	{Method: "GetBars", Reason: ReasonDeprecated},
	{Method: "GetBarsByPage", Reason: ReasonDeprecated},
	{Method: "GetBrief", Reason: ReasonDeprecated},
	{Method: "GetOptionBrief", Reason: ReasonDeprecated},
	{Method: "GetStockDelayBriefs", Reason: ReasonDeprecated},
	{Method: "GetWarrantBriefs", Reason: ReasonDeprecated},

	{Method: "GrabQuotePermission", Reason: ReasonMutating},
	{Method: "OptionExerciseCancel", Reason: ReasonMutating},
	{Method: "OptionExerciseSubmit", Reason: ReasonMutating},
	{Method: "PlaceForexOrder", Reason: ReasonMutating},
	{Method: "QueryToken", Reason: ReasonInternal},
	{Method: "SecretKey", Reason: ReasonInternal},

	// A local struct-field assignment (c.secretKey = key). No HTTP request, so
	// there is nothing to gate and nothing to route.
	{Method: "SetSecretKey", Reason: ReasonNotACall},

	{Method: "StartTokenAutoRefresh", Reason: ReasonInternal},
	{Method: "TransferPosition", Reason: ReasonMutating},
	{Method: "TransferSegmentFund", Reason: ReasonMutating},
}

// AllowList returns a copy, so a caller cannot rewrite the table.
func AllowList() []Entry {
	return append([]Entry(nil), allowList...)
}
