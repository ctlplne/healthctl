# Question intents, red flags and completeness owned by the app

Status: accepted, November 2026 (hardening branch).

## Context

The model chose the wording of questions and decided when the interview ended, so a red-flag question could be skipped and choices could suggest symptoms.

## Decision

The model names an intent from `intents.go`; the app writes the question, its choices and its explanation. Before review the app asks its own red-flag and completeness questions for each active pathway, unless the user stops; what remains is listed in the app's words and kept in the confirmed summary. The model may ask at most 12 questions.

## Consequences

The adaptive interview asks a few more questions per concern. All of this content is development content that needs clinical review (H1).
