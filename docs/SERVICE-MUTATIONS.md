# Transactional service mutations

Quantum Control `0.3.0-alpha.1` introduces the first privileged mutation path.
The implementation is deliberately limited to systemd lifecycle control for a
compile-time set of supported services.

## Initial action set

The broker publishes three confirmation-required operations:

```text
service.start
service.stop
service.restart
```

The compiled mutation allowlist currently contains only:

```text
quantum-runtime.service
```

A deployment policy may remove that unit from the enabled surface. It cannot
add another unit, even when the new name is syntactically valid.

## Trust boundary

The public `quantum-control` process remains unprivileged. It authenticates the
caller, requests a typed broker plan and creates the immutable public operation
plan. Confirmation-grant state is not stored by this process.

`qcored` owns the durable grant store and independently authenticates the human
approver and later the mutation executor. The broker does not trust a boolean,
header or free-form confirmation string supplied by the public process.

The execution sequence is:

```text
proposal
  -> broker policy plan
  -> immutable public plan + SHA-256 digest
  -> distinct authenticated human approval
  -> qcored creates one single-use grant
  -> authenticated mutation executor submits exact plan + grant
  -> qcored revalidates plan and current policy
  -> qcored atomically consumes grant
  -> fixed systemctl argument vector
  -> bounded postcondition and health verification
  -> result + recovery status
  -> durable public audit
```

## Grant binding

A grant is bound to the immutable plan ID and digest, plan actor, session ID and
exact action. The plan digest already binds normalized parameters, risk,
validity and expiry. Any changed action, actor, session or parameter produces a
different plan and cannot reuse the original grant.

The grant token itself is random and is never persisted in plaintext. The
root-owned grant store keeps only its SHA-256 digest and durable consumed state.
Consumption happens before the privileged adapter is called. Therefore a
success, failure, lost HTTP response or process restart cannot make the same
grant valid for another execution attempt.

## Executor and approver separation

The `approver` role is human-only and can issue confirmation for an eligible
plan. It does not automatically receive `operations.execute.mutate`.

The `mutator` role is intended for an authenticated service identity that may
submit an already approved plan. It cannot issue its own confirmation unless it
also represents a separate eligible human identity, which the current actor
model does not permit.

TCI actors cannot hold either authority. Model output remains proposal data.

## Privileged adapter

The service adapter never accepts a command line. Production constructs exactly
one of these argument vectors:

```text
systemctl start -- quantum-runtime.service
systemctl stop -- quantum-runtime.service
systemctl restart -- quantum-runtime.service
```

The executable and verb are selected by code. The unit must first pass both the
unit-name validator and the compiled/deployment allowlist. No shell is involved.

## Precondition and postcondition

Before executing an action, `qcored` captures bounded service state through the
existing fixed `systemctl show` probe.

After the mutation it polls until the expected state is observed or the hard
transaction timeout expires:

```text
start    -> active
restart  -> active
stop     -> inactive
```

For an active `quantum-runtime.service` postcondition, the current profile also
requires HTTP 200 from the fixed loopback Runtime health endpoint:

```text
http://127.0.0.1:11450/healthz
```

The health URL is compile-time policy data, not caller input. Redirects and
proxy use are disabled.

## Recovery behavior

Recovery is intentionally narrow and deterministic. Quantum Control does not
blindly repeat the failed mutation.

If the observed precondition was `active`, a failed transaction may attempt one
`service.start` recovery toward an active state. If the precondition was
`inactive` or `failed`, recovery may attempt one `service.stop` toward an
inactive state. For another or ambiguous precondition, recovery is
`not_defined` and no speculative action is chosen.

A successful restart cannot restore the previous process instance. Recovery
means restoring the expected running/stopped service state, not rolling a
process back in time.

The result exposes a bounded `recovery_status` such as:

```text
not_required
succeeded
failed
not_defined
```

The public durable audit copies this into `rollback_status` for compatibility
with the broader transactional-operation contract.

## Interrupted and ambiguous execution

A public-to-broker transport error after an approved execution request has been
submitted is treated as an unknown result by the public service. It is not
retried automatically. The durable grant has already been consumed before the
system adapter runs, so replaying the same token is rejected.

Operators must inspect current service state and audit evidence, then create a
new plan and obtain a new human confirmation if another mutation is required.

## Explicit non-goals for this milestone

This release does not permit mutation of `quantum-control.service`, Ollama,
Apache/Nginx, databases, PHP, containers or arbitrary systemd units. It does not
manage domains, TLS, packages or files and does not add generic shell access.
