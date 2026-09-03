# ADR 0003: Never allow an unauthenticated remote listener

Status: accepted for the foundation

## Decision

Quantum Control rejects every non-loopback listener unless `QUANTUM_CONTROL_API_TOKEN` is configured with at least 32 characters.

No override variable exists.

## Reasons

An administrative API can gain high-impact operations over time. A development convenience flag that disables authentication could later expose those capabilities accidentally.
