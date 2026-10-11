# Spec Delta: verification-honesty

## MODIFIED Requirements

### Requirement: No successful authenticated response has been observed

No successful authenticated response from the trading API SHALL be claimed, implied or reconstructed in any artifact of this project. Where an artifact describes what the API returns, it SHALL say that the shape is taken from the interface contract and not from a live account. This requirement is unchanged; the delta below adds a smoke-test scenario that exercises the wiring with invalid credentials.

#### Scenario: An artifact does not describe a success as observed

- **WHEN** an artifact describes a field, value, enum or encoding of an API response
- **THEN** it does not present that detail as something a live account has been observed to return

#### Scenario: An artifact does not promise an output a live run has not produced

- **WHEN** an artifact describes a field, value, enum or encoding of an API response
- **THEN** it does not present that detail as something a live account has been observed to return

#### Scenario: The first-contact cost is stated, not hidden

- **WHEN** an artifact describes what is unverified
- **THEN** it says plainly that small corrections are expected on first contact with the real API

### Requirement: A credentials-free smoke test proves the wiring is intact

A credentials-free test SHALL exercise the real Tiger gateway with deliberately invalid credentials and SHALL assert that the response is a parameter-error rather than a network or TLS failure. This test extends the verifiable coverage by proving the SDK wiring, request signing, and HTTP transport without requiring real credentials.

#### Scenario: The test runs without credentials and produces a parameter error

- **WHEN** the smoke test runs with no TIGER_ID or an invalid private key
- **THEN** it receives a parameter-error response and exits without error

#### Scenario: The test distinguishes a parameter error from other failures

- **WHEN** the smoke test runs
- **THEN** it passes only on a parameter-error response and fails on a TLS error, a connection refused error, or any other non-parameter-error response
