# Quantum Control Security Baseline

Status: `0.3.0-alpha.1`

Quantum Control is intended to become a system administration platform. Its
security boundary is established before broad administrative functionality is
added, and the first reviewed mutation path now exercises that boundary against
one explicitly supported service.

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
 immutable plan + durable audit
            |
 protected Unix socket + broker token
            |
          qcored
 privileged typed-operation broker
            |
 root-owned grants + plan revalidation
            |
 fixed allowlisted adapters
```

The public service never receives direct root access. `qcored` never accepts
command lines, scripts or arbitrary executable names.

The first mutation adapter can start, stop or restart only
`quantum-runtime.service`. It constructs a fixed `systemctl` argument vector and
never invokes a shell.

## Identity and authorization

Quantum Control distinguishes three actor kinds:

- `human`
- `service`
- `tci`

Roles expand into fixed permission scopes in code. A caller cannot send a role
or permission in an API request and have it trusted.

The optional actor registry stores only SHA-256 bearer-token digests. Raw actor
tokens must never be placed in the registry, repository, release archive, audit
log or diagnostics.

When the API is loopback-only and no actor credential source is configured, the
bootstrap actor `service:loopback-readonly` may use read-only inventory and
broker operations. It has no audit-read, confirmation or mutation permission and
this bootstrap mode is never sufficient for a non-loopback listener.

The legacy single API token remains supported as a service identity for
compatibility. It does not receive human confirmation or mutation authority.

## Approval and execution separation

The first mutation flow uses two distinct permissions:

```text
operations.confirm
operations.execute.mutate
```

The human `approver` role receives confirmation authority but not mutation
execution authority. The service `mutator` role may submit an already approved
plan but cannot approve it.

Both identities are authenticated again inside `qcored`. The broker does not
trust the public API merely because it says that a request was approved or
executed by a particular actor.

## TCI boundary

The future Terran Cognitive Intelligence is represented by actor kind `tci`.

A TCI actor may receive only the `tci-proposer` and/or `reader` roles. It may:

- inspect permitted state
- inspect the operation catalog
- create an immutable proposal/plan
- explain the proposed plan to a human

It may not:

- receive `operations.confirm`
- receive `operations.execute.mutate`
- mint its own confirmation grant
- execute the approved-mutation endpoint
- manufacture a human or service executor identity
- modify actor credentials or service mutation policy
- bypass plan expiry, plan digest or postcondition checks
- pass model text to a shell

Model output is always untrusted data.

## Immutable plan contract

Planning produces `quantum.control/operation-plan/v1alpha1`.

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

Plans expire. The default remains five minutes and configuration is capped at
fifteen minutes.

Before approved execution, `qcored` independently verifies the schema and digest,
checks time bounds, reconstructs exact parameters and re-runs current broker
policy. A previously valid plan cannot bypass a later policy restriction.

## Confirmation grant contract

Confirmation grants are separate from operation plans and are now owned by
`qcored`.

A grant is:

- issued only after `qcored` authenticates a `human` actor with `operations.confirm`
- bound to one plan ID and digest
- bound to the plan subject actor, session and exact action
- indirectly bound to exact normalized parameters through the plan digest
- short-lived, with a maximum configured TTL of fifteen minutes
- backed by a random token whose raw value is returned only to the caller
- stored on disk only as a SHA-256 token digest
- single-use and durably marked consumed

The v1alpha1 policy also requires the human approver to be different from the
plan subject actor.

`qcored` consumes the grant before invoking the privileged adapter. The grant
remains consumed whether the action succeeds or fails, so an HTTP retry or
process restart cannot replay the same authorization.

A caller-controlled legacy `confirmation` string is still cleared by the public
service and cannot satisfy this contract.

## Transactional service mutation boundary

Current confirmation-required actions are:

```text
service.start
service.stop
service.restart
```

The compile-time mutation target set contains only:

```text
quantum-runtime.service
```

An optional root-controlled deployment policy may narrow that set. It cannot add
another systemd unit.

The adapter constructs only these forms:

```text
systemctl start -- quantum-runtime.service
systemctl stop -- quantum-runtime.service
systemctl restart -- quantum-runtime.service
```

The action captures a bounded precondition state, performs the one selected
operation and waits for the required postcondition. Active Runtime postconditions
also require HTTP 200 from the fixed loopback health endpoint.

If a transaction fails, recovery is deliberately limited to one defined attempt
toward the observed precondition where policy supports it. Quantum Control does
not blindly repeat the failed mutation.

See `docs/SERVICE-MUTATIONS.md`.

## Durable audit

Audit records use `quantum.control/audit-record/v1alpha1` and are written as
append-only JSON Lines.

Each record contains sequence metadata and a SHA-256 hash of the canonical
record plus the previous entry hash. Startup verifies the complete chain.
Truncation, reordering, editing or malformed records cause the audit store to
fail closed instead of silently repairing history.

Mutation flows distinguish proposal/plan creation, human approval, execution
attempt and final result. Recovery state is recorded through the
`rollback_status` field.

Secrets are redacted by parameter name. Raw bearer tokens, confirmation tokens,
passwords, credentials, API keys and private keys must never be stored.
Operational failures are recorded by stable error code rather than arbitrary raw
exception text.

See `docs/AUDIT.md` for retention, export and recovery rules.

## Current controls

- public API defaults to `127.0.0.1:17440`
- every non-loopback bind requires a credential source
- broker authentication is independent from actor authentication
- broker listens only on a protected Unix socket
- public actor fields supplied in JSON are overwritten by authenticated identity
- optional `X-Quantum-Session-ID` is syntax-validated and bound into plans
- only registered typed operations can execute
- read-only inventory uses fixed probes only
- confirmation grants live in root-owned broker state
- approved service mutations require broker-side revalidation and grant consumption
- mutation unit policy currently contains only `quantum-runtime.service`
- `quantum-control.service`, Ollama, Apache, databases and arbitrary systemd units remain outside the mutation surface
- request bodies and broker responses are bounded
- public transport errors do not reveal broker credentials
- ambiguous post-submission mutation outcomes are not automatically retried
- generic shell execution remains unsupported

## Mutation gate for future actions

Every additional write operation must independently satisfy all of these
requirements before entering the broker registry:

1. explicit typed action and bounded parameter schema
2. authenticated actor and permission policy
3. immutable plan describing the exact intended change
4. risk classification and explicit confirmation policy
5. broker-side verification of a single-use structured grant when required
6. precondition checks
7. deterministic adapter execution without a shell
8. postcondition health verification
9. durable audit entries for proposal, approval, attempt and result
10. rollback or a documented non-rollbackable boundary
11. dedicated rejection, replay, interruption and partial-failure tests

Passing this gate for Runtime service lifecycle control does not automatically
authorize another service or another class of administrative write.

A generic `shell.exec` action is a permanent non-goal.

## Threat model summary

The security contracts explicitly address:

- prompt injection or malicious model output attempting to become a command
- forged actor fields in API payloads
- stolen or replayed confirmation tokens
- parameter changes after human review
- stale plans after policy changes
- TCI self-approval or mutation attempts
- arbitrary systemd unit targeting
- blind retry after ambiguous transport failure
- audit modification after execution
- accidental ownership claims over externally managed services
- unsafe remote exposure without authentication

They do not make a compromised root account trustworthy. Root can replace
binaries, configuration and state by definition. Quantum Control protects the
boundary between ordinary clients/AI and privileged system administration.

## Recovery

Corrupt actor, grant or audit state is not silently reset.

If audit-chain verification fails, Quantum Control refuses to initialize the
durable security layer. An operator must preserve the damaged audit file for
investigation and restore a known-good copy or intentionally begin a new lineage
through a future explicit recovery procedure. Automatic truncation is forbidden.

Loss of confirmation-grant state invalidates outstanding approvals. It never
causes them to become valid again.

For an ambiguous mutation result, inspect current service state and audit data.
If another change is required, create a new plan and obtain a new approval. Do
not replay the original confirmation token.

## Reporting

Do not publish live credentials or exploitable private deployment details in a
public issue. Use the official Starlight Unit Studios contact channel for
sensitive reports until a dedicated security address is documented.
