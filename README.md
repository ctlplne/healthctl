# healthctl

A [ctlplne](https://github.com/ctlplne) project maintained solely by [@imfeelingtheagi](https://github.com/imfeelingtheagi).

A personal, self-hosted application for an adaptive symptom interview, a patient-correctable history, and an experimental, patient-visible provisional differential. Patients can begin with a medical report, review the extracted passages, and choose which entries inform the interview. The workspace includes a personal timeline, local evidence lookup, a visit brief and post-visit outcomes. It supports English-language adult encounters. All estimates are AI-generated and unreviewed by a healthcare professional. The synthetic demo uses a fictional scripted case.

## Start a local installation

From this repository, with Python 3 and Docker Compose v2 installed, run:

```sh
python3 scripts/init.py
docker compose up -d --build --wait
```

Open [healthctl on this computer](http://127.0.0.1:8437). Create the owner account. The initialization script creates a private `.env` with an installation-specific random database password and data encryption key. Subsequent runs preserve it. The first build needs access to the pinned base images and package registries. After installation, the application and a separately installed local model can operate offline.

1. Open provider settings. Select **Synthetic demo** for the authored example, or configure a model and test its connection.
2. Start an encounter, review the destination and experimental notice, and provide the requested intake information. For the demo, use the supplied nasal-symptom concern, leave other background blank, then choose **Yesterday** and **After being outdoors** during the interview.
3. Review the history. Compare a finding with the original wording and correct it if needed. Confirm the current summary, then request the estimate. Insufficient histories can be exported and revisited.
4. Open the report to download JSON or print using the browser's Save as PDF destination. History changes create a new revision. Earlier estimates retain their model and revision.
5. Use the encounters list to pause and resume. Deletion removes the encounter from this installation's primary storage.

`docker compose stop` stops the application and database. `docker compose up -d` restarts them with existing data. A fresh installation is unclaimed until the first owner is registered. Complete setup from the host before enabling a reverse proxy.

## Start from a medical report

Choose **Start with a report** in the encounter journal, or open an existing encounter and add a document. The local reader accepts PDF, DOCX, UTF-8 text, pasted text, JPEG and PNG. Scanned pages use English OCR. Confirm whose report it is. Compare each relevant row with its original text or file, check the value, printed units and dates, and review the selected extracts and model destination. A report can be kept on the installation without sending any of its passages to the model.

For a reproducible fictional example select **Synthetic demo**, start with the concern **It burns when I urinate.**, and paste these rows. Confirm your selected report row is about the patient. The authored example uses **Trace** and **Before the sample**. Keep the other opening fields empty except required scope and notice responses.

```text
Test | Result | Collected
Leukocyte esterase | Trace | 2026-10-09
```

The full appendix contains the original history and revisions. The clinician brief and FHIR document use the current handoff. The optional local brief consumer requires a revocable token for the specific confirmed record. [Report formats and local integration](docs/handoff.md) · [Combined release verification](docs/combined-verification.md)

An optional SMART on FHIR patient portal connector brings selected laboratory results into the same source review. The patient authorizes a direct read from a registered hospital and decides which rows enter the encounter. [EHR research, setup and data flows](docs/ehr.md)

## Model setup

Use the best model you have access to and select its highest available reasoning effort. Select the model in the app and configure the effort level where your provider supports it. The persistent reminder also appears in patient and clinician reports.

The **OpenAI-compatible endpoint** option connects to the operator’s explicitly allowlisted base URL. Use the model ID your server accepts. For a remote API, set the following values in the private `.env` file and recreate the app. This example uses a placeholder hostname.

```dotenv
CLINICAL_ALLOWED_ENDPOINTS=https://model.example/v1
BYOM_KEY_ENDPOINT=https://model.example/v1
BYOM_API_KEY=
```

```sh
docker compose up -d --force-recreate app
```

Use the same base URL in Settings. For OpenAI or Anthropic, set the appropriate server-side key in `.env`, recreate the app, select the provider, and enter an explicitly supported model ID. For local inference, choose the compatible adapter and configure a reachable LAN or Docker network address. Inside the app container, `localhost` points to the app container. Docker Desktop supports `host.docker.internal` for a host-side service; Linux operators can supply an explicit private address or a host-gateway mapping.

The server sends relevant reported evidence to the chosen model. External providers have their own data handling and retention practices. The demo runs in process. The application never requires a developer service. Read the adapter protocol, disclosure and live verification details in [Model providers](docs/model-providers.md).

## Operation and verification

- [Architecture and data boundaries](docs/architecture.md)
- [Installation security, backup and restore](docs/operations.md)
- [Reproducing checks and section 16 delivery evidence](docs/verification.md)
- [Clinical scope, limitations and release decisions](docs/release.md)
- [Unvalidated development safety policy and sources](internal/clinical/POLICY.md)
- [Authoritative requirements](PRD.md)

Each version of healthctl is source-available under the Business Source License 1.1 (`BUSL-1.1`) and converts to the Mozilla Public License 2.0 (`MPL-2.0`) on the fourth anniversary of that version’s first public distribution. [License text](LICENSE) · [Release parameters](docs/release.md#source-available-license)
