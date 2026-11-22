# Implementation handoff

The [product brief](PRD.md) defines the original MVP. The current build adds report-guided and return visits, focused interviews, diary and dictation, and a macOS launcher. The November 21, 2026 verification covered 46 browser tests across desktop and mobile, 11 PDF exports, and 23 release and recovery checkpoints. The scripted adaptive interview still missed one positional dizziness finding. [Installation](README.md#install-healthctl) · [Current verification and open checks](docs/combined-verification.md)

## Original MVP

The scoped MVP is implemented. The original check used the default Compose loopback address. QA used a separate synthetic database. The final application binary, frontend assets and shipped license files matched the tested QA runtime byte for byte.

All race-enabled software and PostgreSQL integration checks passed. Eight desktop/mobile browser journeys passed. Four printed reports were checked for warnings, review status and pagination. A fresh-volume encrypted backup/restore drill passed, as did the 38-run deterministic synthetic evaluation. The per-case results and reproduction commands are linked in docs/verification.md.

The live remote check was skipped because there was no configured remote model and credential combination. Real local model inference was outside this milestone's live-test scope. Clinical and jurisdiction-specific review and operator deployment decisions remain open before real-world distribution. The subsequent [preview release record](docs/release.md) identifies the licensor and BUSL parameters.

## Combined iteration 2 and 3

The local build follows the [combined roadmap](docs/iteration-2-3-roadmap.md). It includes report-first locally extracted documents, individual source corrections, selectable report-guided interviewing, dated history and reconciliation, passage-level local evidence, a clinician brief, follow-up, scoped sharing and a FHIR R4 document. The owner can configure a direct, optional SMART R4 portal connection for user-selected lab results. Model guidance is visible throughout the app and in its exports.

The initial subscription build passed 90 top-level Go checks with the race detector and 38 desktop/mobile browser journeys. The reminder appeared on all 19 pages of 11 PDF exports. All 23 release-drill checkpoints passed, including upgrade from the retained v1 binary, rollback, offline FHIR validation, backup/restore, rotation and owner recovery. The 84 scripted interviews and six report comparisons retain individual outcomes. The adaptive fixture missed the positional dizziness target in one case. [Verification and PRD section 16 map](docs/combined-verification.md) · [EHR scope and qualification](docs/ehr.md)

The personal and QA runtimes match across the 22 shipped files. That run used the default Compose loopback port. The combined-release suite used a synthetic provider and portal. Live model and vendor sandbox checks, independent clinical/usability review, and the distribution decisions remain open.


## Personal subscription inference

**Continue with ChatGPT** links a local owner to a specific account and workspace. Separate consent enables inference from that account’s model catalog. healthctl requests the highest supported reasoning level advertised by the selected model. Credentials rotate under a database lock and are excluded from portable backups. Existing encounters require confirmation to change registrations. Claude personal subscriptions cannot fund third-party inference under Anthropic’s published terms. [Setup and provider references](docs/model-providers.md#use-a-chatgpt-subscription)

Verification finished with synthetic data at the owner’s request. Desktop and mobile browsers exercised the rebuilt start and declined-callback workflow. Signed local identity fixtures and a loopback streaming provider covered successful authorization, renewal, validated inference, quota failure and cancellation. A later owner-authorized connection test with GPT-6.2-Sol returned a validated nonclinical result in 11.1 seconds after fixing the handling of event streams with an absent or invalid media type. [Verification and open release gates](docs/combined-verification.md)

## Guided user history

New browser encounters begin with a short concern and the safety and scope screens. Prompt `clinical-v3` prioritizes one decision-changing question at a time and can bring a background question forward. For routine review, the application asks separately about medicines and supplements, allergies, ongoing conditions and prior operations. Users can provide details, report none, express uncertainty, mark a question inapplicable, skip, or go directly to history review. Older encounters retain their stored background. Answers, corrections and the specific scope of a denial travel with the history and export.

The local verification run passed 96 top-level Go tests and 40 desktop/mobile browser tests. It checked 23 pages across 11 PDF exports. Reproduce with `python3 scripts/verify.py`. The local runtime returned ready and matched the QA application binary and UI entry point.

Across 28 synthetic cases, the scripted adaptive arm recovered 31 of 32 target details, the static questionnaire recovered 27, and opening text alone recovered 23. The scripted extractor missed positional dizziness in `dizziness-medication-change`, where the opening already described it. Across all 84 runs, the scorer found no fabricated findings, stale assessments or lost warnings. Results are recorded per case in `build/guided-history-scorecard.json`. The revised prompt still needs live-model evaluation and independent clinician and user testing.
