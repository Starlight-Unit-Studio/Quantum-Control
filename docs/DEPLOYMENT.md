# Quantum Control Deployment Foundation

Status: manual alpha deployment only

Version `0.1.0-alpha.1` is a read-only control-plane foundation. It is not yet a production KeyHelp replacement and does not include an automated installer.

## Intended filesystem layout

```text
/usr/local/bin/quantum-control
/usr/local/libexec/qcored
/etc/quantum-control/quantum-control.env
/etc/quantum-control/qcored.env
/etc/quantum-control/broker.token
/etc/systemd/system/quantum-control.service
/etc/systemd/system/qcored.service
/run/quantum-control/qcored.sock
```

## Service identity

Create a dedicated system group and unprivileged service account according to the conventions of the target distribution:

```text
group: quantum-control
user:  quantum-control
```

`qcored` runs as root but with an empty capability bounding set and a strict read-only service profile in the current foundation. The public `quantum-control` process runs as the dedicated unprivileged user.

## Broker token

Generate at least 32 random bytes and store the encoded result in the broker token file. The same file is read by both services.

Required ownership:

```text
owner: root
group: quantum-control
mode:  0640
```

Never reuse the public API token as the broker token.

## Public listener

The default is loopback-only:

```text
QUANTUM_CONTROL_LISTEN=127.0.0.1:17440
```

A non-loopback address is rejected unless `QUANTUM_CONTROL_API_TOKEN` contains at least 32 characters. Remote use additionally requires TLS through a carefully configured reverse proxy. The alpha service itself does not terminate TLS.

## Build

```bash
make check
make build
```

The resulting binaries are written to `bin/` by `make build`.

## Service order

1. Install configuration and the protected broker token.
2. Start `qcored`.
3. Confirm that `/run/quantum-control/qcored.sock` exists with group access.
4. Start `quantum-control`.
5. Check `/healthz` and `/readyz`.
6. Verify the operation catalog before using convenience endpoints.

## Read-only validation

The current release should expose only:

```text
system.snapshot
service.status
```

Any `shell.exec`, service mutation, file mutation or package-management request must be rejected. Stop deployment immediately if the catalog contains an unexpected operation.

## Upgrade boundary

Until a transactional installer exists, upgrade the two binaries together from one tested release. Do not mix protocol versions. Preserve local environment files and the broker token outside release archives.
