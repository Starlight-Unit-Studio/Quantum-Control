# Quantum Control Roadmap

## 0.1 security foundation

- split public service and privileged broker
- typed operation protocol
- Unix socket and broker authentication
- read-only system and service probes
- operation plans, audit identifiers and structured errors
- secure listener defaults
- tests and CI

## 0.2 inventory and adoption

- detect existing KeyHelp, Apache/Nginx, PHP-FPM, MariaDB/PostgreSQL, Docker and
  Quantum Runtime
- classify components as `managed`, `external` or `disabled`
- persistent non-secret component state
- read-only service, port, certificate and storage inventory
- CoreUI and STU Repack status adapters

## 0.3 transactional service management

- explicit service start/stop/restart actions
- role and confirmation policy
- maintenance windows
- precondition and postcondition health checks
- durable audit records
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
