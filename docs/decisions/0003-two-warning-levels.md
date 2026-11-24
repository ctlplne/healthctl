# Two warning levels

Status: accepted, November 2026 (hardening branch).

## Context

A single "seek emergency care" warning made the app either over-alarm on recent or uncertain signs, or drop them.

## Decision

Two levels with app-authored text: "Seek emergency care", which interrupts, and "Get this checked promptly", which stays visible and lets the interview continue. A model flag the app's own reading doesn't confirm becomes the second level instead of being dropped.

## Consequences

Exports and the brief label the levels separately. Both stay until the user corrects the words they came from.
