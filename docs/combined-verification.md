# Combined release verification

healthctl 0.2.0, local software verification, 21 November 2026. All records used for testing were fictional. The [original PRD](../PRD.md), the [combined roadmap](iteration-2-3-roadmap.md), and the [v1 verification record](verification.md) define the comparison. Independent clinical performance, human handoff outcomes and a real remote model remain open.

## Run locally

From the repository root:

```sh
python3 scripts/init.py
docker compose up -d --build --wait
docker compose ps
```

Open [the local app](http://127.0.0.1:8437). A fresh installation asks its owner to register. Choose the synthetic demo for either the v1 nasal-symptom journey or the [report-first urine-test example](../README.md#start-from-a-medical-report). Selecting an external model discloses the authorized clinical inputs to its configured endpoint. All storage, extraction, UI assets, evidence lookup and demo inference remain on the operator's installation.

## Reproduce verification

```sh
python3 scripts/verify.py
python3 scripts/verify_restore.py
docker build --target evaluator -t clinical-history:eval .
docker run --rm --network none --read-only --tmpfs /tmp clinical-history:eval -release > /tmp/clinical-release-scorecard.json
python3 -m unittest discover -s evaluation/study -v
docker build -f tests/fhir/Dockerfile -t clinical-history:r4-validator .
docker build -f tests/fhir/profile.Dockerfile -t clinical-history:r4-profiles .
python3 scripts/verify_release.py --profile-validator --study-pair
```

The upgrade drill uses the local image `clinical-history:verification-v1`, retained from the verified v1 build before the changes. On a different machine, build the original v1 source and tag its resulting app image with this name, or supply its local tag with `--v1-image`. The drill creates disposable projects on loopback ports 18441–18443. It never recovers, rotates or deletes the personal installation. The profile-validator image fetches public dependencies during its build. Validation of the synthetic document runs with Docker networking disabled.

The browser suite runs in Chromium at 1360×940 and 390×844. To exercise an actual screen reader after the browser suite:

```sh
docker build --file web/e2e/orca.Dockerfile -t clinical-history-qa-orca web/e2e
docker run --rm -i --network container:clinical-history-qa-app-1 --shm-size=512m clinical-history-qa-orca < .secrets/qa-owner
```

## Roadmap commitments and observed evidence

The subscription build passed 90 top-level Go tests with the race detector, including PostgreSQL, and 38 Chromium desktop/mobile journeys in 1.1 minutes. One opt-in live model test was skipped. Verification used synthetic accounts and cases by owner choice. The real browser portal flow checked the browser's network destinations, PKCE, same-session return, selection, replay, denial and a source-marked critical lab. Model guidance was visible during loading, setup, sign-in, scrolling and report review. Text extraction found it on all 19 pages of 11 browser-produced PDFs. The personal and QA containers matched across all 22 shipped files, including the application binary and frontend assets. The personal install is unclaimed and listens on loopback port 8437. [Final software and browser log](../build/chatgpt-final-qa.log) · [Runtime file hashes](../build/chatgpt-artifacts.json)

All 23 release-drill checkpoints passed, including the v1 upgrade and rollback, credential recovery, encrypted originals after restore, exclusion of the host-local subscription vault, and opening the restored urgent report in both desktop and mobile Chromium. The offline official R4 validator returned zero errors, zero warnings and one informational message. Matched handouts from both releases were used to prepare four masked practice packets. Independent clinician and patient readers are still needed. [Release drill](../build/release-drill.json) · [v1 printout](../build/study-pair/baseline.pdf) · [Combined clinician brief](../build/study-pair/brief.pdf)

| Commitment | Result |
|---|---|
| First run and model support | Before implementation, the v1 Go race/PostgreSQL suite and all eight desktop/mobile journeys passed. All three PRD adapter protocols remain covered. The additional ChatGPT subscription adapter has separate authorization and streaming fixtures. A local compatible HTTP fixture exercised the rebuilt app from intake to assessment, including reports and retry. Its synthetic origin is explicit. The live vendor test skipped because no provider/model/credential combination was configured. Model attempts, latency, retries, known usage and optional operator prices are recorded locally. Unknown costs are retained as unknown. [Model providers](model-providers.md) |
| Subscription-backed model selection | The public OpenAI discovery document advertised the issuer and endpoints used in the implementation. Local signed-token and streaming fixtures exercised registration, identity-only choice, plan permission, two workspaces, refresh, revocation, quota errors after partial output, model availability and highest advertised reasoning. Both browser sizes exercised the real app’s start, redirect and declined callback with the external navigation intercepted. Synthetic success paths cover linking and model selection. A real subscriber grant, actual model offer and billable inference remain unverified. [Subscription setup](model-providers.md#use-a-chatgpt-subscription) · [Protocol tests](../internal/planlogin/planlogin_test.go) · [Inference tests](../internal/provider/chatgpt_test.go) · [Owner and persistence tests](../internal/server/chatgpt_test.go) |
| Start from a report | Actual files went through the authenticated upload route. The drill used plain text, a tabular DOCX, digital PDF, image-only PDF, PNG and JPEG. It checked printed rows, units, reference ranges, collection date, OCR uncertainty, original byte identity, private attachment access and idempotent resubmission. Originals are encrypted separately. The review UI permits side-by-side comparison, correction, page selection and local transcription. [Release drill](../scripts/verify_release.py) |
| Grounding and consent | Compared the same fictional history with no report, a relevant urine report, an unrelated report and already-known timing. Separately exercised a wrong-patient document and a blurred decimal with missing units. A real loopback HTTP request proved the selected extract crossed the wire and the unselected identifier and row stayed local. Both engine-selected and model-selected questions can identify the specific approved review. Correction, withdrawal and changing model destinations retire dependent use and require the applicable review and disclosure. [Per-case results](../evaluation/release-fixture-results.json) |
| History, episodes, reconciliation and follow-up | A returning owner can search dated visits, name and join episodes, select a previous confirmed finding and record the date, source, current applicability and category. Medication, allergy, relative and personal history retain attribution. A documented outcome needs an exact source passage. A reported outcome stays labeled. The browser tests grouped two visits and reconciled a medication. The restore drill reproduced the post-visit record. |
| Interview, urgency and assessment revision | Four versioned development pathways cover respiratory symptoms, headache, abdominal symptoms and dizziness. Tests exercised mixed concerns, unknowns, declines, scoped interruptions and abstention. A printed critical finding interrupted an actual report interview. After review and deliberate patient choice, the fixture produced a differential with the warning retained in the brief, full JSON and FHIR. Browser printing also preserved active warnings. Revision comparison shows changed facts and possibilities. Independent clinicians still need to review the content. [Clinical policy](../internal/clinical/POLICY.md) |
| Local medical evidence | Eight passages from seven agency pages are bundled with source, rights, date, population, applicability and review status. General questions, ranked title/section/body searches, supported features, related reading and abstention are distinct. PostgreSQL tests installed operator content, detected version collisions, withdrew and rolled back versions and annotated older citations. An operator must hold applicable rights for each added pack. [Evidence sources and maintenance](evidence.md) |
| Visit brief and patient handoff | Separate patient appendix, one-page clinician print layout and scoped JSON. Browser tests produced actual PDFs for ordinary and corrected urgent fictional encounters. The brief includes uncertainty, priorities, original sources and unresolved questions. The timed, blinded study instrument passed a scripted rehearsal. No patient comprehension or clinician time saving has been measured. [Study instrument](../evaluation/study/README.md) |
| Local API and interoperability | An independent Python consumer read an approved synthetic brief with a 24-hour scoped token. Explicit revocation and record changes denied the old token. FHIR validation covered the R4 document Bundle and named base profiles, reference resolution, warning text, mapping limitations and source attribution. Diagnostic hypotheses remain preliminary narrative. [Local API and resource map](handoff.md) |
| Patient portal import | The self-host can register one SMART R4 public client and let a patient select laboratory results. Local verification includes a separate-origin fictional hospital, unselected and wrong-patient rows, prior authorization, portal refusal and explicit report review. Vendor qualification still requires a registered sandbox client and accepted callback. [Research, setup and data flows](ehr.md) |
| Operator and regional use | Real v1 installation, upgrade, rollback to a v1 snapshot under its original binary, fresh restore, offline key rotation and owner recovery. Prior sessions were excluded from restore and invalidated by recovery. All original files matched after restore and rotation. Scheduled backup with retention ran in a disposable project. Tested default emergency wording, US locale, and an English-India setup with a locally supplied emergency contact. [Operational commands](operations.md) |
| Accessibility and recovery | On the final QA runtime, Orca and Speech Dispatcher announced eight expected controls during sign-in, keyboard history confirmation, assessment, clinician brief and authenticated export. The audio monitor captured 143,874 non-silent samples. A throttled connection dropped an answer response after commit. Retrying saved the answer once. Independent patient comprehension remains to be measured. [Screen-reader run](../build/chatgpt-orca.log) |

## Section 16 of the original PRD

| Requirement | Evidence in this release |
|---|---|
| Deliverable source, scripts, container definitions, migrations, fixtures, prompts and documentation | Go application, React/TypeScript client, migrations 001–005, versioned provider contracts, local reader and evidence packs, QA images, Compose environment template, source fixture generator, reproductions above, [architecture](architecture.md), [operations](operations.md) and [release decisions](release.md). |
| Fresh installation and initialization | Rebuilt app and database started on new local volumes and the owner registered without default credentials. The true v1 upgrade and rollback reproduced the stored assessment. |
| Authenticated, key-free demo | Desktop and mobile journeys exercised owner sign-in, the two fictional cases, correction and print/export. |
| Real adaptive interviewing with a configured provider | The deployed compatible adapter passed the real local HTTP workflow. Vendor inference, real-model quality and its cost and latency are unverified. [Live check procedure](model-providers.md#reproduce-adapter-checks) |
| Patient-visible differential after history review | Synthetic supported results and insufficient-history results exercised. An active warning remains visible when the patient deliberately opens a supported result. |
| Correction and regeneration with version integrity | Correcting a fact, document row or attribution invalidated stale outputs. Late submissions, retries, history revision, former sources, changes and regeneration were exercised. |
| Versioned JSON and printable PDF with persistent warnings | Real downloads and PDFs exercised alongside the new brief, source appendix and validated R4 document subset. |
| Recover from provider failure without data loss | Race/PostgreSQL, real compatible endpoint failure fixture and browser recovery tested cancellation, rejection, interruption, retry and a dropped response after commit. |
| No required developer service | Self-hosted Docker application/database, bundled assets and evidence, local offline OCR and a network-isolated evaluation run. The optional scheduler and QA tools run locally. |
| Rebuild, exercise and document limitations | Rebuilt QA and personal Docker installations, reproducible evidence above, clinical scope and external gates below. Software regression results alone cannot validate medical performance. |

## Case-level evaluation and remaining gates

The expanded benchmark used the same deterministic model for opening-only, static-form and adaptive interviews on 28 cases, yielding 84 interview runs. Six additional report comparisons were exercised. All results retain per-case identifiers and separate fields for fabricated findings, stale assessments, lost warnings, report extraction, attribution and question behavior. The authored invariant checks recorded zero failures. Adaptive history captured 31 of 32 target facts, the static form captured 27, and opening text captured 23. The adaptive arm missed **positional dizziness** in `dizziness-medication-change`. The citation audit covered four differential items. These measurements describe the scripted model and cases. [Current local per-case scorecard](../build/chatgpt-synthetic-scorecard.json) · [Versioned fixture results](../evaluation/release-fixture-results.json) · [Evaluation method](../evaluation/README.md)

1. **Live model qualification.** Configure one remote model and its server-side credential or explicitly allowlisted compatible endpoint. Set the exact `MODEL_LIVE_KIND`, `MODEL_LIVE_MODEL`, and compatible endpoint settings in the test process environment. Run the following with fictional inputs. One passing case establishes connectivity and original-source handling. Run the agreed holdout and repetitions before drawing clinical or cost conclusions. Exact real model ID, latency, tokens and price remain unavailable. Subscription verification stopped at synthetic accounts by request. A future live subscription check requires the owner to authorize an eligible account, select an offered model and run the nonclinical connection probe.
2. **Independent clinical and usability evaluation.** Have a qualified clinical lead review the four pathways and urgent policy, agree criteria in advance, and freeze independent cases. Recruit independent clinicians and English-speaking patients for the blinded grading and timed reading/comprehension study. Report disagreements, missed and excessive alerts, unsupported citations, subgroup denominators, abandoned encounters, and measured cost per clinician-acceptable brief. [Study protocol](../evaluation/study/README.md)
3. **Distribution.** Settle the rights holder, Additional Use Grant covering production, embedding and third-party hosting, and version-specific public distribution/change dates. Each version is BUSL-1.1 converting to MPL-2.0 after four years. Confirm geography-specific intended use and regulatory/privacy review, deployment-specific model retention and emergency wording, rights for any additional content, content stewardship and incident-response responsibility. No external deployment or distribution was performed. [Release decisions](release.md)
4. **EHR vendor qualification.** Supply a registered public-client ID, accepted callback and synthetic sandbox patient for each candidate EHR. Verify the actual consent prompt, patient context, scopes, returned lab fields, omissions and cancellation at that institution. The local SMART portal verifies the software protocol and selection workflow. Epic, Oracle Health and athenahealth registrations and live portal behavior remain unverified. [EHR evaluation and setup](ehr.md)

```sh
MODEL_LIVE=1 go test -count=1 -v ./internal/provider -run '^TestLiveRemoteSynthetic$'
go run ./cmd/evaluate -live -release -case nasal-symptoms -out /tmp/clinical-remote-case.json
```
