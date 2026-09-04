# Quantum Control Security Contracts v1alpha1

This document defines the application-level authorization contracts used by the
first reviewed Quantum Control mutation path.

## Actor registry

Schema:

```text
schema/actor-registry-v1alpha1.schema.json
```

The registry identifies `human`, `service` and `tci` actors. Credentials are
stored as SHA-256 bearer-token digests rather than raw tokens.

Generate a new token and its digest outside the repository, for example:

```bash
TOKEN="$(openssl rand -hex 32)"
printf '%s' "$TOKEN" | sha256sum
```

Store the raw token in the appropriate protected client credential facility and
only the digest in the actor registry. `config/actors.example.json` contains
deliberately unusable placeholder hashes.

## Roles

The v1alpha1 role set is fixed in code:

| Role | Purpose |
|---|---|
| `reader` | system/control metadata and inventory reads |
| `operator` | read access plus planning and current read-only execution |
| `mutator` | service identity allowed to submit an already approved mutation |
| `auditor` | control metadata and durable audit reads |
| `approver` | human confirmation authority plus operator/auditor reads |
| `tci-proposer` | scoped reads and plan/proposal creation only |
| `service` | compatibility/integration access to read-only APIs |

Clients do not submit trusted permissions. The server derives permissions from
the authenticated actor's configured roles.

The critical separation is:

```text
approver -> operations.confirm
mutator  -> operations.execute.mutate
```

A human approval does not automatically become mutation execution authority.
TCI actors are explicitly restricted to `tci-proposer` and `reader` roles and
cannot obtain either permission.

## Operation plans

Schema:

```text
schema/operation-plan-v1alpha1.schema.json
```

An operation plan is a time-limited review snapshot generated after `qcored`
validates an allowlisted operation. The public server computes a canonical
SHA-256 digest over the exact actor, action, normalized parameters, risk,
confirmation requirement, validity, request/session correlation and time
bounds.

The plan cache is intentionally ephemeral. A public-process restart invalidates
unexecuted plans and forces a new review.

Before a mutation, `qcored` independently verifies the plan schema and digest,
checks expiry, reconstructs the exact parameters and re-runs the current broker
operation policy. A correctly signed digest cannot make an operation executable
when current broker policy no longer allows it.

## Confirmation grants

Schema:

```text
schema/confirmation-grant-v1alpha1.schema.json
```

The root-owned `qcored` grant store persists single-use approval state. Raw
grant tokens are returned once and only their SHA-256 digests are stored.

A grant is bound to:

```text
plan ID
plan digest
subject actor
session ID
exact action
expiry
unused state
```

Exact normalized operation parameters are already part of the plan digest.
Changing a parameter changes the digest and invalidates the old grant.

The human approver is independently authenticated inside `qcored`. Under the
current policy the approver must be distinct from the plan subject actor.

## Approved execution

`POST /v1/operations/execute-approved` is the only public path for the first
confirmation-required mutations. The public service forwards the cached plan,
single-use token and authenticated mutation-executor credential over the
protected broker socket.

`qcored` then:

1. authenticates the executor independently,
2. rejects TCI actors and actors without `operations.execute.mutate`,
3. revalidates the exact immutable plan and current operation policy,
4. verifies and atomically consumes the matching grant,
5. only then invokes the fixed privileged system adapter.

The grant is consumed before privileged execution and remains consumed whether
the operation succeeds or fails. This prevents replay after a process restart
or ambiguous network response.

The legacy free-form `confirmation` field is still cleared by the public API and
is never accepted as authority.

## Service mutation policy

Schema:

```text
schema/service-mutation-policy-v1alpha1.schema.json
```

The compile-time mutation unit set currently contains only
`quantum-runtime.service`. A deployment policy may narrow that set. It cannot
broaden it.

The privileged adapter uses a fixed executable and one of the fixed lifecycle
verbs. No shell is used. See `docs/SERVICE-MUTATIONS.md`.

## Audit records

Schema:

```text
schema/audit-record-v1alpha1.schema.json
```

Plans/proposals, confirmation issuance, mutation attempts and final results are
recorded with authenticated actor identity. Mutation results also carry the
recovery/rollback status. Audit persistence and recovery rules are documented
in `docs/AUDIT.md`.

## Session correlation

Clients may provide:

```http
X-Quantum-Session-ID: <validated identifier>
```

If omitted, Quantum Control derives a correlation value from the generated
request ID. Session IDs are correlation metadata, never authentication
credentials, but once placed in a plan they are part of the confirmation
binding.

## Current mutation state

```text
service.start:                  quantum-runtime.service only
service.stop:                   quantum-runtime.service only
service.restart:                quantum-runtime.service only
arbitrary systemd mutation:     REJECTED
legacy confirmation strings:    NEVER AUTHORITY
TCI confirmation permission:    IMPOSSIBLE BY ROLE POLICY
TCI mutation permission:        IMPOSSIBLE BY ROLE POLICY
shell execution:                NONE
```
