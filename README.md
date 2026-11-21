# healthctl

healthctl is a self-hosted tool for taking a medical history before a visit. Start with a symptom in your own words or bring a lab report. The interview asks one question at a time. You can see the words or report entries behind a finding, correct what was misunderstood, and take a concise history to your clinician.

The symptom interview is designed for English-speaking adults. After you confirm your history, you can ask the model for possible explanations. It generates an experimental estimate without clinician review. If you may be having a medical emergency, seek local emergency care immediately.

## Install healthctl

There are two self-hosted installation paths. Both use Docker and keep the journal on your computer.

| Path | What you need | How to start |
| --- | --- | --- |
| Docker Compose from source | Git, Python 3 and a running Docker engine with Compose v2 | Run the commands below. |
| macOS desktop launcher | An Apple silicon Mac and Docker Desktop. Building the disk image also requires Go 1.26 or later and Xcode Command Line Tools. | Build the image locally, drag healthctl to Applications and choose **Open journal or try the demo**. [Desktop installation](docs/desktop.md) |

### Docker Compose

The first build needs internet access to fetch dependencies, container images and the English speech model used for offline dictation.

```sh
git clone https://github.com/ctlplne/healthctl.git
cd healthctl
python3 scripts/init.py
docker compose up -d --build --wait
```

Open [healthctl on your computer](http://127.0.0.1:8437) and create the local owner account. To use a different port, set `CLINICAL_PORT` in `.env` before starting Compose. The setup script generates a private `.env` containing the database password and encryption key. Keep a secure backup. [Backup and recovery](docs/operations.md)

To stop this installation, run `docker compose stop`. To resume it with your saved encounters, run `docker compose up -d`.

## Try the demo

After setup on either installation, the built-in demo runs locally with a fictional patient. It requires no model account, API key or separate inference server.

1. In **Model settings**, select **Synthetic demo** and **Save provider**.
2. Choose **New encounter** and use the prefilled runny-nose example. Answer the intake questions for the fictional adult, then choose **Yesterday** and **After being outdoors** when asked. For follow-ups, you can use your own words, say you’re unsure, or skip.
3. Review the history alongside the original answers. Choose **Confirm my history**, then **Generate provisional estimate**. Open **Report** to print a visit summary, save it as a PDF, or download the encounter as JSON.

## Keep track between visits

Choose **It happened again** to record an episode while you remember it. You can type or dictate, review what was heard, and save the entry locally. A return visit lets you choose which diary entries, reviewed report passages, and earlier findings to bring forward. Mark findings as ongoing, changed, resolved, or uncertain. The shorter interview asks up to two follow-up questions and carries the remaining gaps into the handoff.

In **Visit preparation**, you can compare records with what actually happened, record how you take a medicine alongside a photograph of its label, and invite a witness to contribute their own account. The clinician brief carries your main question, changes, warnings, and outstanding questions. [Voice input and transcription choices](docs/model-providers.md#dictation-and-account-billing)

## Bring your records

Choose **Start with a report** to add a PDF, Word document (.docx), image (JPEG or PNG), or plain text. healthctl extracts text locally, including English OCR for scans. Check dates, values, and units against the original, then choose which entries the model can use. For example, when a urine report includes a collection date, the interview can ask whether symptoms began before or after the sample.

You can revisit encounters on a timeline, record what happened after a visit, and export a clinician brief or a FHIR R4 document. A configured SMART on FHIR portal can also import lab results you select. [Visit briefs and exports](docs/handoff.md) · [Patient portal setup](docs/ehr.md)

## Models and privacy

Your installation stores the encounter and original uploads. You choose where inference runs.

| Model connection | What you need |
| --- | --- |
| Local or remote OpenAI-compatible server | A model ID and an endpoint you explicitly allow in `.env` |
| OpenAI or Anthropic API | A server-side API key and a supported model ID |
| ChatGPT plan | An eligible ChatGPT account linked from **Model settings** on a personal loopback installation |

Use the best model you have access to and select its highest available reasoning effort. A cloud connection sends relevant history and approved report extracts directly from your installation to the selected provider. Moving an existing encounter to another provider or endpoint requires your confirmation. [Model setup](docs/model-providers.md) · [Architecture and data boundaries](docs/architecture.md)

## Verification and releases

After running the setup script, run `python3 scripts/verify.py` from the checkout to exercise the Go and PostgreSQL tests, desktop and mobile browser journeys, and PDF exports. Run `python3 scripts/verify_restore.py` to check a backup against a fresh installation. GitHub Actions runs the software and synthetic case checks on every push to `main` and pull request targeting it. Weekly and tagged builds also exercise recovery, accessibility and both installer image architectures. Version tags prepare a draft release for maintainer review. [CI and release workflow](docs/ci.md) · [Verification results](docs/combined-verification.md) · [GitHub Actions](https://github.com/ctlplne/healthctl/actions)

## Project and license

healthctl is a [ctlplne](https://github.com/ctlplne) project maintained by [@imfeelingtheagi](https://github.com/imfeelingtheagi). For clinical scope and limitations, see the [intended use notes](docs/release.md). The [evaluation guide](evaluation/README.md) explains how the synthetic cases test history capture and safety warnings.

healthctl is source-available under the Business Source License 1.1. Each version converts to the Mozilla Public License 2.0 four years after its first public distribution. The licensor and any additional production-use grant are pending. [License](LICENSE) · [Release parameters](docs/release.md#source-available-license)
