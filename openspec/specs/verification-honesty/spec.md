# verification-honesty Specification

## Purpose

No successful authenticated response from the trading API has ever been observed in this project: it was written without a valid key, so every request it has ever made was rejected — and, unlike its sibling project, this provider does not answer an invalid request with an authorisation error but with a parameter error. This capability fixes the rule that follows: an artifact describes what was observed and what was tested, a rejected request proves only that a request was built, signed, delivered and refused, and a record of past runs is not a statement about the present.

## Requirements

### Requirement: No successful authenticated response has been observed

No successful authenticated response from the trading API SHALL be claimed, implied or reconstructed in any artifact of this project. Where an artifact describes what the API returns, it SHALL say that the shape is taken from the interface contract and not from a live account.

#### Scenario: An artifact does not describe a success as observed

- **WHEN** an artifact describes a field, value, enum or encoding of an API response
- **THEN** it does not present that detail as something a live account has been observed to return

#### Scenario: An artifact does not promise an output a live run has not produced

- **WHEN** an artifact shows sample output for a request against the API
- **THEN** it does not present that output as a transcript from a successful call

#### Scenario: The first-contact cost is stated, not hidden

- **WHEN** an artifact describes what is unverified
- **THEN** it says plainly that small corrections are expected on first contact with the real API

### Requirement: The rejected-request evidence is a parameter error, not an authorisation error

The evidence that requests reach the real gateway and are refused SHALL be described as a parameter-error response to a deliberately invalid request. It SHALL NOT be described as an unauthorised response, and no authorisation-error evidence SHALL be claimed, because this provider answers an invalid request with a parameter error.

#### Scenario: The evidence is described as a parameter error

- **WHEN** an artifact cites a real API response as proof that the request path works
- **THEN** it identifies that response as a parameter error returned to deliberately invalid credentials

#### Scenario: No authorisation error is claimed

- **WHEN** an artifact lists what has been observed from the real gateway
- **THEN** it does not include an unauthorised response, because this project has never recorded one

#### Scenario: The claim made about a rejected request is limited to the request

- **WHEN** an artifact uses a real API error as evidence that a code path is genuinely wired
- **THEN** the artifact limits the claim to the request being built, signed, sent and refused

#### Scenario: Nothing is claimed about a successful payload's shape

- **WHEN** an artifact uses a real API error as evidence for a path
- **THEN** it makes no claim about what a successful response would contain, including field names, order-state values, period names, expiry or strike encoding, filter syntax, or push callback payloads

### Requirement: A record of past runs is not a statement about the present

An artifact that records endpoint invocations from an earlier run SHALL label that record as a past run, SHALL NOT present its endpoint list as the project's current surface, and SHALL NOT present its count as covering what the project can reach today.

#### Scenario: The invocation record is labelled historical

- **WHEN** an artifact reports how many endpoint invocations were made against the real gateway
- **THEN** it states that the record is of a past run and not a current state

#### Scenario: The count is not read as current coverage

- **WHEN** an artifact reports a historical invocation count
- **THEN** it does not present that count as covering the project's present endpoint list

#### Scenario: What the historical run did and did not include is stated

- **WHEN** an artifact reports a historical invocation count
- **THEN** it names at least one endpoint that the run did not include, so the count cannot be read as exhaustive

#### Scenario: Reproducing the record is described as not routine

- **WHEN** an artifact reports a historical invocation count
- **THEN** it says the record cannot simply be re-run on demand

### Requirement: Nothing in this project has been verified against live data

No artifact of this project SHALL state or imply that any command has been run successfully against the provider with working credentials. Where an artifact shows output shape, a sample, or a field value, it SHALL say whether that material comes from the code and the interface contract or from a live run, and this project SHALL be recorded as having the former only.

#### Scenario: A sample is attributed to its origin

- **WHEN** an artifact shows sample output
- **THEN** it says the material is the shape the code emits, and does not present it as a transcript of a successful live call

#### Scenario: A behaviour is not described as observed live

- **WHEN** an artifact describes what a command does against the provider
- **THEN** it does not claim the behaviour was observed with working credentials, because nothing in this project has been

#### Scenario: Per-payload detail is marked as unverified

- **WHEN** an artifact describes the content of a real-time data frame
- **THEN** it records that the field values come from the interface contract and have not been seen from the provider

### Requirement: A subscription is not confirmed until data arrives

The real-time feed's client library SHALL expose no path by which a subscription request is acknowledged. A successful return from a subscribe call SHALL mean only that the request was written to the connection, and SHALL NOT be presented as evidence that the server accepted it. Any claim that a subscription is live SHALL be supported by received data frames for that feed.

#### Scenario: A nil return is not described as an acceptance

- **WHEN** an artifact describes the result of a subscribe call
- **THEN** it states that success means only that the request was sent, and does not claim the server accepted the subscription

#### Scenario: Acceptance is claimed only from data

- **WHEN** an artifact states that a subscription is live
- **THEN** it points at received data frames for that feed as the evidence, never at the subscribe call's return value

#### Scenario: The absence of an error path is stated

- **WHEN** an artifact explains why a refusal is hard to see
- **THEN** it records that the client library acts on connection, heartbeat, data, error and disconnection messages only, and therefore discards the server's subscription acknowledgement

### Requirement: A silent feed is reported rather than assumed healthy

Because a refused subscription produces neither an error nor a callback, silence is the only signal a refusal leaves. A command SHALL therefore count what arrives per feed and report, at the end of a run, which feeds delivered nothing and why that is often expected. A run with a silent feed SHALL NOT be reported as a fully successful subscription.

#### Scenario: Arrivals are counted per feed

- **WHEN** a run receives data frames
- **THEN** the command attributes them to the feeds those frames can serve, and reports a per-feed count at the end of the run

#### Scenario: A silent feed is named, with its likeliest reason

- **WHEN** a feed delivered nothing during the run
- **THEN** the report names that feed and gives the reason it is most likely to have been quiet, rather than leaving the absence unexplained

#### Scenario: A quiet feed is a warning, not a failure

- **WHEN** a feed delivered nothing during the run
- **THEN** the report is a warning rather than an error, because a closed market and a refused subscription are indistinguishable from the client side

#### Scenario: Over-counting is preferred to under-counting

- **WHEN** one data frame could belong to more than one subscribed feed
- **THEN** the command credits every feed that frame can serve, because a false negative would raise a warning about a feed that is plainly delivering

### Requirement: An undocumented vendor format is passed through and flagged

Where the vendor does not document a required format, a command SHALL pass the user-supplied value through unchanged rather than guessing, transforming or validating it against an undocumented assumption. It SHALL warn when no data arrives and name the candidate forms a user can try. A subscription on such a feed SHALL NOT be presented as verified.

#### Scenario: The user's value is not rewritten

- **WHEN** a required symbol format is not documented by the vendor
- **THEN** the value the user supplied is sent exactly as given, with no substitution or normalisation applied to it

#### Scenario: The undocumented format is named when nothing arrives

- **WHEN** a feed documented this way delivers no data
- **THEN** the run says the format is undocumented and lists the candidate forms to try, rather than reporting a bare failure

#### Scenario: The feed is not claimed as working

- **WHEN** an artifact describes a subscription on an undocumented-format feed
- **THEN** it does not state that the subscription is confirmed, because which form the provider accepts has not been established

### Requirement: The unsubscribe cooldown policy is a refusal, not a warning

Where the vendor requires a minimum interval between a subscription and its cancellation, a command SHALL refuse to send a cancellation it knows falls inside that interval, and SHALL say that nothing was sent and why. It SHALL NOT send the request regardless and merely warn that it will probably be ignored. A run's default configuration SHALL NOT request a cancellation at all.

#### Scenario: Cancellation is off unless asked for

- **WHEN** a command starts with default flags
- **THEN** no cancellation is sent, because a run that ends soon after subscribing would fall inside the interval anyway

#### Scenario: A cancellation inside the interval is not sent

- **WHEN** a run ends less than the required interval after its last subscription
- **THEN** nothing is written, and the run says nothing was sent and why, rather than reporting a clean-up

#### Scenario: A warning instead of a refusal is not the policy

- **WHEN** the required interval has not elapsed
- **THEN** the command does not send the request and then warn that the server will probably ignore it, because a warning after a silent send reads as a clean-up that happened

#### Scenario: A permitted cancellation is sent before the connection closes

- **WHEN** the required interval has elapsed
- **THEN** the cancellations are written while the connection is still open, and before it is closed, because a closed connection has nothing left to write to

### Requirement: An impossible cancellation is refused before any connection is made

The pre-flight half matters as much as the shutdown half. A command-line combination that could not possibly work SHALL be refused before any connection is opened, so the user sees an error about the flags rather than a clean-up that did not happen.

#### Scenario: An impossible flag combination is refused up front

- **WHEN** a run requests cancellation with a planned duration shorter than the required interval
- **THEN** the command exits before opening any connection, with a message naming both flags and the required interval

#### Scenario: A run that cannot tell its own length is not refused up front

- **WHEN** a run requests cancellation and has no planned end, continuing until interrupted
- **THEN** the command proceeds, because the elapsed interval cannot be judged before the run happens

#### Scenario: A flag problem is not reported as a connection problem

- **WHEN** a flag combination is refused before the run starts
- **THEN** the message is about the flags, so a typo is diagnosable without a network or credentials

### Requirement: A local record is not presented as a server answer

A report drawn from a process's own in-memory bookkeeping SHALL be labelled as local, SHALL say that the server was not asked, and SHALL NOT be phrased so that an empty result reads as a statement about the account. The label SHALL appear before the result rather than after it.

#### Scenario: The locality is stated before the list

- **WHEN** a run reports a subscription record
- **THEN** the words indicating it is local and that the server was not asked are printed before any subject, so an empty list cannot be read as an answer about the account

#### Scenario: An empty local record says why it is empty

- **WHEN** the local record is empty
- **THEN** the output says that a record is made only by a subscribe call and points at the command that makes them

### Requirement: A token rotation has not been observed to succeed

A command that asks the vendor's gateway for a new token SHALL be documented as unverified beyond being wired, because this project has never made an authenticated request. It SHALL NOT be described as working against a live account, and it SHALL be called in a way that writes the new token to no file.

#### Scenario: The rotation is documented as unverified

- **WHEN** an artifact describes the token refresh path
- **THEN** it states that no successful authenticated response has been observed for it

#### Scenario: The rotation persists nothing

- **WHEN** the refresh is invoked
- **THEN** it is invoked with no token manager, so the new token is held in memory only and no file is written
