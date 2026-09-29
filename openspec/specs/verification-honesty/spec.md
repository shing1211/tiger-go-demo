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
