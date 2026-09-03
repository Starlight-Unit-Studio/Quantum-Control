# Quantum Control Third-Party Notices

Quantum Control project-owned code is governed by the license identified in `LICENSE_HISTORY.md`. Third-party material is not relicensed by that license.

## Current foundation

The `0.1.0-alpha.1` Go source uses the Go standard library and does not vendor an external Go module dependency.

Quantum Control can inspect or administer operating-system services and external software. Detection or administration does not mean that those components are distributed by this repository or licensed by Starlight Unit Studios.

Examples include Linux, systemd, KeyHelp, Apache, Nginx, PHP, MariaDB, PostgreSQL, Docker, Ollama, and Quantum Runtime. Each remains subject to its own license and notices.

## Future bundled components

Before a release bundles a third-party library, package, font, web asset, database client, certificate tool, container component, or other dependency, the release process must record at least:

- component name and version
- source or project location
- applicable license identifier or license file
- required copyright and attribution notices
- whether modification or redistribution is permitted
- any operational or distribution conditions

Component-specific license texts may be stored below `third_party/licenses/` in a future release. This file must be updated when bundled third-party content changes.
