# Changelog

## Unreleased: hardening

Safety and correctness, then reliability, then polish. [HARDENING.md](HARDENING.md) lists every item with its proof; the [decision records](docs/decisions/README.md) explain the main choices.

### Upgrading
Keep your workspace and installation key; the records migrate on first start. Check these before you upgrade:
- **Emergency contact.** If `CLINICAL_EMERGENCY_CONTACT` is set, it must include the local emergency number, such as "Call 911". Otherwise the app refuses to start and says why. Leave it unset for the generic wording.
- **Sessions.** Signed-in sessions stay signed in, but downloading, sharing, deleting and changing where a history is sent ask for the password again. A session also ends after an hour without activity (`CLINICAL_IDLE_MINUTES`).
- **Backups.** Scheduled backups are now kept by age as well as count. An archive is removed only when it is older than `CLINICAL_BACKUP_KEEP_DAYS` (default 30) and not among the newest `CLINICAL_BACKUP_KEEP` (default 7).
- **Reverse proxy.** Sign-in attempts are counted per client. Behind a proxy, set `CLINICAL_TRUSTED_PROXY` to its address.
- **Key rotation** now re-encrypts every record, so it takes longer on a large installation.

### Safety
- One context engine for negation, experiencer, time, hypotheticals and uncertainty; a wider urgent vocabulary (policy `development-5`, development content).
- Every input path goes through the same screen; witness accounts read "he" and "she" as the user.
- Two warning levels; a model's urgent flag needs only an exact quote and never fails a turn.
- The app asks its own safety-net, red-flag and completeness questions; "don't miss" lines for dangerous causes. Answering no to a safety question during an urgent encounter keeps the review open.
- A bare "they" in the user's own answer can mean their own symptoms when the app checks the model's findings (`validator-3`).
- One urgent banner lists every reported wording; the assessment says whether its stability check ran.
- Urgent signs the app records have readable titles ("Facial droop") and are marked as recognized by the app.
- A review that stops a running interview call extracts the answers that call would have read; a model result made for an earlier input is rejected.

### Model output
- Each item is validated on its own with reason codes; targeted repair; per-task schemas and generated prompt limits.
- Question intents with app wording and choices; a question budget; app-authored explanations.
- App-owned assessment rendering with a condition list; an optional stability check.
- Reasoning-aware OpenAI and Anthropic adapters, durable model jobs, cancellation of superseded calls, evidence selection near the input limit.

### Data and security
- A recovery kit, default backups kept by age, a wrapped data key and an independent receipt key; key rotation issues a fresh data key and re-encrypts every record.
- Downloads ask for the password again after ten minutes; viewing the report on screen doesn't.
- **Forgot your password?** on the sign-in page and the password prompt, and **Reset owner password…** in the macOS and Windows launchers.
- Deleting an encounter cancels its running model call.
- Secrets as files, sandboxed readers with limits, per-client sign-in throttling, setup codes, idle time-outs, re-authentication for sensitive actions, Trusted Types.

### App and release
- URL routing, no tab jumps after a turn, accessibility checks, one request path, polling back-off.
- Pinned desktop images and speech model, launcher stop, logs, delete and uninstall, an upgrade script with rollback.
- A labeled corpus, a property test, a user simulator, a mutation check and a scorecard drift check in CI.
