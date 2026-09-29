# sdk-coverage Specification

## Purpose

A demonstration built on someone else's client library is only worth as much as the surface it actually used, and an unwired method is either a deliberate exclusion or an oversight — the two are indistinguishable from a coverage number alone. This capability fixes the current coverage figure, fixes that every remaining gap is excluded for a stated structural reason rather than forgotten, and fixes that a rejected request is not a mutation in this API, so the read/mutation split is drawn on account state rather than on the HTTP verb.

## Requirements

### Requirement: The whole client surface is scanned

The coverage check SHALL scan every client type the shipped library provides, not a subset of them, and SHALL report the covered and uncovered counts over that whole set. Against the library version this project depends on the scanned surface is 159 methods, of which 140 SHALL be referenced from command or internal code and 19 SHALL remain, and those 19 SHALL be exactly the set listed in the allow-list.

#### Scenario: The current state matches the allow-list

- **WHEN** the check runs against the library version this project depends on
- **THEN** it reports 140 of 159 methods covered with 19 uncovered, and the 19 match the allow-list exactly

#### Scenario: A client type added to the library is scanned

- **WHEN** the library ships a client type the check does not enumerate
- **THEN** that type's methods are outside the denominator, and the reported total says nothing about them

#### Scenario: A missing library in the module cache fails loudly

- **WHEN** the library this project depends on is not present locally
- **THEN** the check fails and says so, rather than reporting an empty or vacuous result

### Requirement: The uncovered set is asserted in both directions

A method that is neither on the allow-list nor covered, and an allow-list entry that is now covered, SHALL both fail the check. Neither direction is optional: the first stops a gap appearing, and the second stops an allow-list line outliving the reason it was written for.

#### Scenario: A new gap fails the check

- **WHEN** a method loses its last reference and is not on the allow-list
- **THEN** the check fails and prints the method as uncovered, naming the allow-list line to add

#### Scenario: An allow-list entry that gained a call site fails the check

- **WHEN** a method on the allow-list gains a call site
- **THEN** the check fails and prints the stale allow-list line to delete

#### Scenario: The uncovered set is compared, not counted

- **WHEN** the check runs
- **THEN** it compares the actual uncovered set against the allow-list exactly, so a substitution of one method for another fails even though the count is unchanged

### Requirement: The scan does not depend on a receiver's variable name

The check SHALL identify each method from its declaration alone, and SHALL NOT assume any particular name for the receiver variable. A check that pins a receiver name matches nothing for any type whose receiver is spelled differently, which removes that type from the denominator without raising anything: the run reports a smaller, greener number and no failure.

#### Scenario: A receiver renamed upstream does not shrink the scan

- **WHEN** the library renames the receiver variable on a method
- **THEN** the method is still counted, because the declaration is matched on its shape rather than on the variable's name

#### Scenario: A pinned receiver name is a silent failure

- **WHEN** a check matches declarations by a hardcoded receiver name instead of by shape
- **THEN** the affected types drop out of the denominator and the run still reports success

### Requirement: Every uncovered method carries one reason from a closed vocabulary

Every allow-list entry SHALL carry exactly one reason, and the reason SHALL be one of: the replacement is called instead, the method would change account state, the method is not a request at all, the method is a user-facing escape hatch this project has not reached yet, or the library's own code calls it. No entry SHALL carry a reason outside that set, and no entry SHALL be left without one.

#### Scenario: An entry without a reason is rejected

- **WHEN** an allow-list entry names a method with no reason attached
- **THEN** the check does not accept it, so a gap cannot be tolerated silently

#### Scenario: A reason outside the vocabulary is rejected

- **WHEN** an allow-list entry carries a word that is not one of the five
- **THEN** the check does not accept it, so the vocabulary cannot grow one entry at a time

#### Scenario: Every reason is a single word

- **WHEN** an allow-list entry is read
- **THEN** its reason is one word from the vocabulary, with no free text after it

### Requirement: The library-internal reason is earned only by a library call site

The reason meaning that the library's own code calls the method SHALL be earned only by a call site inside a package that ships as library code, and SHALL NOT be earned by a call site in an example or demonstration program that happens to live in the same module. A method whose only in-module callers are such programs SHALL carry the escape-hatch reason instead.

#### Scenario: A call site inside a library package earns the reason

- **WHEN** a method has a call site in a package that the library ships for its callers to import
- **THEN** it may carry the reason that the library's own code calls it

#### Scenario: A call site in an example program does not earn the reason

- **WHEN** a method's only in-module callers are example or integration programs
- **THEN** it carries the escape-hatch reason, and no artifact states that the library calls it

#### Scenario: No caller in the module at all is the escape-hatch reason

- **WHEN** a method has no caller anywhere in the module, including its own test files
- **THEN** it carries the escape-hatch reason, not the library-internal one

### Requirement: The three module-contained methods are escape hatches, not library calls

Three methods SHALL be recorded as escape hatches rather than library calls: one raw request method with no caller anywhere in the module, and two token-manipulation methods whose only non-test callers are a manual example program and an integration program shipped inside the module. No artifact may state or imply that the library invokes any of them on its own paths, and that record SHALL survive the fact that this project now calls all three.

#### Scenario: The no-caller method is not attributed to the library

- **WHEN** an artifact classifies the raw request escape hatch
- **THEN** it states that nothing in the module calls it, including the library's own code, and does not describe it as library-internal

#### Scenario: The example-program methods are not attributed to the library

- **WHEN** an artifact classifies the two token-manipulation methods
- **THEN** it states that their only in-module callers are programs shipped for a human to run, and does not describe the library as depending on them

#### Scenario: Covering one of the three does not change the record

- **WHEN** this project calls one of the three and it therefore leaves the allow-list as covered
- **THEN** the artifacts still state that the library does not call it, because coverage is a fact about this project and the library-internal reason is a fact about the library

#### Scenario: Relabelling one of the three is a visible change

- **WHEN** a maintainer moves one of the three from the escape-hatch reason to the library-internal reason
- **THEN** the new reason is a claim the library's source does not support, so the change is a matter of record rather than a silent relabel

### Requirement: Every uncovered method is structurally excluded

The 19 uncovered methods SHALL be excluded for one of the stated reasons and no other: six are deprecated aliases whose replacements the commands call instead, seven would change account state and would therefore need the write gate in front of them, one is not a request at all, one is a user-facing escape hatch this project has not reached yet, and four are called by the library's own code rather than by this project.

#### Scenario: The six deprecated aliases are unused because their replacements are called

- **WHEN** a command needs a real-time quote, k-lines by period, k-lines by page, an option quote, a warrant quote or a delayed quote
- **THEN** it calls the non-deprecated method, and the six deprecated aliases have no call site anywhere in the project

#### Scenario: The seven account-mutating methods are not wired up

- **WHEN** the project exposes its trading surface
- **THEN** it excludes every method that would change account state, and includes the read side of the corresponding operations wherever the library provides one

#### Scenario: The one method that is not a request is excluded for that reason

- **WHEN** the trading client method that is not a request is classified
- **THEN** it is a local in-memory assignment on the client rather than a request, so there is nothing to route or to gate

#### Scenario: The escape hatches are not claims about the library

- **WHEN** the remaining escape-hatch method is enumerated
- **THEN** each is described as something this project has not called, and none is described as something the library calls on its own paths

#### Scenario: No uncovered method is a read that was skipped

- **WHEN** the 19 uncovered methods are enumerated
- **THEN** none of them is a plain read, and the read side of each excluded operation is covered

#### Scenario: A deprecated alias gaining a call site is a failure, not progress

- **WHEN** a command starts calling a deprecated alias
- **THEN** the check fails, because the allow-list line that tolerated its absence no longer applies

### Requirement: A name match is not evidence that the right method ran

The check SHALL match call sites by method name, and a method whose name also belongs to a helper of this project SHALL be recorded as a known false positive rather than as coverage. A green run SHALL NOT be presented as evidence that such a method was exercised, and the limitation SHALL be documented in the project's artifacts rather than left for a reader to discover.

#### Scenario: The name collision is documented, not hidden

- **WHEN** an artifact states the coverage figure
- **THEN** it names the method the name-based match cannot distinguish, and does not describe that method as exercised

#### Scenario: A green run is not evidence of the colliding method

- **WHEN** the check passes
- **THEN** the result is not read as proof that the colliding method was called, because the match that produced it can be satisfied by this project's own helpers

#### Scenario: The collision is currently harmless for a stated reason

- **WHEN** the allow-list is read
- **THEN** the reason the collision does not affect the other entries is that none of their names belongs to a helper in this project

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
