# One context engine for negation, experiencer, time and hypotheticals

Status: accepted, November 2026 (hardening branch).

## Context

Clause-wide keyword checks missed 80% of urgent sentences in the baseline corpus and treated "I have chest pain and no fever" as negated.

## Decision

`internal/clinical/context.go` is the only place that decides whether words are negated, about someone else, in the past, hypothetical or uncertain. The safety screen, the validator, pathway matching and contradiction checks all use it. A static test (`TestNoSpecialCaseRegexes`) keeps new one-off rules out, and the corpus and mutation check test it.

## Consequences

Rules are easier to review in one file. Held-out sets still show that keyword rules generalize poorly; the model's urgent flags and the app's explicit safety questions carry the safety load, and clinical review (H1) is still required.
