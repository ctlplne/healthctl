# healthctl hardening checklist

This file tracks the `hardening` branch. It is the source of truth for what is done, what proves it, and what still needs a person. Each item lists its ID, the change, the main files, and the test or command that proves it.

## Rules for every change

1. The app decides state, safety, completeness, provenance, and any wording that isn't the user's own words or a labeled model suggestion.
2. The model only proposes quoted findings, the next question's intent, and explanations that cite fact IDs.
3. Each item is validated separately with a named reason code. Raising concern needs only an exact quote. Lowering it needs strong support. The model never clears a warning.
4. All user text passes through one safety screen.
5. A failure saves the history and tells the user what to do next.

Tests, invariants and checks are never deleted or weakened to get a pass. All data is synthetic. New urgent phrases, red flags and pathway content are development content that has not been clinically reviewed.

**Feature freeze:** no new user-facing features until every P0 item passes and a live-model baseline exists (see H3).

Status: `[x]` done with proof · `[ ]` open · `[h]` handoff, prepared with next steps.

## P0: safety and correctness

- [x] **E1** Labeled corpus of 500+ sentences (urgent, negated, past, third-party, witness, hypothetical, routine) with correct and must-reject extractions; baseline recorded below. Files: `internal/clinical/testdata/corpus.json`, `internal/clinical/corpus_test.go`. Proof: `go test -run TestCorpus -v ./internal/clinical`.
- [x] **S3** One shared context engine: scoped negation, experiencer by source, past, hypothetical, uncertainty. Used by the safety screen, the validator and contradiction checks. Files: `internal/clinical/context.go`, `safety.go`, `model.go`, `encounter.go`. Proof: `TestContext*`, corpus.
- [x] **S2** Experiencer by source type: he/she/they in a witness account mean the user. Files: `context.go`, `safety.go`. Proof: `TestWitnessThirdPersonWarns`, corpus witness rows.
- [x] **S1** Every input goes through the same screen: answers, corrections, diary saves, witness accounts, visual notes, report passages, dictation drafts, return-visit notes, outcomes. Files: `internal/clinical/*`, `internal/server/diary.go`, `internal/server/voice.go`. Proof: `TestEveryInputPathScreens`, `TestDiarySaveScreensPostgres`.
- [x] **S4** Wider urgent vocabulary, labeled development content; policy version `development-4`. Files: `safety.go`, `internal/clinical/POLICY.md`. Proof: corpus urgent rows.
- [x] **S5** A model urgent flag needs only an exact quote from an active, non-screen answer and never fails a turn. When app rules disagree it becomes a "check this" warning. Files: `model.go`, `safety.go`. Proof: `TestModelUrgentFlagNeverFailsTurn`.
- [x] **V1** Per-item validation: invalid findings, flags, choices and items are dropped one by one, and valid items and the question are kept. Files: `internal/clinical/sanitize.go`, `internal/server/encounters.go`. Proof: `TestSanitizeKeepsValidItems`.
- [x] **V2** Named reason codes for every rejection, stored in `model_operations` and shown in the qualification panel. Files: `internal/clinical/reasons.go`, `internal/store/migrations/007_*.sql`, `internal/server/evidence.go`, `web/src/operator.tsx`. Proof: `TestRejectionReasonsRecordedPostgres`, `TestReasonCodes`.
- [x] **V3** Targeted repair: a retry carries app-authored reason codes and item indexes, never rejected text. Files: `internal/provider/client.go`. Proof: `TestRepairCarriesReasonCodes`.
- [x] **V4** The advice filter checks instructions only in statements, so questions such as "Did you start taking…?" pass. Files: `model.go`. Proof: `TestAdviceFilterAllowsQuestions`.
- [x] **V5** Digits inside names (COVID-19, B12, type 2) are not numeric claims. Files: `model.go`. Proof: `TestGroundedNumbersIgnoresNames`.
- [x] **O1** Reasoning effort is chosen by task: medium or lower for interview turns, highest for assessment. Files: `internal/provider/chatgpt.go`. Proof: `TestEffortByTask`.
- [x] **D1** `init.py` refuses to create a new key when installation data already exists, and explains recovery. Files: `scripts/init.py`. Proof: `python3 -m unittest tests.test_init`.
- [x] **D2** Recovery kit: a script writes a private recovery sheet; the app shows a banner until the owner confirms the kit's fingerprint. Files: `scripts/recovery_kit.py`, `internal/server/recovery.go`, `web/src/*`. Proof: `TestRecoveryKitFingerprint`, `TestInstallationSafetyPostgres`, `tests/test_recovery_kit.py`.
- [x] **D3** Backups on by default to a host folder, with last success shown in the app. Files: `compose.yaml`, `scripts/backup.sh`, `scripts/init.py`, `internal/store/migrations/008_installation_safety.sql`, `internal/server/installation.go`, `web/src/safety.tsx`, `desktop/manage.sh`. Proof: `TestInstallationSafetyPostgres`, `tests/test_init.py`, compose config check. Desktop launchers record manual backups in the same status. Scheduled desktop backups and Windows backups are tracked under R3 and H6.
- [x] **F1** Drafts are tied to their question; when the question changes, the old draft is set aside and the user is told. Files: `web/src/draft.ts`, `web/src/encounter.tsx`. Proof: `web/src/draft.test.tsx` (`npm test`).
- [x] **F2** No silent truncation: conflicts get their own section in the brief, and every shortened list says how many more there are. Files: `web/src/handoff.tsx`. Proof: `web/src/handoff.test.tsx`; the provenance appendix lists every open question and conflict.
- [x] **F3** An error boundary that still shows active urgent warnings. Files: `web/src/main.tsx`, `web/src/boundary.tsx`. Proof: `web/src/boundary.test.tsx`.
- [x] **M3** The random-action property test is permanent and runs in CI. Files: `internal/clinical/property_test.go`, `.github/workflows/ci.yml`. Proof: `TestRandomActionInvariants`.
- [x] **K1** `scripts/check_hardening.py` runs every required check and prints `ALL CHECKS PASSED`. Proof: the script itself; its final full run is recorded at the end of this file.

## P1: reliability and trust

- [x] **S6** Two warning tiers ("Seek emergency care" and "Get this checked promptly") with app-authored text, both kept until the source is corrected. Proof: `TestWarningTiers`, `TestModelUrgentFlagNeverFailsTurn`; the UI, report and brief label check warnings separately.
- [x] **S7** App-owned safety-net questions before confirming the history, at the start of a return visit, and for a diary episode happening now. Files: `internal/clinical/safetynet.go`, `web/src/safetynet.tsx`, `web/src/history.tsx`, `web/src/encounter.tsx`, `web/src/diary.tsx`. Proof: `TestSafetyNetQuestions`, `web/src/safetynet.test.tsx`, browser journeys answer it before confirming.
- [x] **S8** Clinical sign-off metadata per policy version, shown in exports; a policy can't be called reviewed without a record. Files: `internal/clinical/policy_review.go`. Proof: `TestPolicyReviewRecord`, `TestProvenanceValidatorVersionPostgres` (exports carry `policyReview`).
- [x] **S9** Safety metrics on the gold set run on every release; CI fails on any missed urgent case. Proof: the `safety` CI job runs `TestCorpus` (fails on any missed urgent sentence) and posts the metrics to the job summary.
- [x] **V6** Separate output schemas per task with enums. Files: `internal/provider/schema.go`. Proof: `TestTaskSchemas`.
- [x] **V7** Prompt limits generated from the validator's constants. Files: `internal/contract/contract.go`, `internal/provider/contracts/clinical-v9.txt` (v8 when this item landed). Proof: `TestPromptMatchesValidator`.
- [x] **V8** `validatorVersion` and an input hash recorded in provenance. Proof: `TestProvenanceValidatorVersionPostgres`, `TestPolicyReviewRecord` (stable input hash).
- [x] **V9** Question intents: the model picks an intent and its slots from an app catalog and the app writes the wording; free-text questions are a fallback with stricter checks (one question ending in "?", at most 300 bytes, no compound question, no repeat of an element the app already asked). Prompt `clinical-v9` lists the catalog, generated from code. Files: `internal/clinical/intents.go`, `question_source.go`, `model.go`, `internal/provider/schema.go`. Proof: `TestQuestionIntents`, `TestPromptMatchesValidator`.
- [x] **V10** Answer choices come from intent templates; a free-text choice that names a symptom or urgent sign the user hasn't reported is dropped (`choice_introduces_symptom`). Files: `intents.go`, `sanitize.go`. Proof: `TestIntentChoices`.
- [x] **C1** App-owned completeness per pathway (onset, course, severity, character, location, triggers, prior episodes, medicine changes). When the model proposes review, the app first asks what is still missing in its own words; if the user stops, the gaps stay listed and go into the confirmed summary, and the model can't remove them. Files: `intents.go`, `model.go`, `web/src/redflags.tsx`, `web/src/history.tsx`, `web/src/handoff.tsx`. Proof: `TestCompletenessGate`, `web/src/redflags.test.tsx`. `TestGuidedHistoryFromSparseConcernToCorrectedExport` now answers the app's two new questions before review; none of its assertions changed.
- [x] **C2** Red-flag questions for each active pathway are asked before review unless the user stops; a yes raises the emergency warning without the model; any still due are asked on the review screen and recorded in the summary. Development content (policy `development-5`, pathways `adult-development-v4`). Files: `intents.go`, `encounter.go`, `web/src/redflags.tsx`. Proof: `TestRedFlagCoverage`, the property test invariant "a yes to a red flag always warns".
- [x] **C3** App-owned question budget (12 model questions; then the question becomes an open item) and dedup by intent and by element; a background or app question already covered by the user's words or an earlier question is not asked. Files: `intents.go`, `model.go`. Proof: `TestQuestionBudget`; the regenerated scorecard records the question counts.
- [x] **M1** Explicit transition table (`transitions.go`): each action lists the states it may start from and end in; `Apply` refuses an action from any other state through one helper (`checkFrom`), and `docs/state-machine.md` is generated from the table. Proof: `TestTransitionTable` drives 1,200 random encounters (demo and scripted models) and checks every observed change against the table, and fails if the diagram is stale.
- [x] **M2** One field list per action (`commands.go`); `Apply` and the server refuse a command that sets a field its action doesn't use. The visit-preparation form now sends only its mode's fields. Proof: `TestTypedCommands`.
- [x] **M4** `Encounter` split into six sub-records (identity, interview state, history, safety, sources, assessments) whose fields are promoted, so the stored JSON keeps its shape; `payloadVersion` 2 and `DecodeEncounter` migrate older payloads and refuse newer ones. Files: `internal/clinical/types.go`, `payload.go`, `internal/store/store.go`. Proof: `TestPayloadMigration`.
- [x] **O2** One model call in flight per encounter (the queue never claims a second job for an encounter with one running); a new revision skips queued calls for older revisions and cancels a running one, so it stops spending tokens. Files: `internal/server/jobs.go`, `encounters.go`. Proof: `TestSupersededCallCancelledPostgres`.
- [x] **O3** Durable job table (`model_jobs`, migration 010) with a bounded worker pool (`CLINICAL_MODEL_WORKERS`, default 2), panic recovery that keeps the history and offers a retry, an attempt limit, and resume after restart. Proof: `TestJobQueuePostgres` (bounded concurrency, resume, panic), `TestAsyncInterviewCommittedBeforeInferenceAndReplayPostgres`.
- [x] **O4** Reasoning-aware API adapters: an owner setting marks a reasoning model, and the app then sends effort by task (OpenAI `reasoning.effort`, Anthropic `output_config.effort` beside the output format), streams the call and allows four minutes; OpenAI and Anthropic always get a 16,000-token ceiling because current models reason by default (the answer stays bounded by its byte limit); a provider that hasn't started answering is reported as a time-out, not a connection failure. The existing request-shape test now expects the per-provider budget. Files: `internal/provider/protocol.go`, `client.go`, `stream.go`, `chatgpt_stream.go`, `web/src/forms.tsx`. Proof: `TestReasoningRequestShapes`, `TestAnthropicStreamAssembly`, `TestOpenAIResponsesStream`, `TestNoAnswerYetIsATimeout`.
- [x] **O5** The connection test runs a tiny synthetic interview through the real validator. Files: `internal/server/settings.go`. Proof: `TestClinicalConnectionProbe`; transport errors leave the probe inconclusive, contract failures fail the test with a reason code.
- [x] **O6** A capability record per provider and model, saved with results. Proof: `TestCapabilityRecorded`.
- [x] **A1** App-owned assessment rendering: names on the app's condition list (`conditions.go`, development content) get the app's name, clinical term and description, labeled "development, not clinically reviewed" because no reviewed reference material exists yet; other names stay, labeled as the model's own suggestion; the rationale is built from the cited facts. Files: `internal/clinical/conditions.go`, `model.go`, `web/src/report.tsx`. Proof: `TestAssessmentRendering`.
- [x] **A2** "Don't miss" lines per pathway (development content): when an assessment leaves out a dangerous cause for an active pathway, the app adds a fixed "Ask your clinician about…" line, never a diagnosis. Proof: `TestDontMissLines`.
- [x] **E3** Natural-language user simulator harness, scored by app invariants: `evaluation.Answerer` with an offline `Natural` simulator (fillers, contractions, hedges, varied non-answers; the case's alert phrase is never changed) and a `ModelSimulator` that asks a separate model to answer as the synthetic person (live only, needs credentials and approval). `go run ./cmd/evaluate -simulate natural` runs it. Proof: `TestSimulatorHarness` (57 runs, every invariant held, 264 answers rephrased).
- [x] **E4** Committed scorecards regenerated (again at the end of this branch: 84 runs, 0 failed, policy `development-5`); CI fails on drift. Proof: `scripts/check_scorecard.py --drift` in CI; the baseline was regenerated in the evaluator image (adaptive questions 39 → 92 since guided background questions).
- [x] **F4** URL routing for deep links, Back and reload: pages and encounters have their own paths (`/encounters/<id>/<tab>`), the server serves the app for those paths only, and Back follows them. Files: `web/src/route.ts`, `web/src/main.tsx`, `internal/server/http.go`. Proof: `web/src/frontend.test.ts`, `TestDeepLinkServesApp`, Playwright "Links, Back and reload keep the user on the same screen".
- [x] **F5** No tab switch when a model turn finishes: a status notice says what is ready and offers a button to go there, so a correction in progress on another tab stays. Files: `web/src/turn.ts`, `web/src/encounter.tsx`. Proof: `web/src/frontend.test.ts` ("a finished turn never moves the user").
- [x] **F6** Accessibility: focus moves to a new question (an announced heading), answer choices are labelled by the question, radio groups are named and grouped, and axe (`@axe-core/playwright` 4.14.0) checks the journal, intake, interview and history review for serious and critical WCAG 2 A/AA problems. Proof: Playwright "Main screens pass automated accessibility checks".
- [x] **F7** One request path (`api`): every call times out, honors cancellation, reports an ended session so the app asks the user to sign in again, and refreshes the CSRF token once; `latest` drops stale responses; report uploads show progress and can be cancelled. Files: `web/src/api.ts`, `web/src/main.tsx`, `web/src/documents.tsx`. Proof: `web/src/frontend.test.ts`.
- [x] **F8** Polling backs off (1 s growing to 10 s) and pauses while the tab is hidden, for both the encounter and the journal. Files: `web/src/poll.ts`, `web/src/main.tsx`. Proof: `web/src/frontend.test.ts`.
- [x] **F9** Medicines and allergies in the brief come from recorded findings: the app sets each finding's category from the question it answered, a medicine entry or its concept, and the brief no longer searches quotes. Files: `internal/clinical/vocabulary.go`, `web/src/handoff.tsx`. Proof: `TestFactCategories`, `web/src/handoff.test.tsx`.
- [x] **D4** Key hierarchy: records are encrypted under a data key (migration 011); the installation key wraps it and a separate random receipt key, so rotation re-wraps two keys in one transaction and never rewrites records; an older installation keeps its original key as the data key and gets an independent receipt key once. Files: `internal/store/store.go`, `maintenance.go`. Proof: `TestReceiptKeyIndependentPostgres`, the existing rotation test in `TestChatGPT*`.
- [x] **D5** Secrets passed as files: the database reads its password from a Compose secret file by default; `compose.secrets.yaml` with `scripts/secret_files.py` passes the installation key and password to the read-only app and backup containers as files (Compose allows only file secrets in read-only services, and the app stays read-only); `main` and `backup.sh` read `*_FILE`. Document and speech readers (the speech reader previously inherited the whole environment) run through `internal/sandbox` with a fixed minimal environment and, on Linux where allowed, a private network namespace; the installation safety panel reports which isolation is in effect, because Docker's default profile blocks namespaces. Proof: `TestReaderEnvironment`, `tests/test_compose_secrets.py`.
- [x] **D6** `scripts/upgrade.py` takes and verifies a pre-upgrade snapshot before changing anything, keeps the previous image, and `--rollback` restores both; docs give the desktop rollback path and a PostgreSQL major-version plan. Proof: `tests/test_upgrade.py`, `docs/operations.md`.
- [x] **X1** Sign-in throttling per client (migration 012): another client is locked out after ten failures in ten minutes, a loopback connection is only slowed down, and `CLINICAL_TRUSTED_PROXY` gives each client behind a reverse proxy its own budget. Files: `internal/server/sessions.go`, `auth.go`. Proof: `TestLoginThrottlePostgres`; the original lockout assertion in `TestAuthenticatedPatientJourneyPostgres` still holds.
- [x] **X2** A public-origin installation needs a one-time setup code, printed once to the server log, before the owner account can be created; the sign-in form asks for it. Proof: `TestSetupTokenPostgres`.
- [x] **X3** Sessions end after an idle period (`CLINICAL_IDLE_MINUTES`, default 60), "End every session" signs out every device, and export, FHIR download, sharing, deletion and provider changes need a password entered in the last ten minutes; the app asks for it and retries the request. Files: `internal/server/sessions.go`, `web/src/reauth.tsx`, `web/src/api.ts`. Proof: `TestSessionPolicyPostgres`.
- [x] **X4** Every reader runs with time, CPU and address-space limits, and uploads and dictation are rate-limited per owner. Files: `internal/sandbox/sandbox.go`, `internal/server/ratelimit.go`. Proof: `TestReaderLimits`, `TestUploadAndVoiceRateLimits`.
- [x] **X5** Dependabot (`.github/dependabot.yml`) and a CI job that runs `govulncheck` v1.9.0 and `npm audit --audit-level=high`. Proof: the workflow. The local `govulncheck` run was blocked by this session's permission check and is listed under H7 with the exact command.
- [x] **R1** The macOS launcher now prefers the bundled prebuilt images over a source build and refuses to start images whose IDs differ from those recorded at release (`images.txt`, `desktop/verify-images.sh`); every base image is pinned by digest, every download by checksum, and the speech model by repository commit. The Windows launcher still builds from its bundled source until signed images are published (H4). Proof: `tests/test_release_pins.py`.
- [x] **R2** The legacy `windows-*` tag trigger and its release jobs are removed; Windows builds ship only through the paired release workflow. Proof: `.github/workflows/windows.yml`, `tests/test_release_pins.py`.
- [x] **R3** Both launchers offer Stop, Save logs, Delete this workspace and Remove all data (with confirmation); a macOS restore stops the previous workspace. Files: `desktop/manage.sh`, `desktop/Launcher.swift` (type-checked locally), `desktop/windows/Launcher.cs` (compiled by the Windows CI job). Proof: new checks in `scripts/verify_desktop.py`.
- [x] **P2** Docs split into a [user guide](docs/user-guide.md), an [operator guide](docs/operator-guide.md) and a [verification log](docs/verification-log.md) generated from the committed scorecard and corpus by `scripts/verification_log.py`. Proof: `scripts/check_docs.py` fails when a guide is missing or unlinked or the log is stale.
- [x] **P3** [CHANGELOG.md](CHANGELOG.md) and six [decision records](docs/decisions/README.md) for the safety and validator choices. Proof: `scripts/check_docs.py` requires them.

## P2: maintainability and polish

- [x] **S10** The emergency contact and care destination are validated at startup (plain one-line text; the contact must include a number), the locale and units already were, and every JSON export carries the settings in effect (`installation`). Files: `internal/server/startup.go`, `cmd/healthctl/main.go`. Proof: `TestStartupConfig`.
- [x] **V11** Controlled vocabulary for concepts (plain term plus an ICD-10-CM code where well established, development content); facts carry the term and code, and contradictions compare terms. Files: `internal/clinical/vocabulary.go`, `encounter.go`. Proof: `TestConceptVocabulary`.
- [x] **C4** "Why this question?" written by the app: intent questions use the catalog reason, free-text and report questions a fixed app sentence, and the model's own note is kept separately and shown only as a labeled suggestion. Files: `question_source.go`, `web/src/encounter.tsx`. Proof: `TestQuestionReasonAppAuthored`.
- [x] **M5** No display-time "patient"→"user" rewriting: the version 1→2 migration rewrites stored model wording once, and `ForDisplay` returns the record as stored. The two existing wording tests now read through the migration with unchanged assertions. Proof: `TestStoredWordingMigrated`, `TestRoleLabelsInOlderModelText`.
- [x] **M6** Special-case context regexes (possessive relatives, whole-reply denials, past-tense questions, colour denials, conditional and current report wording) moved into the context engine; the fainting rule was removed earlier. A static test parses the package and fails if a regex outside `context.go` decides negation, experiencer, time or hypotheticals. Proof: corpus + `TestNoSpecialCaseRegexes`.
- [x] **O7** Deterministic evidence selection near the input budget: instead of failing, the app leaves out older answers oldest first, always keeping the concern, emergency screens, answers behind active warnings, corrected findings and the newest eight answers; it never shortens the user's words and tells the model which answers were left out. `TestRefusesContextOverflowWithoutLosingOriginal` keeps its two properties (bounded input, nothing truncated) and now also covers the refusal when the kept evidence alone is too large. Files: `internal/clinical/budget.go`. Proof: `TestEvidenceSelection`.
- [x] **O8** Cost and wait time per encounter shown to the owner: model calls are linked to their encounter (migration 009), `GET /api/encounters/{id}/usage` adds them up, and the encounter details show calls, tokens, waiting time and a cost only when every call reported a price. Proof: `web/src/usage.test.ts`, `TestEncounterUsagePostgres`.
- [x] **A3** Optional stability check: the user can ask for the assessment to run twice; possibilities that appear in only one run are kept and marked uncertain, and a failed second run keeps the first result and says so. Files: `conditions.go`, `internal/server/encounters.go`, `web/src/history.tsx`. Proof: `TestAssessmentStability`.
- [x] **E6** Mutation testing for `internal/clinical`: 14 mutants of safety and control rules, applied with `go test -overlay` so source files are never touched; a mutant that doesn't compile counts as an error, not a kill. The first run found that no test reached the negation word limit; `TestNegationReachesOnlyAFewWords` and six run-on corpus rows now cover it. Proof: `python3 scripts/mutation_check.py` (14 of 14 killed), run by `check_hardening.py`.
- [x] **F10** Polish: every date goes through one formatter, clinical text areas have `autocomplete="off"`, the journal shows warning tags, dictation resamples to 16 kHz when Firefox won't open a 16 kHz context, and `npm run lint` enforces these rules plus no HTML from strings. Component splitting went only as far as the new pieces (`redflags`, `reauth`, `usage`, `release`); the large `encounter.tsx` and `documents.tsx` remain. Proof: `npm run lint`, `npm test` (29 tests).
- [x] **D7** Backups are kept by age (`CLINICAL_BACKUP_KEEP_DAYS`, default 30) with a floor of the newest `CLINICAL_BACKUP_KEEP`; every failure prints `backup: FAILED` and a one-shot run exits non-zero. Proof: `tests/test_backup_script.py`.
- [x] **X6** Trusted Types in the CSP (`require-trusted-types-for 'script'`; the app writes no markup from strings), and the health check reads its address from `CLINICAL_LISTEN`. Proof: `TestSecurityHeaders`, `TestHealthURLFollowsListenAddress`, browser suite under Chromium.
- [x] **R4** An update reminder computed in the browser from the release date, shown after 90 days with a link to the release page; the app never contacts a server to check. Files: `web/src/release.ts`. Proof: `web/src/release.test.ts`.
- [x] **R5** The default dictation model is Whisper `base.en` (q5_1, pinned by commit and SHA-256) instead of `tiny.en`; `go build ./...` works because a `go.mod` in the scratch `build/` folder keeps stray Go files out (the older files there were left in place). Proof: `tests/test_release_pins.py`, the `go build ./...` check in `check_hardening.py`.

## HANDOFF (needs a person)

Each item is prepared; the next steps are exact.

- [h] **H1** Clinical review of the urgent rules, pathways, red flags, completeness elements, condition list, descriptions and "don't miss" lines.
  1. Name a clinical lead and a backup reviewer.
  2. Review, in this order: `internal/clinical/safety.go` (urgent signs and levels), `intents.go` (red-flag questions, completeness elements, intent wording), `conditions.go` (condition list, descriptions, "don't miss" lines), `vocabulary.go` (terms and ICD-10-CM codes), `pathways.go`, and the corpus labels in `testdata/corpus`.
  3. Record the sign-off in `internal/clinical/policy_review.go` as a `PolicyRecord` for `development-5` with reviewer, date and scope, and set `DescriptionStatus` for reviewed descriptions; `TestPolicyReviewRecord` refuses a "reviewed" status without that record.
  4. Repeat for every policy version, and at least every three months.
- [h] **H2** Clinician-graded holdout as a release gate.
  1. A clinician writes at least 100 synthetic sentences without seeing the rules, labeled `warn` or `none` in the corpus format, into `internal/clinical/testdata/holdout/`.
  2. Run `CORPUS_DETAILS=1 go test -run TestCorpusHoldout -v ./internal/clinical` once and record the result here without tuning on it.
  3. Agree the release threshold (for example, no missed `emergency` sentence) and make `holdout()` in `scripts/check_hardening.py` fail below it.
- [h] **H3** Live-model baseline (needs your credentials and approval; it spends money).
  1. For each of two or three models: `MODEL_LIVE_KIND=openai MODEL_LIVE_MODEL=<model> OPENAI_API_KEY=<key> go run ./cmd/evaluate -live -release -repeat 3 -out build/live-<model>.json` (or `anthropic` with `ANTHROPIC_API_KEY`; set the provider's reasoning setting for reasoning models).
  2. Optionally `-simulate model` with `MODEL_SIMULATOR_ENDPOINT`, `MODEL_SIMULATOR_MODEL` and `MODEL_SIMULATOR_KEY`.
  3. Record pass rate, rejection reason codes, wait times and tokens in `docs/verification-log.md` (extend `scripts/verification_log.py` to read the live files).
- [h] **H4** Code signing and notarization for macOS and Windows.
  1. macOS: a Developer ID Application certificate; sign the app with `codesign --options runtime --timestamp`, submit with `xcrun notarytool submit --wait`, and staple; add these steps to `.github/workflows/release.yml` with the certificate in a protected secret.
  2. Windows: an Authenticode certificate; sign the EXE with `signtool sign /tr <timestamp URL> /td sha256 /fd sha256`.
  3. Publish signed, digest-pinned images to GHCR so the Windows launcher can load images instead of building from source (completes R1 for Windows).
- [h] **H5** Regulatory and license review.
  1. Ask a regulatory adviser whether the app is a medical device in each target market (for example FDA software-as-a-medical-device guidance, EU MDR rule 11) given the urgent warnings and assessments.
  2. Resolve the conflict between the license, which permits clinic use, and the beta notice, which says the app is not clinically reviewed; change the license or the notice before inviting any clinical use.
- [h] **H6** Backup and restore test on real Windows.
  1. On Windows 11 with Docker Desktop (WSL 2 engine), open the journal from the EXE and add a synthetic encounter.
  2. Back up with `docker compose -p <workspace> exec -T db pg_dump -Fc ...` as in `docs/operations.md`, then **Delete this workspace**, open the journal again, restore the dump with the same key, and compare the export.
  3. Check **Stop the workspace**, **Save logs** and **Remove all healthctl data**, and record the result in `docs/release.md`.
- [h] **H7** Local dependency scan (this session's permission check blocked it).
  1. Run `go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./cmd/... ./internal/... ./evaluation/...` and `cd web && npm audit --audit-level=high`.
  2. Fix or record any finding; the `dependencies` CI job runs the same checks on every push.

## Corpus baseline

Recorded on 2026-11-24 against commit `e6ec2dd` (before any rule changed), with `CORPUS_REPORT_ONLY=1 CORPUS_DETAILS=1 go test -run TestCorpus -v ./internal/clinical`.

| Metric | Baseline | Target |
|---|---|---|
| Rows | 552 (256 warn, 296 none) | 500+ |
| Urgent sentences missed | 205 of 256 (80%) | 0 |
| False alarms | 14 of 296 (4.7%) | reported |
| Correct extractions rejected | 129 of 595 (21.7%) | at most 5% |
| Must-reject extractions accepted | 36 of 53 | 0 |

Missed urgent rows by file: urgent 87, mixed 41, witness 30, uncertain 18, past 12, reply 8, third_party 4, hypothetical 4, routine 1. False alarms: past 5, hypothetical 4, negated 3, witness 2. Correct extractions rejected: routine 35, negated 28, witness 27, mixed 24, reply 8, third_party 4, past 2, urgent 1. The top rejection reasons were "Negative findings require explicit, unambiguous user evidence" (56), "Affirmative finding is unsupported by this answer" (45) and "Confirm who experienced the reported finding" (28). Must-reject extractions accepted: hypothetical 30, uncertain 2, urgent 1, routine 1, past 1, mixed 1.

## Corpus after the context engine

| Metric | Baseline | After development-4 |
|---|---|---|
| Rows | 552 | 656 (adds the two held-out sets below as lessons, and six run-on rows from the mutation check) |
| Urgent sentences missed | 205 of 256 | 0 of 324 |
| False alarms | 14 of 296 | 1 of 332 |
| Correct extractions rejected | 129 of 595 | 0 of 595 |
| Must-reject extractions accepted | 36 of 53 | 0 of 53 |

**Held-out results (honest generalization).** Each held-out set was written after the rules and measured before any tuning on it. Set 1: 24 of 34 urgent sentences missed and 4 of 21 false alarms. Set 2, after tuning on set 1: 20 of 27 urgent sentences missed and 1 of 13 false alarms. Both sets were then added to the corpus as `lessons.txt` and `lessons-2.txt`. Keyword rules recognize only the phrasings they were shown. The model's urgent flags (S5) and the explicit emergency question carry the safety load, and clinical review (H1) is still required. **Final untouched set (set 3).** Written after every rule change on this branch and measured once, without tuning (`testdata/holdout/holdout-3.txt`): 12 of 28 urgent sentences missed and 0 of 12 false alarms. The misses are new phrasings ("an elephant is sitting on it", "the whole bottle of my sleeping pills", "no urine at all since yesterday"). Better than sets 1 and 2, and still far from enough: the explicit safety questions, the red-flag questions, the model's urgent flags and clinical review (H1, H2) remain essential. `check_hardening.py` reports this set as information.

## Label changes

None. No corpus label was changed after the baseline.

## Final full run

On 2026-11-24, `python3 scripts/check_hardening.py` at commit `5d6b666` exited 0:

```text
[PASS] go vet ./cmd/... ./internal/... ./evaluation/... clean
[PASS] go build ./... go build ./... ok
[PASS] go test -race ./cmd/... ./internal/... ./evaluation/... 13 packages passed
[PASS] PostgreSQL integration tests 19 PostgreSQL tests passed, 0 skipped
[PASS] web tsc, vite build and unit tests tsc clean, vite build ok, 29 web unit tests passed
[PASS] corpus metrics 0 of 324 urgent sentences missed, 0 of 595 correct extractions rejected (0.0%), 0 of 53 must-reject accepted, 1 of 332 false alarms
[PASS] one urgent sentence through every input path 15 encounter input paths warned (corrected answer, corrected finding, diary entry in a visit, episode log, intake agenda, intake background, inta…
[PASS] random-action property test 1500 random sequences, 37500 steps, every invariant held
[PASS] held-out set informational: 12 of 28 untouched urgent sentences missed, 0 of 12 false alarms
[PASS] fresh release scorecard without drift no drift from the committed scorecard; Evaluated 84 interviews and 6 report comparisons.
[PASS] mutation check for internal/clinical 14 of 14 mutants killed
[PASS] scripts/check_docs.py Checked 154 relative links in 40 documents.
[PASS] Python script and study tests 22 script tests and the study tests passed
[PASS] python3 scripts/verify.py  Image clinical-history-qa-browser-tests Building
HARDENING.md summary: 77 done, 7 handoff, 0 open outside HANDOFF
[PASS] HARDENING.md has no open items outside HANDOFF 77 done, 7 handoff (H1, H2, H3, H4, H5, H6, H7), 0 open
ALL CHECKS PASSED
```

Earlier full runs that day each failed one environment-specific check, fixed before this run: the race-enabled container suite needed a ten-minute limit, the Go build stage lacked the generated state diagram, a queue test needed longer waits on a loaded machine, and a new button's name collided with an existing browser-test locator.
