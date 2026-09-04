# Quantum Control Architecture

Status: `0.3.0-alpha.1`

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
human / service / future TCI
          |
          v
quantum-control
  unprivileged HTTP/API and later UI
  actor policy, immutable plans, public audit
          |
          | authenticated HTTP over protected Unix socket
          v
qcored
  narrowly privileged operation broker
  root-owned grant state + independent actor verification
          |
          v
allowlisted adapters
  read probes + typed service lifecycle adapter
          |
          v
systemd / future explicitly reviewed platform adapters
```

The public process cannot directly perform privileged changes. The broker does
not understand free-form instructions. It receives only typed operations and
bounded structured data.

The broker token authenticates the public Control service to `qcored`. It is
separate from human, service-executor and TCI actor credentials.

## Operation lifecycle

Read-only requests use:

```text
request
  -> actor/permission validation
  -> typed broker validation
  -> fixed read adapter
  -> durable audit result
```

Confirmation-required service mutations use:

```text
request or TCI proposal
  -> broker policy validation
  -> immutable expiring plan
  -> human review and confirmation
  -> root-owned single-use grant
  -> mutation executor authentication
  -> qcored plan + current-policy revalidation
  -> atomic grant consumption
  -> fixed privileged adapter
  -> postcondition/health verification
  -> durable audit result
  -> bounded recovery where defined
```

A submitted mutation is never automatically retried after an ambiguous broker
transport result.

## Current operation allowlist

Read-only:

```text
system.snapshot
service.status
```

Confirmation-required:

```text
service.start
service.stop
service.restart
```

The compiled mutation target set currently contains only:

```text
quantum-runtime.service
```

A root-controlled deployment policy may narrow this set. It cannot broaden it.
There is deliberately no generic command operation.

## Privileged service adapter

The first mutation adapter selects the executable and lifecycle verb in code.
For the only current mutation target, production invokes an argument vector
equivalent to one of:

```text
systemctl start -- quantum-runtime.service
systemctl stop -- quantum-runtime.service
systemctl restart -- quantum-runtime.service
```

No shell parser receives actor, user or model text.

The adapter captures service state before and after execution. Active Runtime
postconditions additionally use a fixed loopback health endpoint. Recovery is
one bounded action toward the observed precondition when a safe policy is
defined, not a blind retry of the failed mutation.

## Authentication layers

1. The public API binds to loopback by default.
2. Every remote binding requires a configured credential source.
3. Public actor credentials identify `human`, `service` or `tci` actors.
4. `qcored` accepts requests only through a protected Unix socket.
5. A separate broker token authenticates `quantum-control` to `qcored`.
6. `qcored` independently authenticates the human approver and mutation executor for privileged writes.
7. Human confirmation and mutation execution are distinct permissions.
8. Root-owned confirmation-grant state is inaccessible to the unprivileged Control process.

The broker token proves service-to-broker identity. It is never a substitute
for per-actor policy or per-operation confirmation.

## State ownership

```text
quantum-control
  ephemeral operation-plan cache
  append-only public audit

qcored
  root-owned durable confirmation-grant store
  current privileged mutation policy

systemd / Runtime
  actual service state
```

The public plan cache is intentionally ephemeral. Losing it forces a new plan
and review. Losing grant state invalidates outstanding approvals. Neither case
silently grants authority.

## TCI boundary

Quantum TCI integrates through the same public actor and plan contracts as
other clients. It may inspect permitted state and create proposals, but it
cannot hold the human approval or mutation-executor permissions.

Model output never becomes executable command text. TCI support belongs in
typed proposal/context adapters, not in a root shell bridge.

## Integration model

Quantum Control is developed and released independently before Quantum CoreOS.

```text
normal Linux server -> Quantum Control
Quantum CoreOS       -> same Quantum Control release + native profile
Ember CoreUI         -> optional status/deployment integration
Quantum Runtime      -> first explicitly supported service mutation target
Quantum TCI          -> typed proposals through Control policy
```

CoreOS-specific optimization belongs in adapters and deployment profiles, not a
private fork.

## Package boundaries

```text
cmd/quantum-control       unprivileged process
cmd/qcored                privileged broker process
internal/control          public API, plan cache and durable audit integration
internal/broker           operation registry, broker server/client, approval boundary
internal/protocol         stable typed messages
internal/security         actors, plans, grants and audit contracts
internal/systemprobe      fixed read-only system adapters
internal/servicecontrol   fixed service lifecycle and Runtime health adapters
internal/inventory        read-only component discovery
internal/config           secure process configuration
```

Future mutating domains must pass the same explicit security and transactional
gate before receiving a privileged adapter.
