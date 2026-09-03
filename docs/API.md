# Quantum Control API

Status: `v1` alpha subset, current implementation `0.2.0-alpha.2`

Public base default: `http://127.0.0.1:17440`

## Authentication and actor identity

Protected requests use:

```http
Authorization: Bearer <actor-token>
```

An optional actor registry maps bearer-token SHA-256 digests to explicit
`human`, `service` or `tci` identities and fixed roles. The legacy
`QUANTUM_CONTROL_API_TOKEN` remains a service identity for compatibility.

On loopback only, when neither credential source is configured, Quantum Control
uses `service:loopback-readonly`. It can use the existing read-only operator
surface but cannot read durable audit or issue confirmations.

Caller-provided JSON `actor` fields are never trusted. Quantum Control replaces
them with the authenticated actor ID before forwarding a typed request to
`qcored`.

Clients may optionally send a validated correlation header:

```http
X-Quantum-Session-ID: my-session-123
```

It is correlation metadata, not an authentication credential.

The internal broker API is transported over a Unix socket and requires the
separate `X-Quantum-Broker-Token` service credential. It is not a public API.

## Permission model

Important permission scopes include:

```text
control.read
inventory.read
operations.catalog.read
operations.plan
operations.execute.readonly
audit.read
operations.confirm
operations.propose
```

TCI actors may receive proposal/read roles only and cannot receive
`operations.confirm`.

## Public health

### `GET /healthz`

Reports public process liveness.

### `GET /readyz`

Verifies that `qcored` and the in-process security plan subsystem are ready.

## Product information

### `GET /v1/control/info`

Requires `control.read`.

Reports version and explicit capability flags.

## Read-only component inventory

### `GET /v1/components`

Requires `inventory.read`.

Returns a snapshot using schema `quantum.control/component-inventory/v1alpha1`.

### `GET /v1/components/{id}`

Requires `inventory.read`.

Runs only the fixed probe definition for the requested canonical component ID.
Unknown IDs return HTTP 404 and cannot create arbitrary commands, paths or
systemd unit probes.

Ownership values remain `managed`, `external`, `disabled` and fail-safe
`unknown`. Listener arrays remain empty when a port cannot be safely attributed.

## Operation catalog

### `GET /v1/operations`

Requires `operations.catalog.read`.

Returns the current broker allowlist and metadata for each operation.

## Immutable planning

### `POST /v1/operations/plan`

Requires `operations.plan`.

Request example:

```json
{
  "request_id": "optional-client-correlation",
  "action": "service.status",
  "parameters": {
    "unit": "quantum-runtime.service"
  }
}
```

The response is no longer merely the broker validation object. Quantum Control
returns `quantum.control/operation-plan/v1alpha1`, including:

- plan ID
- canonical SHA-256 digest
- authenticated actor
- request/session correlation
- exact canonical parameter list
- risk class
- confirmation requirement
- validity and stable rejection code
- creation and expiry timestamps

The digest changes if actor, action, parameters or bound plan metadata change.

For a TCI actor, a successful planning request is treated as a proposal and is
audited distinctly from a human/service plan.

## Read-only execution

### `POST /v1/operations/execute`

Requires `operations.execute.readonly`.

In `0.2.0-alpha.2`, every implemented broker operation remains read-only.
TCI proposal roles do not include this permission.

The caller's legacy `confirmation` string is cleared by the public service and
cannot become authority. `qcored` also rejects every operation marked
`requires_confirmation` until a structured confirmation-grant verifier is
explicitly connected with a future reviewed mutation.

## Confirmation grants

### `POST /v1/confirmations`

Requires `operations.confirm`, which is available only through a human approver
role in v1alpha1.

```json
{
  "plan_id": "plan-..."
}
```

The referenced plan must still exist, be valid, require confirmation and have a
valid digest. The approver must be an authenticated human and must be distinct
from the subject actor under the current policy.

The response contains the grant metadata plus a random raw token. The raw token
is returned once and never stored. Durable state contains only its SHA-256
digest. Grants are short-lived and single-use.

There are currently no mutating broker operations that consume these grants.
The contract is implemented before mutations on purpose.

## Durable audit

### `GET /v1/audit`

Requires `audit.read`.

Optional query parameters:

```text
limit=1..500
actor_id=<exact actor ID>
action=<exact action>
```

Response contains current integrity metadata plus matching records.

### `GET /v1/audit/integrity`

Requires `audit.read`.

Returns record count, current chain head hash and verification state.

There is intentionally no POST, PUT, PATCH or DELETE audit endpoint.

Durable records store stable error codes and redacted parameters rather than raw
backend errors or secret values.

## Convenience reads

### `GET /v1/system/status`

Requires `control.read` and executes `system.snapshot`.

### `GET /v1/services/{unit}`

Requires `control.read` and executes `service.status` after broker-side unit
validation.

## Current broker operations

### `system.snapshot`

Risk: `read-only`

Parameters: none.

### `service.status`

Risk: `read-only`

Parameters:

```json
{
  "unit": "quantum-runtime.service"
}
```

The unit must match a strict identifier policy. It is passed to a fixed
`systemctl show` argument vector and never to a shell.

## Current write surface

```text
service restart:       not implemented
domain changes:        not implemented
TLS changes:           not implemented
database changes:      not implemented
package changes:       not implemented
shell execution:       permanently unsupported
```
