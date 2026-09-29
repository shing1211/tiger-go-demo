# sdk-coverage Specification

## Purpose

A demonstration built on someone else's client library is only worth as much as the surface it actually used, and an unwired method is either a deliberate exclusion or an oversight — the two are indistinguishable from a coverage number alone. This capability fixes the current coverage figure, fixes that every remaining gap is excluded for a stated structural reason rather than forgotten, and fixes that a rejected request is not a mutation in this API, so the read/mutation split is drawn on account state rather than on the HTTP verb.

## Requirements

### Requirement: The read side of the library is covered

Of the 117 exported client methods this project depends on, 103 SHALL be referenced from command or internal code, and the remaining 14 SHALL be exactly the set listed in the allow-list of the coverage check. A gap that is not on the allow-list, and an allow-list entry that is now covered, SHALL both fail the check.

#### Scenario: A new gap fails the check

- **WHEN** a method loses its last reference and is not on the allow-list
- **THEN** the check fails and prints the method as uncovered, naming the allow-list line to add

#### Scenario: An allow-list entry that gained a call site fails the check

- **WHEN** a method on the allow-list gains a call site
- **THEN** the check fails and prints the stale allow-list line to delete

#### Scenario: The uncovered set is compared, not counted

- **WHEN** the check runs
- **THEN** it compares the actual uncovered set against the allow-list exactly, so a substitution of one method for another fails even though the count is unchanged

#### Scenario: The current state matches the allow-list

- **WHEN** the check runs against the library version this project depends on
- **THEN** it reports 103 of 117 methods covered with 14 uncovered, and the 14 match the allow-list exactly

#### Scenario: A missing library in the module cache fails loudly

- **WHEN** the library this project depends on is not present locally
- **THEN** the check fails and says so, rather than reporting an empty or vacuous result

### Requirement: Every uncovered method is structurally excluded

The 14 uncovered methods SHALL be excluded for a stated reason and no other. Six SHALL be deprecated aliases whose replacements the commands call instead; seven SHALL change account state and would therefore need the write gate in front of them; one SHALL not be an API call at all.

#### Scenario: The six deprecated aliases are unused because their replacements are called

- **WHEN** a command needs a real-time quote, k-lines by period, k-lines by page, an option quote, a warrant quote or a delayed quote
- **THEN** it calls the non-deprecated method, and the six deprecated aliases have no call site anywhere in the project

#### Scenario: The seven account-mutating methods are not wired up

- **WHEN** the project exposes its trading surface
- **THEN** it excludes every method that would change account state, and includes the read side of the corresponding operations wherever the library provides one

#### Scenario: The one method that is not a call is excluded for that reason

- **WHEN** the seventh uncovered trading method is classified
- **THEN** it is a local in-memory assignment on the client rather than a request, so there is nothing to route or to gate

#### Scenario: No uncovered method is a read that was skipped

- **WHEN** the 14 uncovered methods are enumerated
- **THEN** none of them is a plain read, and the read side of each excluded operation is covered

#### Scenario: A deprecated alias gaining a call site is a failure, not progress

- **WHEN** a command starts calling a deprecated alias
- **THEN** the check fails, because the allow-list line that tolerated its absence no longer applies

### Requirement: A rejected request is not a mutation

The split between covered and excluded methods SHALL be drawn on whether a method changes account state, and SHALL NOT be drawn on the HTTP verb it uses. Every method in this library is a POST, including reads that cost a real round trip.

#### Scenario: A read that costs a round trip is still a read

- **WHEN** a method that issues a request but cannot place or change anything is classified
- **THEN** it is classified as a read, on the grounds that it cannot change account state

#### Scenario: The verb carries no signal

- **WHEN** a method is classified as read or account-mutating
- **THEN** the classification is not made from the HTTP verb, because every method in this library uses the same verb

#### Scenario: A method that claims a permission is classified as mutating

- **WHEN** a method claims a market-data permission is classified
- **THEN** it is classified as changing account state rather than as a read

### Requirement: The coverage check is static only

The check SHALL be a static reference search over command and internal code. It proves a method is referenced; it does not prove the method was exercised against a live account, and no artifact may state that it does.

#### Scenario: The check makes no network request

- **WHEN** the check runs
- **THEN** it completes with no credentials configured and issues no request

#### Scenario: A reference is not evidence of a live call

- **WHEN** an artifact reports a covered method
- **THEN** it does not claim the method was exercised against a live account

#### Scenario: Coverage is not described as end-to-end validation

- **WHEN** an artifact states the coverage figure
- **THEN** it does not present that figure as evidence that the corresponding API surface works

### Requirement: The coverage check is a build target

The coverage check SHALL be reachable as a named build target and SHALL be part of the aggregate verification target, so a coverage regression is caught by the same command that catches a test or build regression.

#### Scenario: A coverage regression fails the build

- **WHEN** the uncovered set stops matching the allow-list and the coverage target is run
- **THEN** the target exits non-zero and prints both the new gaps and the now-stale allow-list entries

#### Scenario: The aggregate verification target includes it

- **WHEN** the aggregate verification target is listed
- **THEN** the coverage target appears among its prerequisites
