# Where your healthctl data goes

Your healthctl installation stores your journal. When you use AI, your installation sends relevant parts of your history directly to the model you choose. That model reads your answers, identifies findings for you to review, suggests follow-up questions and, if you ask, generates possible explanations. The results come back to your installation. ctlplne studio does not receive your encounter or relay these requests. A default installation also sends no analytics or crash reports to the studio.

## Who can read the journal?

The installation owner can read the journal. Someone with administrator access to the machine, database and encryption key can also read it. When someone else runs the server for you, ask who administers it and how long they keep backups. Encryption at rest protects clinical records and original uploads. Some account and operational metadata, including timestamps, stays visible to the database operator.

## Trying the demo or connecting a model

The synthetic demo runs a fictional script on your installation and makes no model requests. If you run a model offline on your own computer, the AI processing can stay on that computer. If you connect a model running on another machine, that machine receives the request and its operator controls its logging.

With a remote API or a linked ChatGPT plan, the provider processes the request on its servers. It can receive your concern, answers, findings gathered so far and any report extracts you approved for that destination. A linked plan uses your selected OpenAI account and its applicable terms. The provider controls its logs, retention and account settings. Changing the provider or endpoint for an existing encounter requires confirmation. Disconnecting a provider cannot recall earlier requests.

## Uploading a report or reading a patient portal

Your installation stores the original uploaded report and extracts text locally. Check who the report belongs to. Inspect the extract preview for identifiers and leave out any entry you do not want the model to receive. The model request includes approved extracts alongside interview context. Anything you type directly into an interview can also reach that model.

If you connect a patient portal, you sign in at the hospital. The hospital sees the authorization and laboratory read requests. Your installation temporarily previews results and saves the ones you select. Importing results and allowing a model to see selected extracts are separate choices. The connector does not send your interview back to the hospital.

## Voice, witnesses and shared reports

Ask permission before recording another person. Local transcription sends the clip from your browser to your installation, where it is processed temporarily. Optional cloud transcription sends the recording to the OpenAI platform API only after you select and confirm that destination. A linked ChatGPT plan does not provide audio transcription here. You can type instead. Review the draft before including it in the history.

A witness or clinician can fill out a downloaded offline form and return it directly to you. You choose what to import. Anyone you give a PDF, JSON, FHIR document or brief-access token to may see sensitive details. Revoking access stops future authorized reads. Recipients retain their own copies.

## Removing information

Deleting an encounter removes its primary records from the installation. Check separately for journal or diary entries, downloaded reports, shared copies, backups, snapshots, database recovery history and provider-held records. Manage those copies with the person or service that holds them. Restoring an older backup can restore a deleted visit.

## Getting help

There is no automatic support upload. Only if you choose to report a problem do GitHub and the people handling your report receive the account information and text you submit to GitHub. Use a made-up example. Keep patient details, recordings, report files, real screenshots, passwords and access tokens out of every support channel, including private reports. Use a healthcare professional or local emergency service for concerns about your own health.

Downloading software or opening an external source or support link contacts that service and discloses ordinary connection information such as your IP address. If you run a modified installation, use browser extensions, or access it through a proxy, those systems can have their own data practices.
