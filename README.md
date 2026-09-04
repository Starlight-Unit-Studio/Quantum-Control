# Quantum Control

[![DOI](https://zenodo.org/badge/1356058117.svg)](https://doi.org/10.5281/zenodo.22288120)

Quantum Control is the standalone Linux and server administration platform of
Starlight Unit Studios. It is planned as the reusable KeyHelp replacement for
the Starlight stack and, later, as a native module of Quantum CoreOS.

Current version: `0.3.0-alpha.1`

## Project boundary

Quantum Control is an independent product with its own repository, installer,
release cycle and API contract. It must work on supported existing Linux
systems before Quantum CoreOS is built.

Quantum CoreOS will consume released Quantum Control packages and provide
OS-specific policy, packaging and shell integration. CoreOS will not duplicate
Control logic or maintain a private fork.

## Current alpha

The security boundary is:

```text
human / service / future TCI
            |
 authenticated actor + permission policy
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
 root-owned grant verification
            |
 fixed allowlisted system adapters
```

The alpha currently provides:

- separate `quantum-control` and `qcored` processes
- protected Unix-socket broker transport and mandatory broker token
- loopback-first public API and authenticated remote exposure
- actor identities for `human`, `service` and future `tci` clients
- fixed roles and permission scopes derived server-side
- TCI proposal access without execution or confirmation authority
- immutable expiring operation plans with canonical SHA-256 digests
- durable single-use confirmation grants bound to exact plan/actor/session/action state
- root-owned grant creation and consumption inside `qcored`
- separate human approval and service mutation-executor permissions
- append-only hash-chained durable audit with startup integrity verification
- read-only permission-scoped audit API with no audit mutation endpoints
- typed operation catalog, planning and read-only execution
- read-only `system.snapshot` and `service.status` operations
- confirmation-gated `service.start`, `service.stop` and `service.restart`
- compiled mutation target currently limited to `quantum-runtime.service`
- fixed direct `systemctl` argument vectors with no shell
- service precondition/postcondition capture, Runtime health verification and bounded recovery
- versioned read-only component inventory `v1alpha1`
- authenticated `/v1/components` and `/v1/components/{id}` endpoints
- fixed probes for KeyHelp, web servers, PHP, databases, container runtimes, Ollama, Quantum Runtime, SearXNG, Ember CoreUI and the STΛRLIGHT UNIT Game/Repack
- deterministic `managed`, `external`, `disabled` and fail-safe `unknown` ownership states
- bounded detection evidence, version filtering and health reporting
- no guessed listener ports
- systemd hardening and separated persistent state directories
- fixtures, race tests and release-package CI

The service mutation surface is intentionally tiny. `quantum-control.service`,
Ollama, Apache, databases and arbitrary systemd units cannot currently be
started, stopped or restarted by Quantum Control.

## Quick start

Requirements:

- Linux or another compatible Unix-like development environment
- Go 1.23 or newer to build
- systemd for service inspection and the current service mutation adapter

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

The public API listens on `127.0.0.1:17440` by default. When no actor registry
or legacy API token is configured on loopback, Quantum Control uses a local
bootstrap identity that can access only the read-only operator surface. It has
no audit-read, confirmation or mutation authority.

```bash
curl http://127.0.0.1:17440/healthz
curl http://127.0.0.1:17440/readyz
curl http://127.0.0.1:17440/v1/system/status
curl http://127.0.0.1:17440/v1/services/quantum-runtime.service
curl http://127.0.0.1:17440/v1/components
curl http://127.0.0.1:17440/v1/components/quantum-runtime
```

## Actors and TCI

An optional actor registry identifies human administrators, integration
services, mutation executors and the future Quantum TCI. The registry stores
SHA-256 token digests, not raw bearer tokens.

The TCI can be assigned the `tci-proposer` role to inspect permitted state and
create an immutable operation proposal. It cannot receive the `mutator` or
`approver` role, execute the approved-mutation endpoint or mint a confirmation
grant.

A human `approver` and a service `mutator` are deliberately separate roles.
The human approval token authorizes one exact immutable plan. The privileged
broker then independently authenticates the mutation executor and consumes the
single-use grant before invoking the system adapter.

See `config/actors.example.json`, `docs/SECURITY-CONTRACTS.md` and
`docs/SERVICE-MUTATIONS.md`.

## Transactional service control

The first mutation flow is:

```text
authenticated proposer
        |
        v
immutable plan
        |
        v
distinct human approval
        |
        v
root-owned single-use grant
        |
        v
qcored revalidates plan + actor + session + action + parameters
        |
        v
fixed systemctl argv for quantum-runtime.service
        |
        v
postcondition + health verification
        |
        v
durable audit + bounded recovery result
```

The optional deployment policy in
`config/service-mutation-policy.example.json` may remove
`quantum-runtime.service` from the mutation surface. It cannot add another
service. The machine-readable policy contract is
`schema/service-mutation-policy-v1alpha1.schema.json`.

## Durable audit

The production service writes a hash-chained JSONL audit by default to:

```text
/var/lib/quantum-control/audit/audit.jsonl
```

Actors with `audit.read` can query:

```text
GET /v1/audit
GET /v1/audit/integrity
```

There is no public audit write/update/delete API. Secret-like operation
parameters are redacted and arbitrary backend exception text is not stored in
durable audit records. Mutation audit records include attempt, final result and
recovery status. See `docs/AUDIT.md`.

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
command. Every administrative request maps to a named allowlisted action with
individually validated parameters. Public actor fields are overwritten by the
authenticated identity.

There is no `shell.exec` operation. Service mutations use a fixed
`systemctl <verb> -- <unit>` argument vector, and the compiled mutation unit
allowlist currently contains only `quantum-runtime.service`.

## Configuration

See:

- `config/quantum-control.env.example`
- `config/qcored.env.example`
- `config/actors.example.json`
- `config/service-mutation-policy.example.json`

For production, both services read the same root-owned broker token file. The
file should be owned by `root:quantum-control` with mode `0640`.

The actor registry should be root-protected when mutations are enabled. Both
processes may read the same registry, while raw confirmation-grant state is
owned only by `qcored` under `/var/lib/quantum-control-broker`. Plan TTL and
confirmation-grant TTL are configurable but capped at 15 minutes.

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
formatting, vetting, race-enabled tests and both production binaries. Pull
requests also build the amd64/arm64 release archives without publishing them.

## Documentation

- `docs/ARCHITECTURE.md`
- `docs/API.md`
- `docs/ROADMAP.md`
- `docs/DEPLOYMENT.md`
- `docs/SECURITY.md`
- `docs/SECURITY-CONTRACTS.md`
- `docs/SERVICE-MUTATIONS.md`
- `docs/AUDIT.md`
- `docs/LICENSE-POLICY.md`
- `api/openapi.yaml`

## License

Quantum Control project-owned code is licensed under the **Starlight Unit Studios Quantum Control Community Source License 1.0**.

Commercial hosting and managed-service operation are permitted under the
license conditions. Quantum Control itself may not be sold, sublicensed,
white-labeled or offered as a standalone paid control-panel, SaaS or general
Control API product. Third-party components retain their own terms.

The legally controlling German text is in `LICENSE.de.md`. `LICENSE.md` is an
English convenience translation. This is a custom Source Available license and
is not an OSI-approved open-source license.
