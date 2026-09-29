# credential-defence Specification

## Purpose

The trading client library accepts credentials from places this project does not configure, and it applies them with a priority high enough to redirect an order to somebody else's account. This capability fixes that all five of those inputs are neutralised — including a bearer-token file that would otherwise authenticate every request — and it fixes the deliberate asymmetry in the feedback: every input that resolves to a file is named in a warning, and the environment variables that carry a value are not, because a file a user does not know exists is the case worth a line of output.

## Requirements

### Requirement: Five unconfigured input sources are neutralised

The client library SHALL be prevented from taking credentials or endpoints from any of five sources this project does not configure, so that no such source can redirect an order, a gateway or an account. The five are: a properties file in the working directory, a properties file in the user's home directory, a token file in the working directory, a pair of token environment variables, and a group of credential environment variables.

#### Scenario: A working-directory properties file cannot redirect anything

- **WHEN** a properties file in the working directory names a different identifier, account, secret, licence, language, timezone, device identifier, trade gateway and quote gateway than the configuration resolved
- **THEN** every one of those values in the constructed client configuration is the resolved one, and specifically the account is unchanged

#### Scenario: A home-directory properties file cannot redirect anything

- **WHEN** a properties file in the user's home directory names a different account
- **THEN** the constructed client configuration still carries the resolved account and identifier, and its trade gateway is still the configured one

#### Scenario: A field this project leaves empty stays empty

- **WHEN** the configuration leaves the trading account, the app secret and the device identifier empty
- **THEN** a properties file in the working directory does not supply any of them, so no account appears that the user never configured

#### Scenario: Credential environment variables cannot redirect anything

- **WHEN** the library's own credential environment variables are set to a different identifier, account, secret and private key
- **THEN** every one of them loses to the resolved configuration

#### Scenario: The neutralisation is not vacuous

- **WHEN** the client library is built with no explicit options at all, on a machine where a properties file is present
- **THEN** it does adopt that file, which is what makes the five inputs above real inputs rather than a hypothetical

### Requirement: The bearer token discovered from a file is cleared

The bearer token the client library discovers SHALL be cleared on the client configuration this project builds, because that token is copied into the authorisation header of every request. A token file this project never wrote would otherwise authenticate the whole session as a different account.

#### Scenario: A token file does not authenticate the session

- **WHEN** a token file in the working directory is present and this project builds a client configuration
- **THEN** the resulting client configuration carries no token, and the token file's contents authenticate nothing

#### Scenario: A token supplied by environment variable does not authenticate the session

- **WHEN** the token environment variable supplies a value
- **THEN** the resulting client configuration still carries no token

#### Scenario: A token redirected to another file does not authenticate the session

- **WHEN** the token file environment variable points at a file outside the working directory that contains a token
- **THEN** the resulting client configuration still carries no token

#### Scenario: The token file is a real discovery path

- **WHEN** the client library is built with explicit credentials and a token file in the working directory
- **THEN** it does pick the token up from that file, which is what makes clearing it load-bearing

### Requirement: Discovered files are named in a warning, and env-var values are not

A properties file or token file found in the working directory, a properties file found in the home directory, and the file the token-file environment variable names SHALL each be named in a warning that states it is being ignored, explains the consequence, and tells the user to delete it. An environment variable carrying a value SHALL NOT be warned about, and that absence SHALL be a recorded gap, not an oversight.

#### Scenario: A file in the working directory is named and explained

- **WHEN** a properties file is present in the working directory
- **THEN** a warning names that file, states that it is being ignored because credentials are loaded only from the environment or a named YAML file, states that it could otherwise redirect orders, and tells the user to delete it

#### Scenario: The token file is named separately

- **WHEN** a token file is present in the working directory
- **THEN** it is named in its own warning, because it is discovered by a separate mechanism and finding the other file says nothing about it

#### Scenario: The home-directory file is named and counted once

- **WHEN** only a properties file in the home directory is present
- **THEN** it is named exactly once, so the user is not sent to delete something that is not there

#### Scenario: A file the environment variable redirects to is named too

- **WHEN** the token-file environment variable names an existing file outside the working directory
- **THEN** a warning names that path and states the file is being ignored, because it is a file the client library would read a token from and the directory scans cannot reach it by construction

#### Scenario: A redirected path is named as it was exported

- **WHEN** the exported path contains redundant elements that normalising a path would rewrite
- **THEN** the warning reproduces the exported string unaltered, because that is the value the client library would open and the only form the user can recognise

#### Scenario: A redirected path that is not a file is silent

- **WHEN** the token-file environment variable names a path that does not exist
- **THEN** no warning is produced, because nothing was discovered to redirect anything and this check is existence-based like the other three

#### Scenario: An empty token-file environment variable is silent

- **WHEN** the token-file environment variable is set to an empty or whitespace-only value
- **THEN** no warning is produced, because the client library falls back to its default token file for such a value and that file is the working-directory one already covered above

#### Scenario: No token value and no file contents reach the output

- **WHEN** any of these warnings is produced
- **THEN** it names paths only, and never the value of the token environment variable nor the contents of a token file

#### Scenario: A missing home directory disables one detection only

- **WHEN** the home directory is unset or does not exist and a properties file is present in the working directory
- **THEN** the working-directory file is still named and no home-directory file is named

#### Scenario: Nothing is printed when nothing is found

- **WHEN** no stray file exists
- **THEN** no warning is produced, in particular none about a home directory that simply has no such file

#### Scenario: A missing working directory is silent

- **WHEN** the directory checked does not exist
- **THEN** no warning is produced and nothing fails

### Requirement: Building a client makes no network request and starts no background work

Constructing a client configuration SHALL resolve every endpoint locally and SHALL open no socket, so a command can fail fast on bad credentials before dialling. The clients this project builds SHALL start no background token-refresh work, and releasing a session SHALL release both of the clients it holds.

#### Scenario: A client is built with no network access

- **WHEN** a client configuration is built with an explicit gateway and dynamic domain lookup disabled
- **THEN** no request leaves the process, dynamic domain lookup is off, and both gateways are resolved locally

#### Scenario: A new session owns no background work

- **WHEN** a session is built
- **THEN** it starts no background loop, so a session that is never released leaks nothing

#### Scenario: Releasing a session releases both clients

- **WHEN** a session holding two clients is released
- **THEN** both clients are released, and releasing twice is harmless
