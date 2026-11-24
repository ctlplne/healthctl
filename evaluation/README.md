# Synthetic clinical history evaluation

The 19 authored cases in `cases.json` exercise the application's real encounter transitions, evidence checks, correction history, warning retention, and report revisions. All people and symptoms here are fictional. Use this harness separately from the software unit/integration suites.

## Run

From the project root, run:

```sh
go run ./cmd/evaluate -out evaluation/fixture-results.json
go test ./evaluation
```

`fixture-results.json` contains 38 paired runs, two per case, produced using the deterministic fixture. The fixture recognizes a small authored vocabulary. Only the nasal-symptom scenario has an authored differential. An insufficient response for another case says nothing about whether a live model would produce a useful estimate. The reference rank is exploratory.

A real model run is separately opt-in. First configure the remote API credentials and model environment variables described in `docs/model-providers.md`. Then select an individual case and a separate output file:

```sh
go run ./cmd/evaluate -live -case nasal-symptoms -out /tmp/clinical-live-one.json
```

Removing `-case` selects all cases. A full comparison makes many model requests and may incur provider charges. The only clinical content sent consists of the authored synthetic disclosures. The live evaluator requires a public HTTPS model API. Operator allowlisting and endpoint-bound keys apply. The executable returns an error status when an invariant fails or the model cannot return a usable proposal. Results are written before this exit, including per-case failures. The number of permitted follow-up answers is configurable with `-max-turns` (default five).

## Blinding and comparisons

The model receives a `clinical.Input` containing only the same visible encounter fields used by the application. A separate user simulator holds undisclosed answers. Each answer has a topic, an optional targeted question pattern, and user wording. It is revealed in response to a question about that topic. Unmatched questions produce an uncertain response. A corrected opening is volunteered only in the explicitly authored correction case. Case IDs, reference diagnoses, scoring keys, prohibited findings, and future replies never enter the model input.

The free-text arm extracts from the opening alone, then requests an assessment. The adaptive arm permits follow-up questions before review. Cases with an explicit correction apply it in both arms. Both arms use the same configured downstream assessment model and application validator. Live responses may vary even for identical inputs. Each case also exercises the application's failed-assessment transition on a copy, then checks the persisted report representation after adding information. Full database and browser tests exercise storage and presentation separately.

## Interpret a result

| Field | Interpretation |
|---|---|
| `visibleTargetExtraction` | Correctly extracted authored targets divided by targets whose source answers were actually disclosed. |
| `keyHistory` and `missedTargets` | Captured key facts across the entire authored scenario, including facts never elicited. |
| `fabricatedFindings` | Unsupported source spans, wrong status or subject on an authored target, a known target attached to the wrong answer, or a case-specific explicitly prohibited claim. |
| `unadjudicatedFindings` | Claims beyond the authored rubric or with insufficient time information. They require human review and earn no extraction credit. |
| `contradictions` and `contradictionErrors` | Observed unresolved contradictions and disagreement with the authored expectation given information actually disclosed. |
| `checkpointAlerts`, `alertErrors`, `lostWarnings` | Alert observations at intake, follow-ups, correction, review and assessment, plus any lost warning across transitions, failure, or report export. |
| `staleAssessments` | A previously generated assessment remains current, disappears, or its serialized content changes after a history edit. |
| `interviewQuestions` and `modelCalls` | Follow-up answers elicited and model operations made by the harness. Transport retries may add HTTP requests. |
| `exploratoryReferenceRank` | One-based position of a possibility matching the authored term; zero means absent and null means no reference assigned. |
| `errors` | Model/validator failures and incomplete or unexpected workflow results. A failed run cannot verify subsequent checkpoints. |

Per-case results preserve all safety and revision failures even when other cases succeed. The scorer checks scripted terms and evidence attributes. It cannot adjudicate arbitrary clinical free text, rank disease probability, or establish safety or medical validity. The user simulator recognizes authored English topics and a limited set of synonyms. A live model can ask an appropriate question it does not recognize. The cases have not undergone independent clinical adjudication.

## Combined release benchmark

The expanded benchmark adds nine fictional cases across respiratory symptoms, headache, abdominal concerns, dizziness, medication changes, unsupported age and pregnancy-related care. It retains the original 19 case records. The local release scorecard exercises all 28 with an opening-only arm, adaptive interview and a fixed three-question form. All three use the same downstream model. The fixed form asks timing, associated symptoms and medication history, while honoring an active interruption. The 19-case historical fixture baseline is included separately. A synthetic case is an engineering regression scenario; independent clinical adjudication and a pre-registered clinical holdout are pending.

```sh
go run ./cmd/evaluate -release -out /tmp/combined-scorecard.json
```

For the same OCR-enabled run on any Docker host, execute these commands from the repository root. The evaluator uses the application’s local reader dependencies and receives only the bundled fictional records.

```sh
docker build --target evaluator -t clinical-history:eval .
docker run --rm --network none --read-only --tmpfs /tmp clinical-history:eval -release > /tmp/combined-scorecard.json
```

Run the host command on a machine with Tesseract English to exercise the blurred image. If missing, the scorecard marks that comparison `tool_unavailable`. With explicitly configured vendor credentials, use `-live -release -case nasal-symptoms` to qualify one synthetic case. Use `-repeat N` (1–10) to examine variability. Omitting `-case` runs the full suite plus report comparisons and may generate many billable calls. The test runner discloses no hidden case reference or future answer. Use fresh filenames for live runs.

The committed [release fixture scorecard](release-fixture-results.json) was generated in the project's local OCR reader image with networking disabled. It contains 84 synthetic arm runs and six additional report comparisons. The adaptive fixture elicited 31 of 32 authored targets in 92 questions, the fixed form 27 in 56 questions, and the opening-only arm 23. The adaptive count grew from 39 when guided background questions (medicines, allergies, conditions and operations) were added. CI regenerates this scorecard in the evaluator image and fails if any deterministic field differs from the committed copy (`scripts/check_scorecard.py --drift`). The adaptive script missed positional dizziness in the medication-change case. No case failed the fabricated finding, stale assessment, lost warning, or specified alert checks. The Gaussian-blurred synthetic image's decimal was read, all three OCR rows remained uncertain, and the missing unit remained empty. The digital panel kept specimen and issue dates distinct. The relevant and unrelated report arms used the same synthetic user complaint and assessment implementation.

The scorecard lists case-level errors, missed/excessive alert checkpoints, repeated questions, elapsed software time, abstentions, provider attempts and operator-estimated token costs. Machine-generated latency excludes human reading and typing time. The historical baseline predates latency instrumentation. Real-model accuracy, agreement between independent clinicians, user comprehension, actual abandonment, clinician reading time, infrastructure cost, content-maintenance cost, and cost per clinician-acceptable brief remain unmeasured.

Each run records a case, arm, repetition, configuration fingerprint, applied model metadata, actual returned identifiers, and microsecond elapsed time. The scorecard reports median and high-end automated times only when instrumented. Price completeness includes every attempted model operation. A per-completed-encounter figure requires complete returned usage and operator-supplied matching rates. The offline double performs no HTTP inference. Its price fields are unavailable. Run the [local independent review and timed reading instrument](study/README.md) with approved held-out cases and qualified reviewers to measure human outcomes.

The combined run produced four differential-item citation audits. Three linked directly quoted condition features to eligible self-reported facts in an applicable educational source. One explicitly returned evidence unavailable. Most authored histories produced abstention from the scripted downstream model, so this count supplies limited coverage. Each item records its source version and fact identifiers. Negative tests exercise denial, family history, report-only measurements, outdated sources and age mismatch.

## Simulated users

`go run ./cmd/evaluate -simulate natural` answers every case with an offline simulator that rephrases the case's replies the way people type: fillers, contractions, hedges and varied non-answers. The case's alert phrase is never changed. The app's own invariants score each journey: no fabricated finding, lost or wrong warning, stale assessment or wrong contradiction. `-simulate model` asks a separate model to answer as the synthetic person instead; it needs `MODEL_SIMULATOR_ENDPOINT`, `MODEL_SIMULATOR_MODEL` and `MODEL_SIMULATOR_KEY` and sends the case's synthetic replies to that endpoint, so run it only with the maintainer's approval. A model never grades the result.
