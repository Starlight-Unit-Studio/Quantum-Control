# Quantum Control Security Contracts v1alpha1

This document defines the application-level authorization contracts that must exist before Quantum Control gains a mutating broker operation.

## Actor registry

Schema:

```text
schema/actor-registry-v1alpha1.schema.json
```

The registry identifies `human`, `service` and `tci` actors. Credentials are stored as SHA-256 bearer-token digests rather than raw tokens.

Generate a new token and its digest outside the repository, for example:

```bash
TOKEN="$(openssl rand -hex 32)"
printf '%s' "$TOKEN" | sha256sum
```

Store the raw token in the appropriate protected client credential facility and only the digest in the actor registry.

`config/actors.example.json` contains deliberately unusable placeholder hashes. They must be replaced before use.

## Roles

The v1alpha1 role set is fixed in code:

| Role | Purpose |
|---|---|
| `reader` | system/control metadata and inventory reads |
| `operator` | read access plus planning and current read-only execution |
| `auditor` | control metadata and durable audit reads |
| `approver` | operator/auditor abilities plus human confirmation authority |
| `tci-proposer` | scoped reads and plan/proposal creation only |
| `service` | compatibility/integration service access to current read-only APIs |

Clients do not submit trusted permissions. The server derives permissions from the authenticated actor's configured roles.

TCI actors are explicitly restricted to `tci-proposer` and `reader` roles.

## Operation plans

Schema:

```text
schema/operation-plan-v1alpha1.schema.json
```

An operation plan is a time-limited review snapshot generated after `qcored` validates an allowlisted operation. The public server computes a canonical SHA-256 digest over the exact actor, action, parameters, risk and correlation metadata.

The plan cache is intentionally ephemeral. A restart invalidates unexecuted plans and forces a new review rather than silently restoring old intent.

## Confirmation grants

Schema:

```text
schema/confirmation-grant-v1alpha1.schema.json
```

The grant store persists single-use approval state. Raw grant tokens are returned once and only their SHA-256 digests are stored.

A grant is valid only when all bound values still match:

```text
plan ID
plan digest
subject actor
exact action
expiry
unused state
```

The v1alpha1 grant store can issue and consume these objects, but `qcored` deliberately refuses every confirmation-required execution until a broker-side verifier is integrated with the first reviewed mutating operation.

This prevents an untrusted API client or TCI from turning a free-form `confirmation` string into authority.

## Audit records

Schema:

```text
schema/audit-record-v1alpha1.schema.json
```

Plans/proposals, confirmation issuance and read-only execution outcomes can be recorded with authenticated actor identity. Audit persistence and recovery rules are documented in `docs/AUDIT.md`.

## Session correlation

Clients may provide:

```http
X-Quantum-Session-ID: <validated identifier>
```

If omitted, Quantum Control derives a session correlation value from the generated request ID. Session IDs are correlation metadata, never authentication credentials.

## Current mutation state

```text
mutation operations:              NONE
shell execution:                  NONE
confirmation-required execution:  FAIL CLOSED
TCI confirmation permission:      IMPOSSIBLE BY ROLE POLICY
```

The security contracts are infrastructure for later reviewed mutations, not evidence that write operations already exist.
