# Quantum Control Durable Audit

Status: `v1alpha1`

Quantum Control records security-relevant planning, approval and execution
metadata in an append-only local audit chain.

## Storage

Default path:

```text
/var/lib/quantum-control/audit/audit.jsonl
```

The packaged public service receives a private systemd state directory. Audit
files are created with mode `0600`.

Each line is one `quantum.control/audit-record/v1alpha1` JSON object.

The root-owned confirmation-grant state is separate and lives under
`/var/lib/quantum-control-broker`. It is not a replacement for public durable
audit history.

## Integrity chain

Every record contains the authenticated actor plus bounded security metadata,
including monotonically increasing sequence, audit ID, UTC time,
request/session/plan correlation, action/risk, final status, stable error code,
redacted parameters, `previous_hash` and `entry_hash`.

Mutation records may also include `rollback_status`, which currently carries the
bounded service recovery state such as `not_required`, `succeeded`, `failed`,
`not_defined` or `unknown`.

`entry_hash` is a SHA-256 digest over the canonical record excluding
`entry_hash` itself. It includes `previous_hash`, forming a chain.

Quantum Control verifies every line and the full hash chain during startup. It
never skips a damaged line, silently truncates history or rewrites an old entry.

## Mutation event sequence

A normal reviewed service mutation produces evidence across the flow:

```text
plan.created or proposal.created
confirmation.issued
operation.attempt
operation.completed or operation.failed
```

If the public process loses broker transport after the approved request has
already been submitted, it records an `operation.unknown` outcome with
`rollback_status=unknown`. It does not guess whether the privileged action ran
and does not automatically retry it.

The privileged grant has already been consumed before the service adapter is
invoked, so replaying the same token cannot turn an ambiguous result into an
uncontrolled second mutation.

## Redaction

Parameter names that indicate secrets are stored as `[REDACTED]`. This includes
password, passphrase, token, secret, credential, API-key and private-key style
names.

Raw confirmation tokens and actor bearer tokens are not placed into public audit
records. Arbitrary backend error strings are not written to the durable record;
stable error codes are stored instead.

This is defense in depth, not permission to send secrets as operation
parameters. Typed operations should avoid secret-bearing parameters whenever a
separate protected credential channel can be used.

## Query API

Actors with `audit.read` may use:

```text
GET /v1/audit
GET /v1/audit/integrity
```

`/v1/audit` supports read-only filters:

```text
limit      1..500
actor_id   exact actor ID
action     exact action
```

There is intentionally no POST, PUT, PATCH or DELETE audit API.

## Retention policy for v1alpha1

Quantum Control performs no automatic audit deletion or rotation in this
release.

This avoids introducing an unreviewed mechanism that could destroy security
evidence. Operators must provision enough storage for the audit history.

A future rotation design must preserve verifiable chain boundaries and archive
metadata before it can become an automated feature.

## Export policy

The permission-scoped read API is the supported online export surface.

For forensic/offline export, copy the JSONL file while Quantum Control is
stopped or from a filesystem snapshot. Preserve file metadata and record the
final `head_hash` returned by `/v1/audit/integrity` when possible.

Do not transform the source audit file in place.

## Recovery policy

If startup reports an audit integrity failure:

1. stop Quantum Control
2. preserve the damaged audit file unchanged
3. copy it to incident evidence storage
4. investigate filesystem, disk and administrative activity
5. restore a known-good audit file if available
6. otherwise wait for an explicit audited lineage-reset procedure in a future release

The v1alpha1 service does not contain an automatic reset switch. Deleting or
replacing the audit file merely to make the service start is not a supported
recovery procedure.

## Security limitation

The chain detects modification relative to the state Quantum Control later
reads. It does not defeat a fully compromised root account that can replace the
program and all local state together. Its purpose is to make ordinary API
clients, services and AI actors unable to rewrite administrative history through
Quantum Control.
