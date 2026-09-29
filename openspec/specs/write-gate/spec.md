# write-gate Specification

## Purpose

The trading API this project talks to has no paper or sandbox mode: every order it accepts goes to a real, funded account. There is therefore no environment in which a write is harmless, and this capability fixes the two independent conditions that must both hold before a single byte is sent, the fact that neither alone is enough, and the fact that a refusal stops the method call rather than merely reporting on it.

## Requirements

### Requirement: A write needs dry run disabled and an explicit confirmation

A write SHALL be sent only when `TIGER_DRY_RUN` is set to a recognised falsy boolean value and `--confirm-live` is passed on the command line. Both are required. Dry run SHALL default to enabled when the variable is unset, so an absent opt-in is not consent.

#### Scenario: The default refuses a write

- **WHEN** a write is attempted with no flags set and `TIGER_DRY_RUN` unset
- **THEN** it is refused, the process exits `3`, and the message states that no order was sent to the trading provider

#### Scenario: The confirmation flag alone refuses

- **WHEN** `--confirm-live` is passed while `TIGER_DRY_RUN` is unset or enabled
- **THEN** the write is refused, the process exits `3`, and the message names `TIGER_DRY_RUN`

#### Scenario: Dry run disabled alone refuses

- **WHEN** `TIGER_DRY_RUN` is set to a recognised falsy boolean value while `--confirm-live` is absent
- **THEN** the write is refused, the process exits `3`, and the message names `--confirm-live`

#### Scenario: Both conditions open the gate

- **WHEN** `TIGER_DRY_RUN` is set to a recognised falsy boolean value and `--confirm-live` is passed
- **THEN** the gate is open and the write is authorised

#### Scenario: Disabling dry run does not by itself permit a write

- **WHEN** `TIGER_DRY_RUN` is set to a recognised falsy boolean value
- **THEN** a write attempted without `--confirm-live` is still refused

#### Scenario: A per-invocation dry run flag can only tighten the gate

- **WHEN** a run forces dry run regardless of the environment variable
- **THEN** the write is refused even if the environment variable and `--confirm-live` would otherwise permit it

### Requirement: A refusal stops the method call, and the client already exists

A refused write SHALL NOT reach the trading API. The guarantee this project makes and can test is that the trading method is never invoked: the session and its clients are already built before dispatch, so a refusal skips the call and the round trip rather than preventing a client from existing.

#### Scenario: The refused write never calls the trading API

- **WHEN** a write is refused by the gate
- **THEN** no trading method is invoked at all, and the refusal message states that nothing was sent

#### Scenario: A read is not affected by the gate

- **WHEN** a read command is dispatched with dry run enabled and `--confirm-live` absent
- **THEN** it proceeds and invokes exactly the one method it is supposed to

#### Scenario: The refusal shows the request that would have been sent

- **WHEN** a write is refused by the gate
- **THEN** the exact request that would have been sent is printed, framed as a preview and closed by a line stating that nothing was sent

### Requirement: There is no live-mode selector in this project

This project SHALL NOT read a mode, environment, or paper-trading variable. The only conditions that govern a write are `TIGER_DRY_RUN` and `--confirm-live`, and no third switch exists that a reader could mistake for one.

#### Scenario: No mode variable is consulted

- **WHEN** a write is authorised
- **THEN** the only conditions checked are `TIGER_DRY_RUN` and `--confirm-live`

#### Scenario: An unset environment means the default, not a request for input

- **WHEN** the environment contains no trading variables at all and a write is attempted
- **THEN** the write is refused on the grounds that dry run is enabled, and no prompt or advice about choosing a mode is produced

#### Scenario: A simulated account cannot authenticate, and is not a mode here

- **WHEN** a missing-credential report is produced
- **THEN** it states that a simulated account will not authenticate, rather than offering a simulated mode as an alternative
