# Quantum Control API

Status: `v1` alpha subset, current implementation `0.3.0-alpha.1`

Public base default: `http://127.0.0.1:17440`

## Authentication and actor identity

Protected requests use:

```http
Authorization: Bearer <actor-token>
```

An optional actor registry maps bearer-token SHA-256 digests to explicit
`human`, `service` or `tci` identities and fixed roles. The legacy
`QUANTUM_CONTROL_API_TOKEN` remains a service identity for compatibility and
never gains mutation authority.

On loopback only, when neither credential source is configured, Quantum Control
uses `service:loopback-readonly`. It can use the read-only operator surface but
cannot read durable audit, issue confirmations or execute mutations.

Caller-provided JSON `actor` and legacy `confirmation` fields are never trusted.
Quantum Control replaces the actor with the authenticated actor ID and clears
legacy confirmation text before forwarding a typed request to `qcored`.

Clients may optionally send:

```http
X-Quantum-Session-ID: my-session-123
```

The session ID is bound into immutable plans and confirmation grants but is not
an authentication credential.

The internal broker API is transported over a protected Unix socket and uses a
separate `X-Quantum-Broker-Token`. It is not a public API.

## Permission model

Important scopes are:

```text
control.read
inventory.read
operations.catalog.read
operations.plan
operations.execute.readonly
operations.execute.mutate
audit.read
operations.confirm
operations.propose
```

The `mutator` service role receives `operations.execute.mutate`. The human
`approver` role receives `operations.confirm`. These are deliberately separate.
TCI actors may receive proposal/read roles only and cannot receive either
mutation or confirmation authority.

## Public health

### `GET /healthz`

Reports public process liveness.

### `GET /readyz`

Verifies that `qcored` and the in-process operation-plan subsystem are ready.

## Product information

### `GET /v1/control/info`

Requires `control.read` and reports version plus explicit capability flags.

## Read-only component inventory

### `GET /v1/components`

Requires `inventory.read` and returns
`quantum.control/component-inventory/v1alpha1`.

### `GET /v1/components/{id}`

Requires `inventory.read`. Unknown IDs return HTTP 404 and cannot create
arbitrary commands, paths or systemd probes.

Ownership values remain `managed`, `external`, `disabled` and fail-safe
`unknown`.

## Operation catalog

### `GET /v1/operations`

Requires `operations.catalog.read` and returns the current broker allowlist.

Current typed operations include:

```text
system.snapshot
service.status
service.start
service.stop
service.restart
```

The final three are confirmation-required and their current allowed unit list
contains only `quantum-runtime.service`.

## Immutable planning

### `POST /v1/operations/plan`

Requires `operations.plan`.

Example mutation proposal:

```json
{
  "request_id": "optional-client-correlation",
  "action": "service.restart",
  "parameters": {
    "unit": "quantum-runtime.service"
  }
}
```

Quantum Control returns `quantum.control/operation-plan/v1alpha1` containing the
plan ID, SHA-256 digest, authenticated actor, request/session correlation,
canonical parameters, risk, confirmation requirement, validity and time bounds.

The digest changes if actor, action, parameters, correlation or bound policy
metadata change. TCI plans are audited as proposals.

## Read-only execution

### `POST /v1/operations/execute`

Requires `operations.execute.readonly`.

This route remains for read-only operations. A confirmation-required operation
submitted here is rejected by `qcored`; a caller-controlled string cannot
promote it into a mutation.

## Human confirmation

### `POST /v1/confirmations`

Requires the human-only `operations.confirm` permission.

```json
{
  "plan_id": "plan-..."
}
```

The public process looks up the cached plan and forwards the exact plan plus the
authenticated approver credential over the protected broker socket. `qcored`
then independently:

- authenticates the approver
- verifies the human confirmation permission
- revalidates the plan schema, digest, expiry and current operation policy
- rejects self-approval under v1alpha1 policy
- creates one short-lived single-use grant in root-owned state

The raw confirmation token is returned once. Only its SHA-256 digest is stored.

## Approved mutation execution

### `POST /v1/operations/execute-approved`

Requires `operations.execute.mutate`.

```json
{
  "plan_id": "plan-...",
  "confirmation_token": "<single-use token>"
}
```

The public process retrieves the exact cached plan. `qcored` independently
authenticates the mutation executor, verifies mutation permission, revalidates
the plan and current allowlist, then atomically consumes the grant before any
privileged adapter is invoked.

A grant is bound to the plan ID/digest, plan actor, session and action. Exact
normalized parameters are part of the plan digest. Changed actor, session,
action, parameters, expiry or current policy fail closed.

The grant remains consumed after success or failure. A transport failure after
the request has been submitted is reported as an unknown outcome and is never
automatically retried.

## Service mutation operations

### `service.start`

Risk: `low`, confirmation required.

### `service.stop`

Risk: `high`, confirmation required.

### `service.restart`

Risk: `low`, confirmation required.

All three currently accept exactly:

```json
{
  "unit": "quantum-runtime.service"
}
```

The privileged adapter uses only fixed vectors equivalent to:

```text
systemctl start -- quantum-runtime.service
systemctl stop -- quantum-runtime.service
systemctl restart -- quantum-runtime.service
```

No shell is involved. A deployment policy may remove the Runtime unit but may
not add another systemd unit.

Before the action, `qcored` captures service state. Afterward it waits for the
required active/inactive postcondition. Active Runtime postconditions also
require HTTP 200 from the fixed loopback health endpoint. Results include
bounded recovery metadata. See `docs/SERVICE-MUTATIONS.md`.

## Durable audit

### `GET /v1/audit`

Requires `audit.read`.

Optional query parameters:

```text
limit=1..500
actor_id=<exact actor ID>
action=<exact action>
```

### `GET /v1/audit/integrity`

Requires `audit.read` and returns record count, chain head and verification
state.

There is intentionally no public audit mutation endpoint. Mutation records
capture proposal/plan, approval, attempt, final state, stable error code and
recovery/rollback status without storing raw secret-like parameters.

## Convenience reads

### `GET /v1/system/status`

Requires `control.read` and executes `system.snapshot`.

### `GET /v1/services/{unit}`

Requires `control.read` and executes read-only `service.status` after broker-side
unit validation.

## Explicitly unsupported write surface

```text
quantum-control self-restart: unsupported
Ollama mutation:               unsupported
arbitrary systemd units:       unsupported
domain/reverse proxy changes:  unsupported
TLS changes:                   unsupported
database changes:              unsupported
package changes:               unsupported
container lifecycle changes:   unsupported
shell execution:               permanently unsupported
```
