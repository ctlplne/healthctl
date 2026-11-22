# healthctl

**a ctlplne studio product · Free public preview (beta)**

healthctl is a self-hosted tool for taking a medical history before a visit. Start with a symptom in your own words or bring a lab report. The interview asks one question at a time. You can see the words or report entries behind a finding, correct what was misunderstood, and take a concise history to your clinician.

Use healthctl to organize your own account, spot misunderstandings and prepare questions for a medical visit. The symptom interview is designed for English-speaking adults. If you request possible explanations, the model can suggest them before a clinician reviews your history. Suggestions and urgency warnings can be wrong or miss serious illness. Take the findings to a qualified healthcare professional for verification. Do not use the tool to diagnose yourself, change treatment or decide it is safe to wait. If you may be having a medical emergency, seek local emergency care immediately.

healthctl stores your journal on your installation. The model you choose reads relevant parts of your history, suggests follow-up questions and generates possible explanations if you ask. With a cloud model, your installation sends that information directly to the provider for processing. ctlplne studio does not receive it. Cloud model and transcription providers may charge separately. [Public preview and limitations](docs/use-notice.md) · [Where your data goes](PRIVACY.md) · [Report a software or safety problem using fictional details](SECURITY.md)

[![Watch a user describe a concern in their own words](docs/media/history-interview-cover.jpg?v=choir)](https://healthctl.com/#walkthrough)

**[Watch the recorded interview (1 min 23 sec)](https://healthctl.com/#walkthrough)** · [Download the MP4](docs/media/history-interview.mp4?raw=true&v=choir) · [English captions](docs/media/history-interview.en.vtt)

A fictional user describes an echo in their left ear at choir practice. The user types what brings it on, and the connected ChatGPT model asks whether everyday hearing has changed. Watch the user correct the timing, confirm the history, and export the record. The questions came from the running app. Typing and model waits are shortened.

**[Watch the first-run walkthrough (1 min 55 sec, narrated MP4 with captions)](docs/media/healthctl-walkthrough.mp4?raw=true)** · [English captions (WebVTT)](docs/media/healthctl-walkthrough.en.vtt) · [Try the demo](#try-the-demo)

Follow a fictional user from creating the local owner account to reopening a saved visit. See how one approved lab result shapes the interview, what happens to an estimate when the user corrects their history, and what goes into the exported brief.

**[Watch how to connect your ChatGPT plan (1 min 6 sec)](https://healthctl.com/#chatgpt-setup)** · [MP4 in this repository](docs/media/chatgpt-setup.mp4?raw=true) · [English captions](docs/media/chatgpt-setup.en.vtt)

The setup film starts in the local journal and shows the plan permission, model selection, connection test, and destination for a new encounter. It uses an example account and illustrates the private sign-in step.

## Why we built healthctl

A user might start with “I get tired on the stairs.” A useful next question is “What could you do before this started?” The difference between always pausing and needing to stop for the first time last week is worth bringing to a visit. Users should be able to tell that story without having to know in advance which details matter.

In a randomized study of 50 physicians working from six written cases, the median diagnostic-reasoning score was 76% for physicians with access to GPT-4 and 74% with conventional resources. That difference was statistically inconclusive. GPT-4 alone scored 92% in a separate exploratory analysis. The cases already supplied a history, examination findings, and test results. The researchers did not compare complete and incomplete histories or evaluate live interviews. [Goh et al., JAMA Network Open](https://jamanetwork.com/journals/jamanetworkopen/fullarticle/2825395)

A second study examined what happens when people describe a medical scenario to a chatbot. In some conversations, important details never reached the model. That’s the problem healthctl is designed to address. It asks follow-ups, shows where a finding came from, and lets users correct the account before sharing it. Whether healthctl improves diagnostic accuracy or user outcomes still requires clinical evaluation. [Reliability of LLMs as medical assistants, Nature Medicine](https://www.nature.com/articles/s41591-025-04074-y)

## Install healthctl

Choose the Windows launcher, the Apple silicon launcher, or Docker Compose. Each runs the journal on your computer using Docker.

| Path | What you need | How to start |
| --- | --- | --- |
| Docker Compose from source | Git, Python 3 and a running Docker engine with Compose v2 | Run the commands below. |
| Windows desktop launcher | An Intel or AMD x64 Windows PC with Docker Desktop using WSL 2 and Linux containers. SmartScreen may block this unsigned beta. | [Download the Windows EXE](https://github.com/ctlplne/healthctl/releases/download/windows-0.2.0-beta.4/healthctl-windows-x64.exe), double-click it, and select **Open journal or try the demo**. [Windows installation](docs/windows.md) |
| macOS desktop launcher | An Apple silicon Mac and Docker Desktop. The beta download is unnotarized and macOS may block it. | [Download the beta DMG](https://github.com/ctlplne/healthctl/releases/download/macos-0.2.0-beta.4/healthctl-0.2.0-beta.4-macos-arm64.dmg), drag healthctl to Applications, then choose **Open journal or try the demo**. The first launch uses the internet to build the services. [Desktop installation](docs/desktop.md) |

### Docker Compose

The first build needs internet access to fetch dependencies, container images and the English speech model used for offline dictation.

```sh
git clone https://github.com/ctlplne/healthctl.git
cd healthctl
python3 scripts/init.py
docker compose up -d --build --wait
```

For an exact match to the beta 4 desktop downloads, check out `macos-0.2.0-beta.4` or `windows-0.2.0-beta.4` before building. Both tags identify the same source revision.

Open [healthctl on your computer](http://127.0.0.1:8437) and create the local owner account. To use a different port, set `CLINICAL_PORT` in `.env` before starting Compose. The setup script generates a private `.env` containing the database password and encryption key. Keep a secure backup. [Backup and recovery](docs/operations.md)

To stop this installation, run `docker compose stop`. To resume it with your saved encounters, run `docker compose up -d`.

## Try the demo

To try the interview shown in the featured video, connect your model, open **New encounter**, and leave **Answer style** set to **My own words first**. Type a fictional concern and answer in your own words. The follow-ups depend on the history you give.

On a fresh installation, choose **Try the offline demo**. If you already have a model configured, select **Synthetic demo** in Model settings and open **New encounter**. Choose **Practice with your own fictional concern**. The named demo cases use scripted questions and answer choices. You can review, correct, and export a history in either mode.

After setup on any of the three installation paths, the built-in demo runs locally with a fictional user. It requires no model account, API key or separate inference server.

<details>
<summary>Follow the report-first case from the video</summary>

1. **Set up the journal (0:00).** Create the owner account. Choose **Configure a provider**, select **Synthetic demo**, then test and save the connection.
2. **Start the encounter (0:19).** Choose **Start with a report**. For this scripted example, enter “It burns when I urinate.” Set the age to 34, answer **No** to both screening questions, and acknowledge the notice.
3. **Review the lab result (0:35).** Upload the [fictional lab PDF used in the video](docs/media/fictional-urinalysis.pdf). Select **Mine** and **Read report locally**. Review the leukocyte esterase entry, confirm the printed value **Trace** and collection date **2026-11-18**, then preview and approve the selected extract.
4. **Answer the interview (0:52).** Choose **Before the sample**. The video also shows how to record no medicines, uncertainty about allergies, a condition question that doesn’t apply, and a skipped surgery question. [See the report-linked question](docs/media/interview-question.png)
5. **Review and correct the history (1:06).** Confirm the history and request the optional provisional estimate. In **History review**, edit the onset finding to “The burning started the evening before I gave the sample.” Save, confirm again, and request a fresh estimate. In this scripted case, the revised assessment asks for more information. [See the corrected timing](docs/media/correct-a-finding.png)
6. **Export and reopen (1:32).** Under **Report**, select **Clinician brief** to print or save a PDF. **Download JSON** saves the encounter data. Return to the journal and reopen the visit to find the correction again. [See the fictional clinician brief](docs/media/clinician-brief.png)

</details>

For a shorter symptom-only example, try the prefilled runny-nose case.

1. In **Model settings**, select **Synthetic demo** and **Save provider**.
2. Choose **New encounter** and use the prefilled runny-nose example. Answer the intake questions for the fictional adult, then choose **Yesterday** and **After being outdoors** when asked. For follow-ups, you can use your own words, say you’re unsure, or skip.
3. In **History review**, choose the pencil beside **Runny nose**. Add when it stopped in **Corrected wording**, save the correction, then choose **Confirm my history**.
4. Open **Report** and choose **Clinician brief**. Look under **Reported positives** for the wording you just saved. You can print the brief or save it as a PDF. The report also offers a JSON download. For an optional experimental estimate, choose **Generate provisional estimate** after confirming the history.

When a connected model takes time to respond, the journal shows the task, model and elapsed wait. You can return to the journal and reopen the same encounter while it runs. If the connection drops, healthctl checks again automatically.

## Keep track between visits

Choose **It happened again** to record an episode while you remember it. You can type or dictate, review what was heard, and save the entry locally. A return visit lets you choose which diary entries, reviewed report passages, and earlier findings to bring forward. Mark findings as ongoing, changed, resolved, or uncertain. The shorter interview asks up to two follow-up questions and carries the remaining gaps into the handoff.

In **Visit preparation**, you can compare records with what actually happened, record how you take a medicine alongside a photograph of its label, and invite a witness to contribute their own account. The clinician brief carries your main question, changes, warnings, and outstanding questions. [Voice input and transcription choices](docs/model-providers.md#dictation-and-account-billing)

## Document a visible change

In **Source reports**, or alongside a relevant interview question, open **Optional photo of a visible change**. Use your camera or select a JPEG or PNG of your own eye, hand, foot or another area. A resized copy stays on your installation for you to show a clinician. With a connected model, you can add your own description to the interview with or without a photo. The model reads your words. A photo cannot establish jaundice, a nail disorder or its cause. If the whites of your eyes or your skin look newly yellow, seek urgent medical advice today.

## Bring your records

Choose **Start with a report** to add a PDF, Word document (.docx), image (JPEG or PNG), or plain text. healthctl extracts text locally, including English OCR for scans. Check dates, values, and units against the original, then choose which entries the model can use. For example, when a urine report includes a collection date, the interview can ask whether symptoms began before or after the sample.

You can revisit encounters on a timeline, record what happened after a visit, and export a clinician brief or a FHIR R4 document. A configured SMART on FHIR portal can also import lab results you select. [Visit briefs and exports](docs/handoff.md) · [Health portal setup](docs/ehr.md)

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

healthctl is source-available under the Business Source License 1.1. Each version converts to the Mozilla Public License 2.0 on the earlier of its specified Change Date and the fourth anniversary of its first public distribution under BUSL. The free grant covers personal use and internal use, including by clinics. Third-party hosting or embedding in a commercial service requires separate permission during the BUSL period. [License](LICENSE) · [Release parameters](docs/release.md#source-available-license)
