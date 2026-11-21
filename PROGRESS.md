# Implementation handoff

The authoritative product brief is PRD.md. The build followed three steps: complete the authenticated patient journey, integrate all adapters and safety/version controls, then rebuild and verify the local deployment.

The scoped MVP is implemented. The personal installation runs on http://127.0.0.1:8437. QA used a separate synthetic database. The final application binary, frontend assets and shipped license files matched the tested QA runtime byte for byte.

All race-enabled software and PostgreSQL integration checks passed. Eight desktop/mobile browser journeys passed. Four printed reports were checked for warnings, review status and pagination. A fresh-volume encrypted backup/restore drill passed, as did the 38-run deterministic synthetic evaluation. The per-case results and reproduction commands are linked in docs/verification.md.

The live remote check was skipped because there was no configured remote model and credential combination. Real local model inference was outside this milestone's live-test scope. Clinical and jurisdiction-specific review, operator deployment decisions, and the licensor's BUSL release parameters remain open before real-world distribution.

## Combined iteration 2 and 3

The local build follows the [combined roadmap](docs/iteration-2-3-roadmap.md). It includes report-first locally extracted documents, individual source corrections, selectable report-guided interviewing, dated history and reconciliation, passage-level local evidence, a clinician brief, follow-up, scoped sharing and a FHIR R4 document. The owner can configure a direct, optional SMART R4 portal connection for patient-selected lab results. Model guidance is visible throughout the app and in its exports.

The initial subscription build passed 90 top-level Go checks with the race detector and 38 desktop/mobile browser journeys. The reminder appeared on all 19 pages of 11 PDF exports. All 23 release-drill checkpoints passed, including upgrade from the retained v1 binary, rollback, offline FHIR validation, backup/restore, rotation and owner recovery. The 84 scripted interviews and six report comparisons retain individual outcomes. The adaptive fixture missed the positional dizziness target in one case. [Verification and PRD section 16 map](docs/combined-verification.md) · [EHR scope and qualification](docs/ehr.md)

The personal and QA runtimes match across the 22 shipped files. The personal app listens at http://127.0.0.1:8437. The combined-release suite used a synthetic provider and portal. Live model and vendor sandbox checks, independent clinical/usability review, and the distribution decisions remain open.


## Personal subscription inference

**Continue with ChatGPT** links a local owner to a specific account and workspace. Separate consent enables inference from that account’s model catalog. healthctl requests the highest supported reasoning level advertised by the selected model. Credentials rotate under a database lock and are excluded from portable backups. Existing encounters require confirmation to change registrations. Claude personal subscriptions cannot fund third-party inference under Anthropic’s published terms. [Setup and provider references](docs/model-providers.md#use-a-chatgpt-subscription)

Verification finished with synthetic data at the owner’s request. Desktop and mobile browsers exercised the rebuilt start and declined-callback workflow. Signed local identity fixtures and a loopback streaming provider covered successful authorization, renewal, validated inference, quota failure and cancellation. A later owner-authorized connection test with GPT-6.2-Sol returned a validated nonclinical result in 11.1 seconds after fixing the handling of event streams with an absent or invalid media type. [Verification and open release gates](docs/combined-verification.md)
