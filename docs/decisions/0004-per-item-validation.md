# Validate each item separately, with reason codes

Status: accepted, November 2026 (hardening branch).

## Context

One invalid finding used to fail a whole model turn, wasting the valid parts and giving the model no usable feedback.

## Decision

Each finding, flag, choice and assessment item is checked on its own and dropped with a named reason code (`internal/clinical/reasons.go`). Only a problem with the whole proposal fails the call, and the single repair request carries the app's reason codes, never the rejected text. Reason codes are stored with every model call.

## Consequences

Turns succeed more often, failures are measurable per reason, and a validator version and input hash let past results be re-checked after rules change.
