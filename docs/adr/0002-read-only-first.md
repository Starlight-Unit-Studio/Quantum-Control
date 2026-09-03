# ADR 0002: Begin with read-only operations

Status: accepted for the foundation

## Decision

Version `0.1.0-alpha.1` implements only `system.snapshot` and `service.status`.

## Reasons

- the transport, authentication and operation protocol can be tested before system mutation exists
- rejection behavior can be proven with actions such as `shell.exec`
- audit and confirmation requirements can be designed around real contracts instead of ad hoc commands
- existing KeyHelp and server infrastructure remain untouched

## Consequence

The first alpha is an executable control-plane foundation, not yet a KeyHelp replacement.
