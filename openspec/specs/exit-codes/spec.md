# exit-codes Specification

## Purpose

Every binary in this project terminates with one of four exit statuses, and the status alone has to be enough for a wrapper to tell a completed action from a declined one. This capability fixes what each status means, fixes the two classifications that are easiest to get wrong — a missing credential is not a usage error, and a refusal is not a failure — and records the structural gap in the scheme: every read-only binary cannot produce the refusal status at all, because it has no write path to refuse.

## Requirements

### Requirement: Four distinct exit statuses

Every binary SHALL exit with one of exactly four statuses, and no two meanings SHALL share a status: `0` success, `1` an ordinary failure, `2` missing credentials, `3` a safety refusal.

#### Scenario: Success exits 0

- **WHEN** a command completes without error
- **THEN** the process exits `0`

#### Scenario: An ordinary failure exits 1

- **WHEN** a command fails for a reason that is neither missing credentials nor a safety refusal
- **THEN** the process exits `1` and the failure is reported on standard error

#### Scenario: Missing credentials exit 2

- **WHEN** a command is started with a required credential absent
- **THEN** the process exits `2`

#### Scenario: A safety refusal exits 3

- **WHEN** the write gate refuses a write
- **THEN** the process exits `3`

#### Scenario: The four statuses do not collide

- **WHEN** a nil error, an ordinary error, a missing-credential error and each of the two refusal errors are classified
- **THEN** they map to `0`, `1`, `2` and `3` respectively, with the two refusal kinds both landing on `3`

#### Scenario: The mapping is shared, not reimplemented per binary

- **WHEN** the read-only commands and the trading command are inspected
- **THEN** the outcome-to-status mapping is defined once for the shared read-only plumbing, and the trading command applies the same four values

### Requirement: Missing credentials are the only source of exit 2

Exit `2` SHALL be produced by a missing-credential condition and by nothing else, so that a script reading `2` knows only that the environment needs fixing.

#### Scenario: Every absent credential is named in one report

- **WHEN** no credential is resolvable from the environment
- **THEN** the process exits `2` and the single message names every absent required credential, the portal where each is issued, and that a simulated account will not authenticate

#### Scenario: A trading account is reported separately when it is missing

- **WHEN** the required credentials resolve but the trading account does not, and a trading command is run
- **THEN** the process exits `2` and the message names the trading account as the missing setting

#### Scenario: The read side does not demand an account

- **WHEN** the required credentials resolve but no trading account does, and a read-only command is run
- **THEN** the command proceeds, because none of the endpoints it calls read account state

#### Scenario: A credential problem is not reported for other failures

- **WHEN** a command fails because of an unusable flag, an unparseable value, or an unknown operation
- **THEN** the process does not exit `2`

#### Scenario: An unreadable named config file is not a credential problem

- **WHEN** a config file is named explicitly but cannot be read
- **THEN** the failure is reported as a file error and the process does not exit `2`

### Requirement: Every read-only binary cannot produce the refusal status

Every binary classified read-only — `quote`, `options`, `futures`, `reference`, `corporate`, `push` and `token` — SHALL map failures only to `1` and `2` and SHALL NOT contain a branch producing `3`, because none issues a write and therefore none can reach a refusal. The shared read-only plumbing DOES define `3`, so the status is reachable in the code, but no command that uses it has a write path, and in this project only the trading command can ever refuse. The classification these seven share is specified in `openspec/specs/command-classification/`, and its refusal requirement is what makes this one checkable rather than per-binary.

#### Scenario: No read-only binary has a refusal branch

- **WHEN** the failure-to-status mapping of each of the seven read-only binaries is read
- **THEN** none contains a branch producing `3`, so a guard refusal is unreachable in them. This is a property of their source, not something the test suite exercises

#### Scenario: The shared read-only mapping defines a status those binaries cannot reach

- **WHEN** the shared read-only status mapping is classified against each of the two refusals and a missing-credential error
- **THEN** both refusals map to `3` and the missing-credential error maps to `2`, even though no command using that mapping produces a refusal

#### Scenario: Only the trading command can refuse

- **WHEN** a guard refusal occurs anywhere in this project
- **THEN** it occurs in the trading command, whose three write operations are the only writes in the project
