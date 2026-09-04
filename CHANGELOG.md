# Changelog

## 0.3.0-alpha.1

First confirmation-gated privileged service mutation milestone.

### Added

- typed `service.start`, `service.stop` and `service.restart` operations
- compiled mutation allowlist initially limited to `quantum-runtime.service`
- optional deployment policy that can narrow but never broaden the compiled service allowlist
- dedicated `operations.execute.mutate` permission and `mutator` service role, separate from human approval authority
- root-owned `qcored` confirmation-grant state under `/var/lib/quantum-control-broker`
- broker-side actor authentication for human approvals and mutation execution
- broker-side revalidation of immutable plan schema, digest, correlation, expiry, risk and exact normalized parameters
- single-use grant consumption before the privileged action
- fixed `systemctl <verb> -- <unit>` execution without a shell
- precondition and postcondition service-state capture
- bounded Quantum Runtime loopback health verification for active postconditions
- deterministic transaction timeout and service polling
- one defined recovery attempt toward the observed precondition when a mutation fails
- public `POST /v1/operations/execute-approved` route gated by mutation permission
- service-mutation policy schema and configuration example
- tests for replay, actor/session/action/parameter tampering, stale plans, arbitrary-unit rejection, TCI denial and recovery behavior

### Security posture

- the TCI may still inspect and propose but cannot approve or execute mutations
- human approvers do not automatically receive mutation-executor authority
- ordinary read-only execution cannot satisfy a confirmation-required action with a caller-controlled string
- the privileged broker independently authenticates the approver and executor instead of trusting the public API result
- the grant remains consumed after success or failure so an interrupted or failed action cannot be blindly replayed
- `quantum-control.service`, Ollama, Apache, databases and arbitrary systemd units remain outside the mutation allowlist
- no shell, package, domain, TLS, database or container mutation is introduced

## 0.2.0-alpha.2

Pre-mutation identity, authorization, plan, confirmation and durable audit foundation.

### Added

- versioned actor, immutable operation-plan, confirmation-grant and audit-record contracts
- authenticated actor kinds for human, service and future TCI clients
- fixed roles expanded into explicit server-side permission scopes
- optional actor registry containing SHA-256 bearer-token digests instead of raw tokens
- immutable time-limited operation plans with canonical SHA-256 digests over exact actor/action/parameter/correlation/risk state
- durable single-use confirmation grants bound to one plan digest, subject actor and action
- mandatory human approver identity for confirmation grants
- append-only JSONL audit with sequence numbers, hash-chain integrity and startup verification
- permission-scoped `GET /v1/audit` and `GET /v1/audit/integrity`
- `POST /v1/confirmations` for eligible cached confirmation-required plans
- optional validated `X-Quantum-Session-ID` correlation
- systemd protected `StateDirectory=quantum-control`
- security threat-model, audit retention/export/recovery and contract documentation
- actor registry example with deliberately unusable placeholder token digests

### Security posture

- TCI actors may read permitted state and create proposals but cannot execute current operations or mint confirmations
- caller-supplied actor fields are overwritten by authenticated identity
- secret-like operation parameters are redacted from durable audit
- raw confirmation tokens are never persisted, only SHA-256 digests
- grant replay remains rejected after process restart
- audit tampering causes startup verification to fail closed
- confirmation-required broker operations cannot execute until a structured grant verifier is explicitly wired into `qcored`
- arbitrary legacy `confirmation` strings no longer satisfy a confirmation-required broker boundary
- no mutating service, domain, package, database or shell operation is introduced

## 0.2.0-alpha.1

First read-only component inventory and adoption foundation.

### Added

- versioned `quantum.control/component-inventory/v1alpha1` schema
- authenticated `GET /v1/components` and `GET /v1/components/{id}` API routes
- fixed read-only probes for KeyHelp, Apache, Nginx, PHP/PHP-FPM, MariaDB/MySQL, PostgreSQL, Docker/Podman, Ollama, Quantum Runtime, SearXNG, Ember CoreUI and the STΛRLIGHT UNIT Game/Repack
- deterministic `managed`, `external`, `disabled` and fail-safe `unknown` ownership states
- service, filesystem, version, capability, health and evidence reporting
- explicit empty listener surface until ports can be mapped safely instead of guessed
- bounded version-file reads and secret-like metadata suppression
- fixtures for a clean host, a KeyHelp host and a partial Starlight stack
- stale managed-marker handling that fails to `unknown` rather than claiming ownership
- API tests proving bearer authentication remains enforced on inventory routes

### Security posture

- inventory performs no service restart, package installation or configuration mutation
- no caller-provided text is passed to a shell
- all command names, arguments, paths, globs and unit probes are fixed in code
- failed or ambiguous observations produce `unknown` rather than optimistic ownership claims
- raw command failures are not exposed in public inventory responses
- externally owned services remain observation-only

## 0.1.0-alpha.1

Initial executable Quantum Control foundation.

### Added

- separate unprivileged `quantum-control` API and privileged `qcored` broker
- protected Unix-socket transport with a mandatory broker token
- loopback-only public API default and mandatory bearer authentication for every non-loopback bind
- typed operation catalog, planning and execution protocol
- read-only `system.snapshot` and `service.status` operations
- strict systemd unit validation and fixed `systemctl show` invocation
- request and audit identifiers
- structured health, readiness and capability endpoints
- stable handling of semantic operation rejection across the broker boundary
- systemd service hardening examples
- OpenAPI, architecture, API, security and roadmap documentation
- formatting, vet, race, build and live two-process verification

### Licensing

- adopted the Starlight Unit Studios Quantum Control Community Source License 1.0
- explicitly permitted commercial hosting and managed-service operation while prohibiting sale and white-label resale of Quantum Control itself
- added controlling German and translated English license texts
- added license history, copyright, notice, trademark and third-party notice files
- added CI verification that required legal files are present and internally consistent

### Security posture

- no arbitrary command or shell operation
- no mutating administrative operation
- no unauthenticated remote-listener override
- no secrets in source configuration examples
- public broker failures do not expose internal socket paths or transport details

### Not yet implemented

- domains, reverse proxy and TLS
- databases and containers
- backups, restore and updates
- web interface
- Quantum Runtime and CoreOS adapters
