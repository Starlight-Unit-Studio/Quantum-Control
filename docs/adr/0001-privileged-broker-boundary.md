# ADR 0001: Separate public Control and privileged qcored

Status: accepted for the foundation

## Decision

Quantum Control uses two processes:

- `quantum-control`, an unprivileged public API and later user interface
- `qcored`, a narrowly privileged broker available through a protected Unix socket

## Reasons

- a web or TCI-facing process must not run as root
- privileged code remains small and reviewable
- filesystem permissions and a separate broker token create an additional local boundary
- future actions can be authorized and audited independently

## Consequence

Features that need privilege must be implemented as typed broker operations. They cannot call a shell through the public process.
