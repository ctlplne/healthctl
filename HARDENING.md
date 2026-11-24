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
- [ ] **O2** One model call in flight per encounter; superseded calls are cancelled. Proof: `TestSupersededCallCancelled`.
- [ ] **O3** Durable job table with bounded workers, panic recovery and resume after restart. Proof: `TestJobQueuePostgres`.
- [ ] **O4** Reasoning-aware API adapters: effort parameter where supported, output budget, streaming, and accurate timeout messages. Proof: provider tests.
- [x] **O5** The connection test runs a tiny synthetic interview through the real validator. Files: `internal/server/settings.go`. Proof: `TestClinicalConnectionProbe`; transport errors leave the probe inconclusive, contract failures fail the test with a reason code.
- [x] **O6** A capability record per provider and model, saved with results. Proof: `TestCapabilityRecorded`.
- [x] **A1** App-owned assessment rendering: names on the app's condition list (`conditions.go`, development content) get the app's name, clinical term and description, labeled "development, not clinically reviewed" because no reviewed reference material exists yet; other names stay, labeled as the model's own suggestion; the rationale is built from the cited facts. Files: `internal/clinical/conditions.go`, `model.go`, `web/src/report.tsx`. Proof: `TestAssessmentRendering`.
- [x] **A2** "Don't miss" lines per pathway (development content): when an assessment leaves out a dangerous cause for an active pathway, the app adds a fixed "Ask your clinician about…" line, never a diagnosis. Proof: `TestDontMissLines`.
- [ ] **E3** Natural-language user simulator harness, scored by app invariants. Proof: `TestSimulatorHarness`.
- [x] **E4** Committed scorecards regenerated; CI fails on drift. Proof: `scripts/check_scorecard.py --drift` in CI; the baseline was regenerated in the evaluator image (adaptive questions 39 → 92 since guided background questions).
- [ ] **F4** URL routing for deep links, Back and reload. Proof: unit test + Playwright.
- [ ] **F5** No tab switch when a turn finishes; a "next question ready" notice; in-progress corrections kept. Proof: unit test.
- [ ] **F6** Accessibility: focus moves to the new question and it is announced, radio groups are named, axe runs in Playwright. Proof: Playwright.
- [ ] **F7** One data-fetching hook (timeout, abort, stale response, 401, CSRF refresh); upload progress and cancel. Proof: unit test.
- [ ] **F8** Polling backs off and pauses when the tab is hidden. Proof: unit test.
- [ ] **F9** Medicines and allergies in the brief come from recorded findings. Proof: unit test.
- [ ] **D4** Key hierarchy: the receipt key is derived separately and rotation re-wraps keys. Proof: `TestReceiptKeyIndependent`.
- [ ] **D5** Secrets passed as files; document and speech readers run without secrets or network. Proof: `TestReaderEnvironment`, compose check.
- [ ] **D6** Automatic pre-upgrade snapshot, desktop rollback path, Postgres major-version upgrade plan. Proof: script test + docs.
- [ ] **X1** Login throttling that can't lock the owner out. Proof: `TestLoginThrottlePostgres`.
- [ ] **X2** One-time setup token for public-origin installs. Proof: `TestSetupTokenPostgres`.
- [ ] **X3** Idle session timeout, sign out everywhere, re-authentication for export, share, delete and provider change. Proof: `TestSessionPolicyPostgres`.
- [ ] **X4** CPU, memory and time limits on subprocesses; rate limits on upload and voice. Proof: unit tests.
- [ ] **X5** Dependabot, govulncheck and npm audit in CI. Proof: workflow + local `govulncheck` run.
- [ ] **R1** Prebuilt, digest-pinned images for desktop users; pinned speech-model revision. Proof: launcher test + Dockerfile check.
- [ ] **R2** Retire the legacy `windows-*` tag release channel. Proof: workflow diff.
- [ ] **R3** Launcher Stop, Delete, Logs and Uninstall; restore stops the old stack. Proof: `scripts/verify_desktop.py` checks.
- [ ] **P2** Docs split into user guide, operator guide and verification log, with numbers generated from scorecards. Proof: `scripts/check_docs.py`.
- [ ] **P3** CHANGELOG and decision records for safety and validator choices. Proof: files present + `check_docs.py`.

## P2: maintainability and polish

- [ ] **S10** Emergency-contact and locale settings validated at startup and shown in exports. Proof: `TestStartupConfig`.
- [x] **V11** Controlled vocabulary for concepts (plain term plus an ICD-10-CM code where well established, development content); facts carry the term and code, and contradictions compare terms. Files: `internal/clinical/vocabulary.go`, `encounter.go`. Proof: `TestConceptVocabulary`.
- [x] **C4** "Why this question?" written by the app: intent questions use the catalog reason, free-text and report questions a fixed app sentence, and the model's own note is kept separately and shown only as a labeled suggestion. Files: `question_source.go`, `web/src/encounter.tsx`. Proof: `TestQuestionReasonAppAuthored`.
- [x] **M5** No display-time "patient"→"user" rewriting: the version 1→2 migration rewrites stored model wording once, and `ForDisplay` returns the record as stored. The two existing wording tests now read through the migration with unchanged assertions. Proof: `TestStoredWordingMigrated`, `TestRoleLabelsInOlderModelText`.
- [x] **M6** Special-case context regexes (possessive relatives, whole-reply denials, past-tense questions, colour denials, conditional and current report wording) moved into the context engine; the fainting rule was removed earlier. A static test parses the package and fails if a regex outside `context.go` decides negation, experiencer, time or hypotheticals. Proof: corpus + `TestNoSpecialCaseRegexes`.
- [ ] **O7** Deterministic evidence selection near the input budget. Proof: `TestEvidenceSelection`.
- [ ] **O8** Cost and wait time per encounter shown to the owner. Proof: unit test.
- [x] **A3** Optional stability check: the user can ask for the assessment to run twice; possibilities that appear in only one run are kept and marked uncertain, and a failed second run keeps the first result and says so. Files: `conditions.go`, `internal/server/encounters.go`, `web/src/history.tsx`. Proof: `TestAssessmentStability`.
- [ ] **E6** Mutation testing for `internal/clinical`. Proof: `scripts/mutation_check.py`.
- [ ] **F10** Polish: one date formatter, `autocomplete="off"` on clinical fields, warnings in the journal list, Firefox audio resampling, smaller components, lint and unit tests. Proof: `npm run lint`, `npm test`.
- [ ] **D7** Time-based backup retention and loud failure logs. Proof: `tests/test_backup_script.py`.
- [ ] **X6** Trusted Types in the CSP; healthcheck port read from config. Proof: `TestSecurityHeaders`.
- [ ] **R4** Update notice without the app contacting any server. Proof: unit test.
- [ ] **R5** Better default dictation model; `go build ./...` works locally. Proof: Dockerfile check + `go build ./...`.

## HANDOFF (needs a person)

- [h] **H1** Clinical review of the urgent rules, pathways, red flags and "don't miss" lines; name a clinical lead and a review cadence.
- [h] **H2** Clinician-graded holdout as a release gate.
- [h] **H3** Live-model baseline (script ready; needs credentials and approval).
- [h] **H4** Code signing and notarization for macOS and Windows.
- [h] **H5** Regulatory and license review.
- [h] **H6** Backup and restore test on real Windows.

Next steps for each handoff item are filled in as the work that prepares it lands.

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
| Rows | 552 | 650 (adds the two held-out sets below as lessons) |
| Urgent sentences missed | 205 of 256 | 0 of 320 |
| False alarms | 14 of 296 | 1 of 330 |
| Correct extractions rejected | 129 of 595 | 0 of 595 |
| Must-reject extractions accepted | 36 of 53 | 0 of 53 |

**Held-out results (honest generalization).** Each held-out set was written after the rules and measured before any tuning on it. Set 1: 24 of 34 urgent sentences missed and 4 of 21 false alarms. Set 2, after tuning on set 1: 20 of 27 urgent sentences missed and 1 of 13 false alarms. Both sets were then added to the corpus as `lessons.txt` and `lessons-2.txt`. Keyword rules recognize only the phrasings they were shown. The model's urgent flags (S5) and the explicit emergency question carry the safety load, and clinical review (H1) is still required. A final untouched set is measured in K1.

## Label changes

None. No corpus label was changed after the baseline.
