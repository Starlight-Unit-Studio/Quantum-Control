# Quantum Control Deployment Foundation

Status: manual alpha deployment only

Version `0.3.0-alpha.1` includes the first narrowly scoped privileged service
mutation path. It is not yet a production KeyHelp replacement and does not
include an automated installer.

## Intended filesystem layout

```text
/usr/local/bin/quantum-control
/usr/local/libexec/qcored
/etc/quantum-control/quantum-control.env
/etc/quantum-control/qcored.env
/etc/quantum-control/broker.token
/etc/quantum-control/actors.json
/etc/quantum-control/service-mutation-policy.json
/etc/systemd/system/quantum-control.service
/etc/systemd/system/qcored.service
/run/quantum-control/qcored.sock
/var/lib/quantum-control/audit/audit.jsonl
/var/lib/quantum-control-broker/grants.json
```

## Service identity

Create a dedicated system group and unprivileged service account:

```text
group: quantum-control
user:  quantum-control
```

The public `quantum-control` process runs as this unprivileged account.
`qcored` runs as root inside the hardened systemd unit because it is the only
component allowed to invoke privileged typed system adapters.

`qcored` does not expose a shell or arbitrary command API. The initial compiled
mutation target is only `quantum-runtime.service`.

## Broker token

Generate at least 32 random bytes and store the encoded result in the broker
token file. The same file is read by both services.

Required ownership:

```text
owner: root
group: quantum-control
mode:  0640
```

Never reuse an actor/public API token as the broker token.

## Actor registry

Approved mutations require an actor registry configured for both processes:

```text
QUANTUM_CONTROL_ACTOR_FILE=/etc/quantum-control/actors.json
```

The file contains SHA-256 digests of actor bearer tokens, not plaintext tokens.
A practical ownership profile is:

```text
owner: root
group: quantum-control
mode:  0640
```

The public service needs the registry to map the incoming credential to its
public actor/permission set. `qcored` reads the same root-controlled policy and
independently authenticates approvers and mutation executors.

At minimum keep human approval and service mutation execution on distinct
credentials. See `config/actors.example.json`.

## Root-owned confirmation state

`qcored.service` provisions:

```text
StateDirectory=quantum-control-broker
StateDirectoryMode=0700
```

The default durable grant path is:

```text
/var/lib/quantum-control-broker/grants.json
```

The unprivileged Control process must not receive write access to this file.
Only SHA-256 token digests and grant metadata are persisted.

The public durable audit remains in the separate `quantum-control` state
directory.

## Mutation policy

The built-in allowlist contains only:

```text
quantum-runtime.service
```

An optional root-controlled file may narrow that list:

```text
QUANTUM_CONTROL_SERVICE_POLICY_FILE=/etc/quantum-control/service-mutation-policy.json
```

Use `config/service-mutation-policy.example.json` as the shape. A policy file
that names any unit outside the compiled allowlist causes startup/configuration
validation to fail. It cannot turn a syntactically valid arbitrary unit into a
privileged target.

## Public listener

The default is loopback-only:

```text
QUANTUM_CONTROL_LISTEN=127.0.0.1:17440
```

A non-loopback address is rejected unless a valid public credential source is
configured. Remote use additionally requires TLS through a carefully configured
reverse proxy. The alpha service itself does not terminate TLS.

## Build

```bash
make check
make build
```

The resulting binaries are written to `bin/` by `make build`.

## Service order

1. Install the root-controlled environment files, actor registry and broker token.
2. Optionally install a narrowing service-mutation policy.
3. Start `qcored`.
4. Confirm that `/run/quantum-control/qcored.sock` exists with group access.
5. Start `quantum-control`.
6. Check `/healthz` and `/readyz`.
7. Inspect the operation catalog before issuing a mutation plan.

## Catalog validation

The expected 0.3 alpha operation catalog includes:

```text
system.snapshot
service.status
service.start
service.stop
service.restart
```

The mutation definitions must advertise only `quantum-runtime.service` in the
allowed value set. Stop deployment if another mutation unit appears
unexpectedly.

Any `shell.exec`, arbitrary command, file mutation or package-management action
must still be rejected.

## Approved mutation sequence

Use a proposer/operator credential to create the immutable plan. Use a distinct
human approver credential to call `/v1/confirmations`. Submit the returned
single-use token through `/v1/operations/execute-approved` using a `mutator`
service identity.

Do not retry an approved mutation automatically after an HTTP/network failure.
The grant is consumed in `qcored` before the privileged adapter executes. An
ambiguous result requires status/audit inspection and a new plan plus a new
approval if another change is necessary.

## Upgrade boundary

Until a transactional installer exists, upgrade both binaries together from one
tested release. Do not mix broker/public protocol versions. Preserve local
environment files, actor registry, broker token, audit data and root-owned grant
state outside release archives.
