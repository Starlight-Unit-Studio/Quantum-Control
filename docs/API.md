# Quantum Control API

Status: `v1` alpha subset

Public base default: `http://127.0.0.1:17440`

When `QUANTUM_CONTROL_API_TOKEN` is configured, it must contain at least 32 characters and protected requests require:

```http
Authorization: Bearer <token>
```

The internal broker API is transported over a Unix socket and requires the
`X-Quantum-Broker-Token` service credential. It is not a public API.

## Public health

### `GET /healthz`

Reports public process liveness.

### `GET /readyz`

Verifies that `qcored` is reachable.

## Product information

### `GET /v1/control/info`

Reports version and explicit capability flags.

## Operation catalog

### `GET /v1/operations`

Returns the current allowlist and metadata for each operation.

## Planning

### `POST /v1/operations/plan`

Validates an operation without executing it.

```json
{
  "request_id": "optional-client-correlation",
  "action": "service.status",
  "parameters": {
    "unit": "quantum-runtime.service"
  }
}
```

The public service overwrites the `actor` field with its authenticated service
identity. Future releases will attach an authenticated user/session identity.

## Execution

### `POST /v1/operations/execute`

Executes only an implemented allowlisted operation. In `0.1.0-alpha.1`, every
implemented operation is read-only.

The response contains status, risk, timestamps and an audit identifier. Semantic rejection returns HTTP 400, an operation failure returns HTTP 500 and completed operations return HTTP 200.

## Convenience reads

### `GET /v1/system/status`

Executes `system.snapshot`.

### `GET /v1/services/{unit}`

Executes `service.status` after broker-side unit validation.

## Current operations

### `system.snapshot`

Parameters: none.

### `service.status`

Parameters:

```json
{
  "unit": "quantum-runtime.service"
}
```

The unit must match a strict identifier policy. It is passed to a fixed
`systemctl show` argument vector and never to a shell.
