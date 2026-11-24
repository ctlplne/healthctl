# A wrapped data key and an independent receipt key

Status: accepted, November 2026 (hardening branch).

## Context

The installation key encrypted every record directly and doubled as the key for submission receipts, so rotating it meant rewriting every record, and receipts were tied to it.

## Decision

Records are encrypted under a data key; the installation key only wraps it and a separate random receipt key. Rotation re-wraps the two keys in one transaction. Older installations keep their original key as the data key.

## Consequences

Rotation is fast and never rewrites clinical records. Backups still need the matching installation key.
