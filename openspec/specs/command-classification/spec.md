# command-classification Specification

## Purpose

This project has no paper-trading mode: every order it can send goes to a real, funded brokerage account. The property that makes it safe to run any of its commands is therefore structural rather than procedural — the read-only commands have no code in them that *could* place an order, and the one command that can is behind two independent gates. This capability fixes how each command is classified, fixes that the classification is complete in both directions, and fixes that a classification is a deliberate statement about a named directory rather than the absence of one.

## Requirements

### Requirement: Every command is classified, in both directions

Every directory directly under `cmd/` SHALL appear in exactly one of two lists — read-only or write — and every name in either list SHALL have a directory behind it. Both directions are load-bearing and both are silent when broken: a classified name with no directory says a command is read-only when there is no such command, and a directory with no classification is a command nobody has looked at.

#### Scenario: A classified command with no directory fails

- **WHEN** the read-only or write list names a command and `cmd/` has no such directory
- **THEN** the check fails, because a classification with no command behind it is a false safety claim and the next person to add one inherits it for free

#### Scenario: A directory with no classification fails

- **WHEN** a directory exists under `cmd/` and appears in neither list
- **THEN** the check fails, because an unclassified command defaults to being nobody's problem and nobody has decided whether it is safe

#### Scenario: A name in both lists fails

- **WHEN** a command appears as both read-only and write
- **THEN** the check fails, because a command classified as safe and unsafe at once satisfies neither reader

### Requirement: Exactly one command writes, and it is the trading command

The write list SHALL contain exactly one entry and it SHALL be `trade`. A second write command is not a detail: it means the write gate's blast radius grew, which changes the safety argument the whole project rests on.

#### Scenario: A second write command fails

- **WHEN** the write list names anything other than exactly `trade`
- **THEN** the check fails, and the failure says the safety argument needs arguing again rather than reporting a count

#### Scenario: A stale write entry fails

- **WHEN** the write list names `trade` and `cmd/trade` does not exist
- **THEN** the check fails, for the same reason a stale read-only entry does

### Requirement: A read-only command cannot reach an order write

A command classified read-only SHALL NOT import the SDK's trade package, and SHALL NOT name any of that package's order methods as an identifier in its code. Reaching an order method requires importing the package, so the import is the boundary, and the identifier check catches a method reached without the import. Comments and string literals SHALL be skipped, so documentation that must name these methods to explain why they are absent does not fail the check — the gate is about code.

#### Scenario: A read-only command importing the trade package fails

- **WHEN** any file in a read-only command imports the SDK's trade package
- **THEN** the check fails, because that import is the only route to an order method

#### Scenario: A read-only command naming an order method fails

- **WHEN** any file in a read-only command uses an order method name as an identifier
- **THEN** the check fails, so an order method cannot be reached without the import appearing

#### Scenario: Documentation naming an order method does not fail

- **WHEN** a read-only command names an order method only in a comment or a string literal
- **THEN** the check passes, because the gate is about what the program can do, not about what its prose may discuss

#### Scenario: The classification is the only thing that permits a trade import

- **WHEN** a directory under `cmd/` imports the trade package and is not in the write list
- **THEN** the check fails, naming the directory, because a trade import outside the write list is a write path nobody gated

### Requirement: Classification is deliberate, never inherited

A command's classification SHALL be a reviewed statement, so moving a command between the lists SHALL require an explicit edit that fails loudly until the consequences are addressed. A command added to the read-only list inherits a safety claim it has never been examined for; a command moved to the write list gains an ungated write path.

#### Scenario: Moving the token command to the write list fails

- **WHEN** the token command appears in the write list
- **THEN** the check fails and says why: it authenticates and reports, and it never places, modifies or cancels an order, so giving it a write path requires the write gate and that is a change of behaviour rather than a reclassification

#### Scenario: The token command is classified read-only

- **WHEN** the lists are read
- **THEN** the token command appears in the read-only list, and the classification is backed by the same import and identifier checks as every other entry

### Requirement: A read-only command cannot produce the refusal status

A command classified read-only SHALL NOT contain a branch producing exit status `3`, because it has no write path to refuse. The shared read-only plumbing does define `3`, so the status is reachable in the code; what is unreachable is any route to it from a command that never asks to write.

#### Scenario: A read-only command containing the refusal status fails

- **WHEN** any file in a read-only command names the refusal exit status
- **THEN** the check fails, so a command that cannot write cannot claim to have refused

#### Scenario: Only the write command can refuse

- **WHEN** a guard refusal occurs anywhere in this project
- **THEN** it occurs in the trading command, whose three write operations are the only writes in the project

### Requirement: The classification check is not vacuous

The check SHALL object to each of its own failure modes, proven against a synthetic command tree rather than assumed. A check that stops complaining is measuring nothing, and the failure is silent.

#### Scenario: A synthetic tree produces a complaint for each defect

- **WHEN** the check is pointed at a tree containing a correctly classified command, a stale classification, and two unclassified directories
- **THEN** it produces a distinct complaint for the stale entry and for each unclassified directory, and no complaint for the correct one

#### Scenario: A fully classified tree is silent

- **WHEN** the check is pointed at a tree in which every directory is classified and no entry is stale
- **THEN** it produces no complaint at all, or the control above would pass for the wrong reason
