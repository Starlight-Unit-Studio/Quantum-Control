# Quantum Control Security Baseline

Status: `0.2.0-alpha.2`

Quantum Control is intended to become a system administration platform. Its security boundary is established before any mutating feature is added.

## Process separation

```text
human / service / future TCI
            |
     authenticated actor
            |
     permission policy
            |
     quantum-control
     unprivileged API
            |
     immutable plan + audit
            |
 protected Unix socket + broker token
            |
          qcored
 privileged typed-operation broker
            |
 fixed allowlisted adapters
```

The public service never receives direct root access. `qcored` never accepts command lines, scripts or arbitrary executable names.

## Identity and authorization

Quantum Control distinguishes three actor kinds:

- `human`
- `service`
- `tci`

Roles expand into fixed permission scopes in code. A caller cannot send a role or permission in an API request and have it trusted.

The optional actor registry stores only SHA-256 bearer-token digests. Raw actor tokens must never be placed in the registry, repository, release archive, audit log or diagnostics.

When the API is loopback-only and no actor credential source is configured, the bootstrap actor `service:loopback-readonly` may use the existing read-only inventory and broker operations. It has no audit-read or confirmation permission and this bootstrap mode is never sufficient for a non-loopback listener.

The legacy single API token remains supported as a service identity for compatibility. It does not receive human confirmation authority.

## TCI boundary

The future Terran Cognitive Intelligence is represented by actor kind `tci`.

A TCI actor may receive only the `tci-proposer` and/or `reader` roles. It may:

- inspect permitted state
- inspect the operation catalog
- create an immutable proposal/plan
- explain the proposed plan to a human

It may not:

- receive `operations.confirm`
- mint its own confirmation grant
- execute the current read-only operation endpoint
- manufacture a human identity
- modify actor credentials
- bypass plan expiry or plan digest checks
- pass model text to a shell

Model output is always untrusted data.

## Immutable plan contract

Planning produces a separate `quantum.control/operation-plan/v1alpha1` object.

Its SHA-256 digest binds:

- plan ID
- HTTP request and session correlation
- actor ID and actor kind
- exact normalized action
- exact parameter names and values in canonical order
- risk class
- confirmation requirement
- validation status
- creation and expiry time
- stable rejection code when invalid

Changing the actor, action, parameters or bound metadata invalidates the digest.

Plans expire. The current default is five minutes and configuration is capped at fifteen minutes.

## Confirmation grant contract

Confirmation grants are deliberately separate from operation plans.

A grant is:

- issued only by an authenticated `human` actor with `operations.confirm`
- bound to one plan ID and digest
- bound to the subject actor and exact action
- short-lived, with a maximum configured TTL of fifteen minutes
- backed by a random token whose raw value is returned only to the caller
- stored on disk only as a SHA-256 token digest
- single-use and durably marked consumed

The v1alpha1 policy also requires the human approver to be different from the plan's subject actor. This is intentionally stricter than the minimum requirement and can be revisited only through an explicit policy change.

A TCI can never issue a grant.

### Fail-closed broker boundary

`qcored` does not yet consume confirmation grants because there are no mutating operations.

Any future operation marked `requires_confirmation` is currently rejected at execution with `confirmation_verifier_required`, even if a caller sends an arbitrary legacy `confirmation` string. The first mutating operation may be introduced only together with the broker-side structured grant verifier.

## Durable audit

Audit records use `quantum.control/audit-record/v1alpha1` and are written as append-only JSON Lines.

Each record contains sequence metadata and a SHA-256 hash of the canonical record plus the previous entry hash. Startup verifies the complete chain. Truncation, reordering, editing or malformed records cause the audit store to fail closed instead of silently repairing history.

Audit records distinguish human, service and TCI actors and include stable request/session/plan correlation where available.

Secrets are redacted by parameter name. Raw bearer tokens, confirmation tokens, passwords, credentials, API keys and private keys must never be stored. Operational failures are recorded by stable error code rather than arbitrary raw exception text.

See `docs/AUDIT.md` for retention, export and recovery rules.

## Current controls

- public API defaults to `127.0.0.1:17440`
- every non-loopback bind requires either the legacy strong API token or an actor registry
- broker authentication is independent from public actor authentication
- broker listens only on a protected Unix socket
- public actor fields supplied in request JSON are overwritten by authenticated identity
- optional `X-Quantum-Session-ID` is syntax-validated and otherwise generated from request correlation
- only registered typed operations can execute
- current broker operations are still read-only
- component inventory uses fixed probes only
- confirmation-required broker execution is fail-closed until a verifier is connected
- security state lives under the protected systemd `StateDirectory=quantum-control`
- request bodies and broker responses are bounded
- public transport errors do not reveal internal broker paths or credentials

## Mutation gate

No write operation may enter the registry until all of the following exist:

1. explicit typed action and bounded parameter schema
2. authenticated actor and permission policy
3. immutable plan output describing the exact intended change
4. risk classification and explicit confirmation policy
5. broker-side verification of a single-use structured confirmation grant when required
6. precondition checks
7. deterministic adapter execution without a shell
8. postcondition health verification
9. durable audit entries for proposal, approval, attempt and result
10. rollback or a documented non-rollbackable boundary
11. dedicated rejection, replay, interruption and partial-failure tests

A generic `shell.exec` action is a permanent non-goal.

## Threat model summary

The security contracts explicitly address:

- prompt injection or malicious model output attempting to become a command
- forged actor fields in API payloads
- stolen or replayed confirmation tokens
- parameter changes after human review
- TCI self-approval
- audit modification after execution
- accidental ownership claims over externally managed services
- unsafe remote exposure without authentication

They do not make a compromised root account trustworthy. Root can replace binaries, configuration and state by definition. Quantum Control protects the boundary between ordinary clients/AI and privileged system administration.

## Recovery

Corrupt actor, grant or audit state is not silently reset.

If audit-chain verification fails, Quantum Control refuses to initialize the durable security layer. An operator must preserve the damaged audit file for investigation and restore a known-good copy or intentionally start a new audit lineage under an explicit future recovery procedure. Automatic truncation is forbidden.

Loss of confirmation-grant state invalidates outstanding approvals. It never causes them to become valid again.

## Reporting

Do not publish live credentials or exploitable private deployment details in a public issue. Use the official Starlight Unit Studios contact channel for sensitive reports until a dedicated security address is documented.
