# Architecture and data boundaries

The Go service serves a bundled React/TypeScript application, a same-origin JSON API and browser-printable reports. PostgreSQL is the only persistent runtime dependency. Docker exposes the app on loopback and isolates PostgreSQL on an internal network. The browser loads assets and fonts from the installation. Optional model requests go from the backend to the chosen model service. An explicitly started patient-portal import sends the browser to the hospital for sign-in and the backend to the registered SMART and FHIR endpoints.

| Component | Responsibility |
|---|---|
| `internal/server` | Owner registration and sign-in, sessions, request limits, same-origin access, destination disclosure, model orchestration, authenticated reports and deletion |
| `internal/store` | Versioned, transactional PostgreSQL migrations, owner-scoped queries, encrypted encounter documents, submission receipts and compare-and-swap writes |
| `internal/clinical` | Allowed transitions, evidence spans, active and superseded facts, contradictions, summary confirmations, longitudinal warnings and assessment versions |
| `internal/provider` | Explicit destinations, provider wire formats, fixed prompts, bounded attempts, response schema checks and provenance |
| `internal/ehr` | Standalone SMART public-client launch, direct lab observation search, patient-scoped parsing and selected-only transcription |
| `web/src` | Setup, encounter list, interview, correction, review, estimate, history and print views |
| `evaluation` | Authored synthetic cases, deterministic checks and separate optional live model evaluation |

## One submission

An intake includes the displayed provider configuration and acknowledged notice version. Creation locks the settings row and compares the disclosed destination before any inference. Changes to the installation default apply to future encounters. Moving an existing encounter requires explicit confirmation, records the old and new configuration, then waits for a patient action before transmitting new content. Demo and real encounters cannot be converted into each other.

Every accepted patient submission commits its revision, encrypted content and keyed request receipt in a transaction before calling the model. Duplicate identifiers with identical contents return the saved encounter and avoid another call. Reusing an identifier for different contents fails. A model result can update only its exact source revision. Concurrent corrections, deletion or another completed operation take precedence. Network calls run outside database locks. An interrupted or malformed response leaves the patient's input available for retry. On startup the single application instance marks interrupted, pending model calls as recoverable failures.

The browser requests asynchronous completion with `Prefer: respond-async`. When a remote call is needed, the server returns the saved encounter and its status URL with HTTP 202. Only the original submission starts inference. Opening the encounter polls its authenticated status endpoint, so leaving the page or reconnecting can resume the same record. API clients that omit the preference receive the result in the original response.

The interview model receives structured active facts and original patient evidence within explicit input limits. It proposes only typed operations. The app validates supporting spans and references; a model can still misinterpret genuine text. The model has no application tools. Model output cannot set the destination, application notice, evidence confirmation, warning status, owner identity, or review status.

Age and pregnancy-specific scope are checked at intake. Blocked or insufficient encounters can still produce a history report. Urgency is tracked independently of the diagnosis ranking. The patient can interrupt questioning, correct the history, see a sustained warning, review what they reported, and elect to request an estimate when evidence supports it. A negative screen is never a safety clearance.

## Report and retention

Every patient report carries the app-controlled notice, review status and warnings. The full encounter export includes confirmation and correction history, model metadata and assessment revisions. JSON format `clinical-history/1` is application-specific. The print report is rendered by the browser from authenticated encounter data. Save as PDF and downloaded JSON create sensitive copies controlled by the recipient.

Encrypted encounter documents contain the patient history, accepted model turns, assessment versions and provider changes. Per-encounter submission receipts cascade on deletion. The minimal deletion audit event records the owner, time and action. Operational application logs record fixed event names, event IDs, and startup status. Rejected raw provider responses and hidden reasoning are discarded.

## Combined release data flow

The optional portal connector is disabled without operator configuration. State and PKCE verifier are tied to the authenticated local session and encounter revision. After portal authorization, a single bounded laboratory search yields a temporary local preview. Saving commits only the chosen transcriptions, encrypted original text and a retry receipt. The connector discards the preview on a successful save or cancellation. The local model-disclosure workflow still applies. [Portal data flow and trust boundaries](ehr.md)

Uploaded bytes go through an authenticated, CSRF-protected multipart endpoint. Bounded native readers operate locally. A single transaction persists the separately encrypted original, candidate rows, an encounter revision and a retry receipt. Uploading, transcribing, consenting and sending are distinct transitions. The model input builder selects approved row revisions at the pinned destination and filters dependent prior questions, answers, findings and warnings. Full original files are available only through the encounter owner's attachment endpoint. Raw titles, text and unselected rows are excluded from model requests.

Returning-patient history is explicit. The timeline returns dated record metadata and active facts. Reconciliation records the source encounter, fact, reviewer date, applicability status and wording. Linking an episode preserves prior assessments. A patient-reported or documented outcome remains distinct from the contemporaneous AI estimate. Report corrections invalidate current estimates and their older evidence snapshots remain inspectable.

Evidence packs are versioned clinical literature. Their content and source rights are separate from patient statements. Source status can change after a report was produced. The stored answer and snapshot remain intact. A brief is projected from the current encounter, and an external read-only token is scoped to one confirmed revision with a 24-hour expiry. Any subsequent encounter mutation expires that grant. The FHIR document uses a constrained R4 mapping and carries a machine-readable mapping-loss report. [Handoff and local integration](handoff.md) · [Evidence collections](evidence.md)

Voice capture uses a same-origin browser AudioWorklet on a secure context (localhost or HTTPS). It produces mono 16 kHz PCM. The authenticated speech route accepts at most 90 seconds, processes one clip at a time, and times out after two minutes. The offline backend uses whisper.cpp and the bundled quantized English tiny model in a temporary directory. The API backend requires the platform credential and a separate consent header, sends a multipart clip to the fixed OpenAI transcription endpoint, and returns a draft. A transcript enters the journal only after patient review and submission. Local playback of selected phrases uses an English voice advertised by the browser as local when available.

Diary payloads are encrypted with owner- and entry-specific associated data. The server records the entry timestamp. The patient supplies occurrence times and uncertainty. Creating a return visit locks the selected prior revision, validates the selected passages and destination consent, and commits source snapshots with the new encounter in one transaction. Its local originals are encrypted for the new encounter. Snapshot attribution identifies the prior encounter and revision. Subsequent corrections belong to the visit where they were made. Later review of an amended original requires deliberately selecting the newer version. Witness and feedback forms run offline and contain an opaque visit identifier and revision. The contributor returns the response to the patient, who reviews it before inclusion. Contributors' identities are self-reported.
