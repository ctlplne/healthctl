# The app, not the model, is in charge

Status: accepted, November 2026 (hardening branch).

## Context

healthctl used a language model to decide what to ask, when to stop, what was urgent and how results were worded. A model's output can be wrong, inconsistent between runs, or written to sound more certain than the evidence.

## Decision

The application decides state, safety, completeness, provenance and every piece of wording that isn't the user's own words or a labeled model suggestion. The model only proposes quoted findings, the next question's intent and explanations that cite fact IDs. Raising concern needs only an exact quote; lowering it needs strong support; the model never clears a warning.

## Consequences

Many model outputs are now replaced or checked by app code: questions come from an intent catalog, choices are filtered, red flags and completeness are asked by the app, condition names come from a list, and the state machine is an explicit table. The model's freedom to phrase things is smaller, by design.
