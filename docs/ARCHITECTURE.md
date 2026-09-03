# Quantum Control Architecture

Status: `0.1.0-alpha.1` foundation

## Responsibility

Quantum Control owns server and operating-system administration workflows. It
will eventually manage services, domains, reverse proxies, TLS, databases,
containers, firewall policy, backups, updates and application deployment.

It does not own:

- AI inference
- model lifecycle
- TCI personality
- Ember CoreUI application data
- Quantum CoreOS desktop composition
- arbitrary remote shell access

## Process boundary

```text
quantum-control
  unprivileged HTTP/API and later UI
          |
          | authenticated HTTP over Unix socket
          v
qcored
  narrowly privileged operation broker
          |
          v
allowlisted adapters
  systemd, web server, database, firewall, backup tools
```

The public process cannot directly perform privileged changes. The broker does
not understand free-form instructions. It receives only a typed action and a
bounded parameter map.

## Operation lifecycle

```text
request
  -> schema and action validation
  -> plan
  -> authorization and confirmation when required
  -> fixed adapter execution
  -> health verification
  -> audit result
  -> rollback where the operation supports it
```

The alpha implements only the validation, planning and read-only execution
portion of this lifecycle.

## Initial allowlist

```text
system.snapshot  read-only bounded machine information
service.status   read-only systemd unit state
```

There is deliberately no generic command operation.

## Authentication layers

1. The public API binds to loopback by default.
2. Every remote binding requires a bearer token of at least 32 characters. There is no unauthenticated override.
3. `qcored` accepts requests only through a Unix socket.
4. Socket filesystem permissions restrict local callers.
5. A separate broker token authenticates the Control process to `qcored`.
6. Future user roles and confirmation grants exist above the broker token.

The broker token proves service identity. It is not a replacement for user
roles, multi-factor confirmation or per-operation policy.

## Integration model

Quantum Control is developed and released independently before Quantum CoreOS.

```text
normal Linux server -> Quantum Control
Quantum CoreOS       -> same Quantum Control release + native profile
Ember CoreUI         -> optional status/deployment integration
Quantum TCI          -> typed plans and requests through Control policy
```

CoreOS-specific optimization belongs in adapters and deployment profiles, not a
private fork.

## Package boundaries

```text
cmd/quantum-control  unprivileged process
cmd/qcored           broker process
internal/control     public API
internal/broker      operation registry, server and client
internal/protocol    stable typed messages
internal/systemprobe fixed read-only system adapters
internal/config      secure process configuration
```
