# Quantum Control Security Baseline

Status: `0.1.0-alpha.1`

Quantum Control is intended to become a system administration platform. Its security boundary is therefore established before any mutating feature is added.

## Process separation

```text
client or TCI
    |
quantum-control
    unprivileged public API
    |
protected Unix socket and broker token
    |
qcored
    narrowly privileged typed-operation broker
    |
fixed allowlisted adapters
```

The public service never receives direct root access. `qcored` never accepts command lines, scripts or arbitrary executable names.

## Current controls

- the public API binds to `127.0.0.1:17440` by default
- every non-loopback bind requires a bearer token of at least 32 characters
- no environment variable can bypass remote authentication
- the broker requires a separate token of at least 32 characters
- the broker listens only on a protected Unix socket
- an existing regular file is never replaced by the socket listener
- only registered typed operations can execute
- unexpected actions and parameters are rejected
- `service.status` accepts a strict unit identifier and uses a fixed `systemctl show` argument vector
- request bodies and broker responses are bounded
- public transport errors do not reveal internal broker paths or credentials
- logs contain request metadata and operation audit metadata, not bearer or broker tokens
- all implemented operations are read-only

## Token storage

For packaged installations, create the broker token outside the repository and store it as:

```text
/etc/quantum-control/broker.token
owner: root
 group: quantum-control
 mode: 0640
```

The public API token belongs in a protected local environment file or credential facility. It must never be committed, included in a release archive, placed in screenshots or printed by diagnostics.

## Mutation gate

No write operation may enter the registry until all of the following exist:

1. an explicit typed action and bounded schema
2. authenticated user identity and role policy above the broker token
3. plan output describing the exact intended change
4. confirmation rules appropriate to the risk class
5. precondition checks
6. deterministic adapter execution without a shell
7. postcondition health verification
8. durable audit storage
9. rollback or a documented non-rollbackable boundary
10. dedicated tests for rejection, interruption and partial failure

A generic `shell.exec` action is a permanent non-goal.

## TCI boundary

A future Terran Cognitive Intelligence may read scoped status, explain a plan and propose an allowlisted action. It may not bypass authentication, confirmation, risk policy or the broker. Model text is untrusted data and is never interpreted as a command.

## Reporting

Do not publish live credentials or exploitable private deployment details in a public issue. Use the official Starlight Unit Studios contact channel for sensitive reports until a dedicated security address is documented.
