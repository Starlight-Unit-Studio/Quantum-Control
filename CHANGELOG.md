# Changelog

## 0.1.0-alpha.1

Initial executable Quantum Control foundation.

### Added

- separate unprivileged `quantum-control` API and privileged `qcored` broker
- protected Unix-socket transport with a mandatory broker token
- loopback-only public API default and mandatory bearer authentication for every non-loopback bind
- typed operation catalog, planning and execution protocol
- read-only `system.snapshot` and `service.status` operations
- strict systemd unit validation and fixed `systemctl show` invocation
- request and audit identifiers
- structured health, readiness and capability endpoints
- stable handling of semantic operation rejection across the broker boundary
- systemd service hardening examples
- OpenAPI, architecture, API, security and roadmap documentation
- formatting, vet, race, build and live two-process verification

### Licensing

- adopted the Starlight Unit Studios Quantum Control Community Source License 1.0
- explicitly permitted commercial hosting and managed-service operation while prohibiting sale and white-label resale of Quantum Control itself
- added controlling German and translated English license texts
- added license history, copyright, notice, trademark and third-party notice files
- added CI verification that required legal files are present and internally consistent

### Security posture

- no arbitrary command or shell operation
- no mutating administrative operation
- no unauthenticated remote-listener override
- no secrets in source configuration examples
- public broker failures do not expose internal socket paths or transport details

### Not yet implemented

- user and role management
- mutating service operations
- domains, reverse proxy and TLS
- databases and containers
- backups, restore and updates
- web interface
- Quantum Runtime and CoreOS adapters
