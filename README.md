# Quantum Control

Quantum Control is the standalone Linux and server administration platform of
Starlight Unit Studios. It is planned as the reusable KeyHelp replacement for
the Starlight stack and, later, as a native module of Quantum CoreOS.

Current version: `0.2.0-alpha.1`

## Project boundary

Quantum Control is an independent product with its own repository, installer,
release cycle and API contract. It must work on supported existing Linux
systems before Quantum CoreOS is built.

Quantum CoreOS will consume released Quantum Control packages and provide
OS-specific policy, packaging and shell integration. CoreOS will not duplicate
Control logic or maintain a private fork.

## Current alpha

The security boundary remains:

```text
browser / API / TCI
        |
quantum-control
unprivileged public service
        |
protected Unix socket
        |
qcored
privileged typed-operation broker
        |
fixed allowlisted system adapters
```

The alpha currently provides:

- separate `quantum-control` and `qcored` processes
- protected Unix-socket broker transport and mandatory broker token
- loopback-first public API with bearer authentication for remote exposure
- typed operation catalog, planning and read-only execution
- read-only `system.snapshot` and `service.status` operations
- versioned read-only component inventory `v1alpha1`
- authenticated `/v1/components` and `/v1/components/{id}` endpoints
- fixed probes for KeyHelp, web servers, PHP, databases, container runtimes, Ollama, Quantum Runtime, SearXNG, Ember CoreUI and the STΛRLIGHT UNIT Game/Repack
- deterministic `managed`, `external`, `disabled` and fail-safe `unknown` ownership states
- bounded detection evidence, version filtering and health reporting
- no guessed listener ports: unknown mappings remain an empty listener array until safe socket attribution is implemented
- request correlation IDs, audit identifiers and structured results
- systemd hardening examples
- fixtures, race tests and CI

Mutating operations are intentionally absent. Domains, TLS, databases,
containers, backups and updates will be added only as separately reviewed typed
operations after the identity, authorization and durable audit gate is complete.

## Quick start

Requirements:

- Linux or another compatible Unix-like development environment
- Go 1.23 or newer to build
- systemd for the current `service.status` and service-state inventory probes

Create a local broker token:

```bash
export QUANTUM_CONTROL_BROKER_TOKEN="$(openssl rand -hex 32)"
```

Build and start the broker first:

```bash
go test ./...
go build -o qcored ./cmd/qcored
go build -o quantum-control ./cmd/quantum-control

export QUANTUM_CONTROL_BROKER_SOCKET=/tmp/quantum-control-qcored.sock
./qcored serve
```

In a second terminal, reuse the same token and socket:

```bash
export QUANTUM_CONTROL_BROKER_TOKEN='<same token>'
export QUANTUM_CONTROL_BROKER_SOCKET=/tmp/quantum-control-qcored.sock
./quantum-control serve
```

The public API listens on `127.0.0.1:17440` by default.

```bash
curl http://127.0.0.1:17440/healthz
curl http://127.0.0.1:17440/readyz
curl http://127.0.0.1:17440/v1/system/status
curl http://127.0.0.1:17440/v1/services/quantum-runtime.service
curl http://127.0.0.1:17440/v1/components
curl http://127.0.0.1:17440/v1/components/quantum-runtime
```

## Read-only adoption inventory

The inventory is designed for existing KeyHelp servers, ordinary Linux hosts
and the Starlight stack before Quantum Control owns anything on the machine.
A detected component is normally `external`. Quantum ownership is claimed only
when an explicit managed marker and separate runtime evidence agree.

Ambiguous or inaccessible evidence becomes `unknown`, never an optimistic
`managed` or `disabled` result. Externally owned components are not modified.

The machine-readable schema is:

```text
schema/component-inventory-v1alpha1.schema.json
```

## Security rule

Quantum Control never accepts model output, user text or API text as a shell
command. Every administrative request must map to a named allowlisted action
with individually validated parameters. Component inventory uses only fixed
command names, fixed argument vectors, fixed paths and fixed service probes.

For example:

```json
{
  "action": "service.status",
  "parameters": {
    "unit": "quantum-runtime.service"
  }
}
```

There is no `shell.exec` operation.

## Configuration

See:

- `config/quantum-control.env.example`
- `config/qcored.env.example`

For production, both services read the same root-owned broker token file. The
file should be owned by `root:quantum-control` with mode `0640`. A configured
public API token must contain at least 32 characters.

## Commands

```text
quantum-control serve
quantum-control version
quantum-control check-config

qcored serve
qcored version
qcored check-config
qcored catalog
```

## Development

```bash
make check
```

The check target validates required legal files, version consistency,
formatting, vetting, race-enabled tests and both production binaries.

## Documentation

- `docs/ARCHITECTURE.md`
- `docs/API.md`
- `docs/ROADMAP.md`
- `docs/DEPLOYMENT.md`
- `docs/SECURITY.md`
- `docs/LICENSE-POLICY.md`
- `docs/adr/0001-privileged-broker-boundary.md`
- `docs/adr/0002-read-only-first.md`
- `docs/adr/0003-no-unauthenticated-remote-mode.md`
- `api/openapi.yaml`

## License

Quantum Control project-owned code is licensed under the **Starlight Unit Studios Quantum Control Community Source License 1.0**.

- private and internal use is royalty-free
- commercial hosting and managed-service operation are expressly permitted
- customers may receive authenticated access limited to resources provided or managed for them
- there is no user, customer, domain, server, or instance limit and no license-enforcement telemetry requirement
- distributed modifications must retain attribution, provide corresponding source code, and use the same license
- Quantum Control itself may not be sold, sublicensed, white-labeled, or offered as a standalone paid control-panel, SaaS, or general Control API product
- installation, administration, maintenance, consulting, support, hosting, hardware, compute, storage, network, and backup charges remain permitted under the license conditions
- managed and bundled third-party components retain their own terms

The legally controlling German text is in `LICENSE.de.md`. `LICENSE.md` is an English convenience translation. See also `LICENSE_HISTORY.md`, `NOTICE.md`, `COPYRIGHT.md`, `TRADEMARKS.md`, and `THIRD_PARTY_NOTICES.md`.

This is a custom Source Available license and is not an OSI-approved open-source license.
