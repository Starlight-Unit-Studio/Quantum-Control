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

## Read-only component inventory

Introduced in `0.2.0-alpha.1`.

### `GET /v1/components`

Returns a snapshot using schema `quantum.control/component-inventory/v1alpha1`.
Every component record contains a stable ID, version when safely detectable,
ownership, health, service identities, listener array, filesystem roots,
capabilities, bounded evidence, observation time and warnings.

Ownership values are:

- `managed`: an explicit Quantum-managed marker exists and independent runtime evidence confirms the component
- `external`: the component is detected but is not owned by Quantum Control
- `disabled`: no positive evidence exists and the relevant probes completed conclusively
- `unknown`: evidence is ambiguous, inaccessible or internally inconsistent

The initial inventory covers KeyHelp, Apache, Nginx, PHP/PHP-FPM,
MariaDB/MySQL, PostgreSQL, Docker/Podman, Ollama, Quantum Runtime, SearXNG,
Ember CoreUI and the STΛRLIGHT UNIT Game/Repack.

Listener arrays intentionally remain empty when a port cannot be mapped safely
to a component. Quantum Control does not report guessed default ports as
detected state.

### `GET /v1/components/{id}`

Runs only the fixed probe definition for the requested canonical component ID.
Unknown IDs return HTTP 404 and cannot create arbitrary commands, paths or
systemd unit probes.

The inventory layer is strictly read-only. It has no mutation primitive and
never accepts shell text from requests.

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

Executes only an implemented allowlisted operation. In `0.2.0-alpha.1`, every
implemented broker operation remains read-only.

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
