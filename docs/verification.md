# Verification and section 16 delivery

Verified locally on 21 November 2026 with fictional records. No patient history or application was published. PostgreSQL and the rebuilt Go/React application ran in Docker Compose. The browser suite used an isolated QA installation, including a separately labeled synthetic API double and the application’s own demo workflows. The personal installation remains ready for its owner to register.

## Reproduce

From the repository root, prepare a private installation configuration and run:

```sh
python3 scripts/init.py
docker compose up -d --build --wait
python3 scripts/verify.py
python3 scripts/verify_restore.py
go run ./cmd/evaluate -out /tmp/clinical-history-evaluation.json
go vet ./...
```

The verification script builds the Go runner with the race detector, runs it against PostgreSQL, rebuilds the QA app and synthetic compatible API, and runs Playwright on desktop and mobile. The two suites can also be run separately with `python3 scripts/verify.py backend` and `python3 scripts/verify.py browser`. The browser suite generates its own QA owner password in `.secrets/qa-owner` and uses loopback port 18437. Specify `CLINICAL_QA_PORT` if the port is in use. Test screenshots and PDFs are local, ephemeral artifacts in `build/test-results`.

`verify_restore.py` creates two temporary Compose projects on loopback ports 18439 and 18440, creates and assesses a fictional history, uses PostgreSQL custom-format backup and restore, signs in to the restored installation, and compares the full JSON reports excluding export time. It also verifies that existing sessions were excluded. It removes its two test volumes on exit. The single-owner application and the QA project keep separate databases.

The normal application requires Docker Compose and Python 3 for one-time initialization. Go 1.26 is needed for the last two host commands. The Go and browser build stages use pinned dependency manifests. The runtime uses a nonroot, read-only image. A command-line-only `go test ./...` skips database checks unless `CLINICAL_TEST_POSTGRES=1` is explicitly set; the Compose verification script enables them.

## Observed results

| Section 16 requirement | Evidence |
|---|---|
| Fresh installation and database initialization | Started new database volumes in independently named Compose projects. Both migrations ran. Owner registration worked with no default credentials; the restored owner remained intact. |
| Authenticated demo without API keys | Desktop and mobile exercised the demo from provider selection through a two-item provisional estimate. Missing live credentials and provider rejections had their own error paths. |
| Adaptive interviewing through an operator-configured endpoint | The rebuilt service exercised a compatible HTTP endpoint end to end, including a failed turn, successful retry and a follow-up question. Every provider protocol also passed fixture-based transport tests. External vendor service and real-model behavior remain unverified in this environment. |
| Patient-visible estimates after history review | The browser suite confirmed the history and displayed the ranked demo result. A separate compatible fixture generated a supported estimate after an urgent interruption, with the warning retained on screen and in both export formats. |
| Correction, regeneration and version integrity | Clinical and browser tests corrected individual findings, invalidated earlier results, accepted the revised summary, and generated a new result. Concurrent duplicate submissions persisted once. An old in-flight provider response failed its revision check after a correction. |
| Versioned JSON and printable reports | Authenticated JSON included original evidence, status, history and model revisions. Chromium generated actual PDFs on desktop and mobile. Extracted text contained the experimental notice and diagnostic result, and both urgent-case PDFs contained the active warning. |
| Failure recovery without data loss | Go integration and browser tests exercised a rejected provider request, reload, explicit retry, and a successful turn with the same input. Tests covered transport cancellation and deadlines, malformed replies, limits, interrupted server work, and no automatic provider substitution. |
| No mandatory developer infrastructure | The deployment consists of the app and PostgreSQL. JS, CSS, fonts and demo fixtures are bundled. The browser suite ran against local containers. Only an explicitly selected external model needs remote inference connectivity. |
| Source and operational deliverables | `cmd/`, `internal/`, `web/`, `evaluation/`, Dockerfiles, Compose files, `.env.example`, migration SQL, versioned prompts/schemas, policy notes, scripts, README and release documentation are in the project. |

The race-enabled Go suite passed, including domain validation, provider protocol fixtures, authentication, session limits, evidence integrity, ownership, encrypted storage, deletion and late-response handling against PostgreSQL. The opt-in live provider test was skipped. Static Go checks and the production TypeScript/Vite build passed. The fresh-volume backup/restore drill passed.

Eight Playwright journeys passed across 1360×940 and 390×844 viewports. They cover registration/sign-in, settings and destination disclosure, adaptive answers, corrected and outdated results, insufficient history, warning persistence, privacy boundaries, explicit destination changes, failure/retry, unsupported scope, JSON download, printing, resume and deletion.

## Synthetic evaluation

The 19 authored cases ran in two arms with the same deterministic fixture model. All 38 runs completed without failures in the authored checks for fabricated findings, stale assessments or lost warnings. The adaptive arm captured 26 of 26 scripted history targets in 31 follow-up answers. The opening-only arm captured 18 of 26. The model sees only information already volunteered or revealed in response to a question. The patient simulator, answer keys and exploratory reference diagnoses stay outside the model request. See [per-case results](../evaluation/fixture-results.json) and [evaluation methods and limitations](../evaluation/README.md).

## Live and release boundaries

No remote API credential/model combination or configured remote compatible endpoint was available for the live check. No local model inference engine was exercised, as requested for this milestone. The three actual HTTP adapters are implemented. Their test doubles establish wire, validation and error-handling behavior; the synthetic scores do not measure the clinical performance of a deployed model. The opt-in remote API check and blinded live evaluation commands are in [Model providers](model-providers.md) and [Synthetic evaluation](../evaluation/README.md).

Clinical review, real-world patient-facing evaluation, market-specific regulatory and privacy review, and final BUSL release parameters remain open. See [Release decisions](release.md).

For the final local check, all 22 files comprising the app binary, bundled frontend and shipped license files matched between the tested QA container and the personal installation. The final test log and hash comparison are in the workspace's ignored `build/` directory. Both app and database health checks passed. Only the loopback app port was published.
