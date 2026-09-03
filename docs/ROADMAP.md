# Quantum Control Roadmap

## 0.1 security foundation

Implemented:

- split public service and privileged broker
- typed operation protocol
- Unix socket and broker authentication
- read-only system and service probes
- secure listener defaults
- tests and CI

## 0.2 inventory, adoption and pre-mutation security

Implemented in `0.2.0-alpha.1`:

- read-only detection of KeyHelp, Apache/Nginx, PHP/PHP-FPM, MariaDB/MySQL, PostgreSQL, Docker/Podman, Ollama, Quantum Runtime, SearXNG, Ember CoreUI and the STΛRLIGHT UNIT Game/Repack
- versioned component inventory schema and authenticated read API
- deterministic `managed`, `external`, `disabled` and fail-safe `unknown` states
- service, filesystem, version, capability, health and detection evidence
- clean-host, KeyHelp-host and partial-Starlight fixtures
- fixed probes only, bounded reads and secret-like metadata suppression

Implemented in `0.2.0-alpha.2`:

- authenticated human, service and future TCI actor contracts
- fixed roles and explicit permission scopes
- immutable expiring plan snapshots with exact canonical SHA-256 digests
- durable single-use confirmation grants bound to reviewed plans
- append-only hash-chained durable audit with secret redaction
- permission-scoped read-only audit API
- TCI proposal-only role boundary without confirmation or execution authority
- broker fail-closed gate for every future confirmation-required operation until structured grant verification is connected
- threat-model, retention, export and recovery documentation

Remaining read-only/adoption work may proceed independently:

- safely map detected listeners/ports to components instead of guessing defaults
- certificate and storage inventory
- durable non-secret component cache/state
- richer CoreUI and STU Repack status adapters

## 0.3 transactional service management

The first mutating milestone may begin only after `0.2.0-alpha.2` is merged and verified.

Planned:

- broker-side structured confirmation-grant verifier
- explicit `service.start`, `service.stop` and `service.restart` actions
- per-action role and confirmation policy
- maintenance windows
- precondition and postcondition health checks
- durable audit records for proposal, approval, attempt and result
- rollback/recovery behavior
- no generic command execution

## 0.4 web and TLS management

- domain and reverse-proxy objects
- generated configuration staging and validation
- certificate request and renewal
- atomic activation and rollback
- external web-stack adoption without implicit overwrite

## 0.5 data and application management

- database and credential lifecycle
- backup and restore transactions
- container and volume inventory
- application manifests and package deployment
- signed update metadata

## 0.6 Quantum Runtime integration

- runtime/model health surfaces
- GPU and storage status
- model provisioning plans
- CoreUI runtime selection
- TCI-safe explanatory and proposal interfaces

## 0.7 Quantum CoreOS profile

- native package and service profile
- unified health and update coordination
- Shell panels and notifications
- TCI context providers and typed action adapters
- Server/Desktop/Workstation policy presets

Quantum CoreOS implementation starts only after Quantum Runtime and Quantum
Control are independently usable and their contracts are stable.
