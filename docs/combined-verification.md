# Combined release verification

healthctl 0.2.0, local software verification, 21 November 2026. All records used for testing were fictional. The [original PRD](../PRD.md), the [combined roadmap](iteration-2-3-roadmap.md), and the [v1 verification record](verification.md) define the comparison. Independent clinical performance, human handoff outcomes and clinical evaluation with a real remote model remain open.

The current release completed 46 desktop and mobile browser tests, checked model guidance on all 23 pages across 11 PDF exports, and passed 23 release and recovery checkpoints. The Go suite ran with the race detector and PostgreSQL. Local verification also exercised both installation paths. A clean Compose checkout completed the fictional demo and retained the assessment after restart. The Apple silicon disk image passed signature, offline speech, encrypted backup and restore checks. Earlier milestone results below retain their original counts.

## Run locally

From the repository root:

```sh
python3 scripts/init.py
docker compose up -d --build --wait
docker compose ps
```

Open [the local app](http://127.0.0.1:8437). A fresh installation asks its owner to register. Choose the synthetic demo for either the v1 nasal-symptom journey or the [report-first workflow](../README.md#bring-your-records). Selecting an external model discloses the authorized clinical inputs to its configured endpoint. All storage, extraction, UI assets, evidence lookup and demo inference remain on the operator's installation.

## Reproduce verification

After running `python3 scripts/init.py` in a fresh checkout, use these commands for the application tests, encrypted backup/restore, synthetic benchmark and study-instrument tests.

```sh
python3 scripts/verify.py
python3 scripts/verify_restore.py
docker build --target evaluator -t clinical-history:eval .
docker run --rm --network none --read-only --tmpfs /tmp clinical-history:eval -release > /tmp/clinical-release-scorecard.json
python3 -m unittest discover -s evaluation/study -v
```

### Upgrade from the original MVP

This additional drill requires a retained local v1 application image. A fresh clone contains the current source and cannot supply that historical binary. With the retained image tagged `clinical-history:verification-v1`, run the following commands. For a different local tag, pass `--v1-image YOUR_TAG` to the Python command.

```sh
docker build -f tests/fhir/Dockerfile -t clinical-history:r4-validator .
docker build -f tests/fhir/profile.Dockerfile -t clinical-history:r4-profiles .
python3 scripts/verify_release.py --profile-validator --study-pair
```

The drill creates disposable projects on loopback ports 18441–18443. It never recovers, rotates or deletes the personal installation. The profile-validator image fetches public dependencies during its build. Validation of the synthetic document runs with Docker networking disabled.

### Screen-reader check

The browser suite runs in Chromium at 1360×940 and 390×844. To exercise an actual screen reader after the browser suite:

```sh
docker build --file web/e2e/orca.Dockerfile -t clinical-history-qa-orca web/e2e
docker run --rm -i --network container:clinical-history-qa-app-1 --shm-size=512m clinical-history-qa-orca < .secrets/qa-owner
```

## Linked-plan user journeys and latency · November 21, 2026

All histories in these checks were fictional. A separate journal ran the production HTTP handlers, frontend and connected ChatGPT plan adapter for the Chromium timing comparison. The local installation's normal token resolver authorized inference.

| Journey | Observed result |
|---|---|
| Guided intake and pause/resume | The same fictional dizziness case took 56 seconds to show a model-written opening in Chromium. The revised flow showed an enabled, locally selected question in 78 ms. After the answer, the user left and reopened the journal. One model turn extracted lightheadedness and returned a follow-up 76 seconds after intake. The user confirmed the history and downloaded the current differential. The assessment was ready at 144 seconds from intake. |
| Opening question selection | Eight additional desktop and mobile timings covered onset, course, functional impact and the user's main concern. All eight questions were answerable in 54–62 ms and made zero opening model requests. Emergency, report-first and out-of-scope intakes each took their own branch on both viewports. |
| Correction and reassessment | A fictional shoulder complaint produced a provisional differential. Correcting the opening superseded the first estimate and the export identified the second assessment as current. |
| Urgent interruption | An explicit emergency interrupted the routine interview. After review and confirmation, the live model returned insufficient information. The urgent warning stayed active in the export. The local browser suite also checks a supported differential with an active warning. |
| Report-first interview | The first question linked to the reviewed, authorized urine-test row. The backend rejected one follow-up that omitted the required source reference. A fresh proposal passed validation, and the completed encounter exported a provisional differential. That interview operation took 232 seconds across two requests. |

At the guided-intake milestone, 99 top-level Go tests passed with the race detector, including PostgreSQL integration. The optional API-key live test was skipped. All 42 desktop and mobile browser tests passed, with model guidance on all 23 pages across 11 PDFs. Orca announced all eight expected controls in the keyboard journey through authenticated export. The 23 release and recovery checkpoints passed. Across 84 deterministic runs there were zero recorded fabricated findings, stale assessments or lost warnings. The adaptive fixture missed the positional dizziness target in `dizziness-medication-change`. The live Chromium case extracted lightheadedness, but its output has not been independently graded.

The new opening avoids a provider round trip. Subsequent model work still depends on the selected provider. Rejected, partial and unrepaired results cannot become user-visible assessments. A clinician-adjudicated holdout, user comprehension study and external production EHR exercise remain open.

## Roadmap commitments and observed evidence

The initial subscription build passed 90 top-level Go tests with the race detector, including PostgreSQL, and 38 Chromium desktop/mobile journeys in 1.1 minutes. One opt-in live model test was skipped. Verification used synthetic accounts and cases by owner choice. The real browser portal flow checked the browser's network destinations, PKCE, same-session return, selection, replay, denial and a source-marked critical lab. Model guidance was visible during loading, setup, sign-in, scrolling and report review. Text extraction found it on all 19 pages of 11 browser-produced PDFs. The personal and QA containers matched across all 22 shipped files, including the application binary and frontend assets. This run used the default Compose loopback port 8437. [Software and browser test runner](../scripts/verify.py)

All 23 release-drill checkpoints passed, including the v1 upgrade and rollback, credential recovery, encrypted originals after restore, exclusion of the host-local subscription vault, and opening the restored urgent report in both desktop and mobile Chromium. The offline official R4 validator returned zero errors, zero warnings and one informational message. Matched handouts from both releases were used to prepare four masked practice packets. Independent clinician and user readers are still needed. [Reproduce the upgrade drill and PDF pair](#upgrade-from-the-original-mvp) · [Prepare the masked packets](../evaluation/study/README.md)

| Commitment | Result |
|---|---|
| First run and model support | Before implementation, the v1 Go race/PostgreSQL suite and all eight desktop/mobile journeys passed. All three PRD adapter protocols remain covered. The additional ChatGPT subscription adapter has separate authorization and streaming fixtures. A local compatible HTTP fixture exercised the rebuilt app from intake to assessment, including reports and retry. Its synthetic origin is explicit. The live vendor test skipped because no provider/model/credential combination was configured. Model attempts, latency, retries, known usage and optional operator prices are recorded locally. Unknown costs are retained as unknown. [Model providers](model-providers.md) |
| Subscription-backed model selection | The public OpenAI discovery document advertised the issuer and endpoints used in the implementation. Local signed-token and streaming fixtures exercised registration, identity-only choice, plan permission, two workspaces, refresh, revocation, quota errors after partial output, model availability and highest advertised reasoning. Both browser sizes exercised the real app’s start, redirect and declined callback with the external navigation intercepted. Synthetic success paths cover linking and model selection. A subsequent owner-authorized nonclinical connection test used a real subscription and the offered GPT-6.2-Sol model. The endpoint returned HTTP 200 and a completed event stream without a parseable media type. After the adapter fix, the probe validated the expected ready response in 11.1 seconds. Subsequent fictional interviews exercised this route end to end. Independent assessment quality and representative costs remain open. [Subscription setup](model-providers.md#use-a-chatgpt-subscription) · [Protocol tests](../internal/planlogin/planlogin_test.go) · [Inference tests](../internal/provider/chatgpt_test.go) · [Owner and persistence tests](../internal/server/chatgpt_test.go) |
| Start from a report | Actual files went through the authenticated upload route. The drill used plain text, a tabular DOCX, digital PDF, image-only PDF, PNG and JPEG. It checked printed rows, units, reference ranges, collection date, OCR uncertainty, original byte identity, private attachment access and idempotent resubmission. Originals are encrypted separately. The review UI permits side-by-side comparison, correction, page selection and local transcription. [Release drill](../scripts/verify_release.py) |
| Grounding and consent | Compared the same fictional history with no report, a relevant urine report, an unrelated report and already-known timing. Separately exercised a wrong-user document and a blurred decimal with missing units. A real loopback HTTP request proved the selected extract crossed the wire and the unselected identifier and row stayed local. Both engine-selected and model-selected questions can identify the specific approved review. Correction, withdrawal and changing model destinations retire dependent use and require the applicable review and disclosure. [Per-case results](../evaluation/release-fixture-results.json) |
| History, episodes, reconciliation and follow-up | A returning owner can search dated visits, name and join episodes, select a previous confirmed finding and record the date, source, current applicability and category. Medication, allergy, relative and personal history retain attribution. A documented outcome needs an exact source passage. A reported outcome stays labeled. The browser tests grouped two visits and reconciled a medication. The restore drill reproduced the post-visit record. |
| Interview, urgency and assessment revision | Four versioned development pathways cover respiratory symptoms, headache, abdominal symptoms and dizziness. Tests exercised mixed concerns, unknowns, declines, scoped interruptions and abstention. A printed critical finding interrupted an actual report interview. After review and deliberate user choice, the fixture produced a differential with the warning retained in the brief, full JSON and FHIR. Browser printing also preserved active warnings. Revision comparison shows changed facts and possibilities. Independent clinicians still need to review the content. [Clinical policy](../internal/clinical/POLICY.md) |
| Local medical evidence | Eight passages from seven agency pages are bundled with source, rights, date, population, applicability and review status. General questions, ranked title/section/body searches, supported features, related reading and abstention are distinct. PostgreSQL tests installed operator content, detected version collisions, withdrew and rolled back versions and annotated older citations. An operator must hold applicable rights for each added pack. [Evidence sources and maintenance](evidence.md) |
| Visit brief and user handoff | Separate user appendix, one-page clinician print layout and scoped JSON. Browser tests produced actual PDFs for ordinary and corrected urgent fictional encounters. The brief includes uncertainty, priorities, original sources and unresolved questions. The timed, blinded study instrument passed a scripted rehearsal. No user comprehension or clinician time saving has been measured. [Study instrument](../evaluation/study/README.md) |
| Local API and interoperability | An independent Python consumer read an approved synthetic brief with a 24-hour scoped token. Explicit revocation and record changes denied the old token. FHIR validation covered the R4 document Bundle and named base profiles, reference resolution, warning text, mapping limitations and source attribution. Diagnostic hypotheses remain preliminary narrative. [Local API and resource map](handoff.md) |
| Health portal import | The self-host can register one SMART R4 public client and let a user select laboratory results. Local verification includes a separate-origin fictional hospital, unselected and wrong-user rows, prior authorization, portal refusal and explicit report review. Vendor qualification still requires a registered sandbox client and accepted callback. [Research, setup and data flows](ehr.md) |
| Operator and regional use | Real v1 installation, upgrade, rollback to a v1 snapshot under its original binary, fresh restore, offline key rotation and owner recovery. Prior sessions were excluded from restore and invalidated by recovery. All original files matched after restore and rotation. Scheduled backup with retention ran in a disposable project. Tested default emergency wording, US locale, and an English-India setup with a locally supplied emergency contact. [Operational commands](operations.md) |
| Accessibility and recovery | On the final QA runtime, Orca and Speech Dispatcher announced eight expected controls during sign-in, keyboard history confirmation, assessment, clinician brief and authenticated export. The audio monitor captured 143,874 non-silent samples. A throttled connection dropped an answer response after commit. Retrying saved the answer once. Independent user comprehension remains to be measured. [Reproduce the screen-reader check](#screen-reader-check) |

## Section 16 of the original PRD

| Requirement | Evidence in this release |
|---|---|
| Deliverable source, scripts, container definitions, migrations, fixtures, prompts and documentation | Go application, React/TypeScript client, migrations 001–006, versioned provider contracts, local reader and evidence packs, QA images, Compose environment template, source fixture generator, reproductions above, [architecture](architecture.md), [operations](operations.md) and [release decisions](release.md). |
| Fresh installation and initialization | Rebuilt app and database started on new local volumes and the owner registered without default credentials. The true v1 upgrade and rollback reproduced the stored assessment. |
| Authenticated, key-free demo | Desktop and mobile journeys exercised owner sign-in, the two fictional cases, correction and print/export. |
| Real adaptive interviewing with a configured provider | The compatible adapter passed the local HTTP workflow. The linked ChatGPT plan was exercised with fictional cases and measured in Chromium as documented above. Clinical quality and model cost per acceptable brief remain unverified. [Live check procedure](model-providers.md#reproduce-adapter-checks) |
| User-visible differential after history review | Synthetic supported results and insufficient-history results exercised. An active warning remains visible when the user deliberately opens a supported result. |
| Correction and regeneration with version integrity | Correcting a fact, document row or attribution invalidated stale outputs. Late submissions, retries, history revision, former sources, changes and regeneration were exercised. |
| Versioned JSON and printable PDF with persistent warnings | Real downloads and PDFs exercised alongside the new brief, source appendix and validated R4 document subset. |
| Recover from provider failure without data loss | Race/PostgreSQL, real compatible endpoint failure fixture and browser recovery tested cancellation, rejection, interruption, retry and a dropped response after commit. |
| No required developer service | Self-hosted Docker application/database, bundled assets and evidence, local offline OCR and a network-isolated evaluation run. The optional scheduler and QA tools run locally. |
| Rebuild, exercise and document limitations | Rebuilt QA and personal Docker installations, reproducible evidence above, clinical scope and external gates below. Software regression results alone cannot validate medical performance. |

## Case-level evaluation and remaining gates

The expanded benchmark used the same deterministic model for opening-only, static-form and adaptive interviews on 28 cases, yielding 84 interview runs. Six additional report comparisons were exercised. All results retain per-case identifiers and separate fields for fabricated findings, stale assessments, lost warnings, report extraction, attribution and question behavior. The authored invariant checks recorded zero failures. Adaptive history captured 31 of 32 target facts, the static form captured 27, and opening text captured 23. The adaptive arm missed **positional dizziness** in `dizziness-medication-change`. The citation audit covered four differential items. These measurements describe the scripted model and cases. [Versioned fixture results](../evaluation/release-fixture-results.json) · [Evaluation method](../evaluation/README.md)

1. **Live model qualification.** Configure one remote model and its server-side credential or explicitly allowlisted compatible endpoint. Set the exact `MODEL_LIVE_KIND`, `MODEL_LIVE_MODEL`, and compatible endpoint settings in the test process environment. Run the following with fictional inputs. One passing case establishes connectivity and original-source handling. Run the agreed holdout and repetitions before drawing clinical or cost conclusions. The linked-plan journeys above measured operation times and provider-reported token counts. Representative latency distributions and cost per acceptable brief remain unmeasured. The initial subscription verification used synthetic accounts. The later owner-authorized GPT-6.2-Sol connection probe verified one live nonclinical reply. A clinical holdout and independent grading are still required to evaluate this model for the intended use.
2. **Independent clinical and usability evaluation.** Have a qualified clinical lead review the four pathways and urgent policy, agree criteria in advance, and freeze independent cases. Recruit independent clinicians and English-speaking users for the blinded grading and timed reading/comprehension study. Report disagreements, missed and excessive alerts, unsupported citations, subgroup denominators, abandoned encounters, and measured cost per clinician-acceptable brief. [Study protocol](../evaluation/study/README.md)
3. **Distribution.** The 0.2.0 preview release record now identifies the rights holder, personal and internal-use grant, third-party hosting and commercial service terms, and conversion date. Each version is BUSL-1.1 converting to MPL-2.0 after four years. Confirm geography-specific intended use and regulatory/privacy review, deployment-specific model retention and emergency wording, rights for any additional content, content stewardship and incident-response responsibility. The source is public. Packaged distribution and a hosted deployment remain outside the local verification. [Release decisions](release.md)
4. **EHR vendor qualification.** Supply a registered public-client ID, accepted callback and synthetic sandbox user for each candidate EHR. Verify the actual consent prompt, user context, scopes, returned lab fields, omissions and cancellation at that institution. The local SMART portal verifies the software protocol and selection workflow. Epic, Oracle Health and athenahealth registrations and live portal behavior remain unverified. [EHR evaluation and setup](ehr.md)

```sh
MODEL_LIVE=1 go test -count=1 -v ./internal/provider -run '^TestLiveRemoteSynthetic$'
go run ./cmd/evaluate -live -release -case nasal-symptoms -out /tmp/clinical-remote-case.json
```

## Return visits, dictation and desktop verification

The local November 21 build exercises the episode diary, exact-word review, short interview, selected earlier findings and original reports, medication-label reconciliation, witness response, user-reviewed clinician feedback, and export. Selected earlier findings carry an explicit current status. The source contains clinical and PostgreSQL regressions for consent, idempotence, amended passages, original file recovery, historical attribution, the interview answer limit and retained warnings.

A live focused case used entirely fictional dizziness and medication history with the linked GPT-6.2-Sol plan. Before the short-flow change, the first remote follow-up took 136 seconds. Preview questions now run locally. The two question-selection steps together took under one millisecond in the Go process. The remote review took 95.4 seconds and the assessment took 82.2 seconds. Each made one request. The resulting export contained a current differential and retained the unresolved questions. These timings exclude user typing and do not measure browser rendering. An earlier live draft exposed an explicit fainting denial that the validator rejected. The regression preserves the exact user clause and rejects a denial qualified by “until today.”

The offline benchmark produced 84 runs over 28 authored cases plus six report comparisons. The run recorded zero fabricated findings, stale assessments or lost warnings. The adaptive fixture missed positional dizziness in the medication-change case. Run the release evaluator to inspect each case. The four-arm interview pilot rehearses allocation, timing and case-level failure reporting. Independent users, clinicians and a practice baseline are needed to measure clinical usefulness.

The macOS package drill uses an isolated installation and the bundled whisper.cpp reference clip. It verifies that offline speech returns an uncommitted draft, the original journal survives a failed restore, sessions require a fresh login, and the restored diary and complete demo assessment equal the originals. Browser tests exercise microphone capture, editable transcription, offline witness forms and clip-specific consent. Remote transcription was tested with a simulated platform endpoint. A live platform-key transcription check remains open. The linked plan is qualified for text inference only. See the [desktop recovery drill](desktop.md) and [transcription options](model-providers.md#dictation-and-account-billing).
