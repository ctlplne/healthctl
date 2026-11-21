# Implementation handoff

The authoritative product brief is PRD.md. The build followed three steps: complete the authenticated patient journey, integrate all adapters and safety/version controls, then rebuild and verify the local deployment.

The scoped MVP is implemented. The personal installation is healthy on http://127.0.0.1:8437 and awaits owner registration. QA used a separate synthetic database. The final application binary, frontend assets and shipped license files matched the tested QA runtime byte for byte.

All race-enabled software and PostgreSQL integration checks passed. Eight desktop/mobile browser journeys passed. Four printed reports were checked for warnings, review status and pagination. A fresh-volume encrypted backup/restore drill passed, as did the 38-run deterministic synthetic evaluation. The per-case results and reproduction commands are linked in docs/verification.md.

The live remote check was skipped because there was no configured remote model and credential combination. Real local model inference was outside this milestone's live-test scope. Clinical and jurisdiction-specific review, operator deployment decisions, and the licensor's BUSL release parameters remain open before real-world distribution.

## Combined iteration 2 and 3

The local build follows the [combined roadmap](docs/iteration-2-3-roadmap.md). It includes report-first locally extracted documents, individual source corrections, selectable report-guided interviewing, dated history and reconciliation, passage-level local evidence, a clinician brief, follow-up, scoped sharing and a FHIR R4 document. The owner can configure a direct, optional SMART R4 portal connection for patient-selected lab results. Model guidance is visible throughout the app and in its exports.

The final images passed 73 top-level Go checks with the race detector and 28 desktop/mobile browser journeys. The reminder appeared on all 19 pages of 11 PDF exports. All 21 release-drill checkpoints passed, including upgrade from the retained v1 binary, rollback, offline FHIR validation, backup/restore, rotation and owner recovery. The 84 scripted interviews and six report comparisons retain individual outcomes. The adaptive fixture missed the positional dizziness target in one case. [Verification and PRD section 16 map](docs/combined-verification.md) · [EHR scope and qualification](docs/ehr.md)

The personal and QA runtimes match across the 22 shipped files. The personal app listens at http://127.0.0.1:8437 and is awaiting owner registration. No remote model was configured, no real patient portal was registered, and no patient data or application was published. Live model and vendor sandbox checks, independent clinical/usability review, and the distribution decisions remain open.
