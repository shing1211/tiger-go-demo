# secret-redaction Specification

## Purpose

Two values in this project are credentials in the full sense: the RSA private key that signs every request, and the app secret used by institutional accounts. Neither may ever reach a terminal, a log line, or a formatted error. This capability fixes the exact form a secret takes in any printable rendering, makes the string form of the configuration the redacted one so that an accidental print cannot leak, and records that an unset app secret is left out of a request rather than sent empty.

## Requirements

### Requirement: Secrets render as a length-only placeholder

A non-empty secret SHALL render as a placeholder that discloses its length and nothing else, and an empty secret SHALL render as a literal unset marker so the two cases are distinguishable without either revealing a value.

#### Scenario: A populated secret renders without any of its characters

- **WHEN** a secret is rendered
- **THEN** the result is a placeholder of the form `<redacted:N bytes>` where `N` is the secret's length, and none of the secret's own characters appear

#### Scenario: An unset secret renders as the unset marker

- **WHEN** an empty secret is rendered
- **THEN** the result is the literal unset marker

#### Scenario: Unset and set are distinguishable

- **WHEN** the same secret is rendered once empty and once populated
- **THEN** the two renderings differ

#### Scenario: The two secrets are both redacted in the configuration view

- **WHEN** the configuration's log-safe view is produced with a private key and an app secret loaded
- **THEN** neither the private key's contents nor the app secret's contents appear anywhere in it

### Requirement: The string form of the configuration is the redacted form

The value produced by the configuration's string conversion SHALL be the redacted view, so that formatting the configuration with a generic verb cannot disclose a secret.

#### Scenario: Generic string formatting is safe

- **WHEN** the configuration is formatted with the generic string verb
- **THEN** the result is the redacted view, and the private key does not appear in it

#### Scenario: The string form and the redacted view agree

- **WHEN** both the string form and the log-safe view are produced
- **THEN** the string form contains the log-safe view's content and adds no unmasked field

#### Scenario: Non-secret fields are still visible

- **WHEN** the redacted view is produced
- **THEN** the identifier, account, licence, language, timezone, device identifier, gateways, timeout, the dry-run state and the log level are all present, so the view is still useful for diagnosis

### Requirement: An unconfigured app secret is left out of a request, not sent empty

An order request SHALL omit the app secret field entirely when no app secret is configured, and SHALL NOT send it as an empty string, because the provider rejects an empty value in that field as a parameter error.

#### Scenario: No app secret configured means no app secret on the wire

- **WHEN** an order request is built and no app secret is configured
- **THEN** the serialised request contains no app-secret field, rather than an empty one. This is a property of the serialisation of the request body, and the repository's test suite does not assert it directly

#### Scenario: A configured app secret is carried through

- **WHEN** a client configuration is built and an app secret is configured
- **THEN** the constructed client configuration carries that value, and carries it in preference to any value discovered from a stray file or from the library's own environment variables. The corresponding step for the request body is the omission above and is not asserted by the suite
