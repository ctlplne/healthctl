# Model providers

Each encounter pins its provider configuration. The application controls the interview state, emergency messaging, review status, permitted actions, and exports. Model results are proposals. The backend accepts a proposal only after JSON structure and clinical evidence validation.

## Select an endpoint

In the authenticated provider settings, select a kind and enter a model identifier from your deployment or provider account. The application has no default live model. Test the connection before beginning an encounter. The test sends a fixed, nonclinical request for a small JSON response.

Use the best model you have access to and select its highest available reasoning effort. Choose the model in provider settings. Where the endpoint supports a reasoning-effort setting, configure it through the serving deployment. The app displays this recommendation throughout the workspace and in exported reports. For ChatGPT plan requests, healthctl requests the highest recognized effort the selected account model advertises and records the requested value with accepted results. Other adapters use the serving deployment’s configuration.

| Kind | Destination and protocol | Server credential |
|---|---|---|
| Compatible | Operator-selected base URL plus `/chat/completions` | Optional `BYOM_API_KEY` bound to `BYOM_KEY_ENDPOINT` |
| OpenAI | `https://api.openai.com/v1/responses` | `OPENAI_API_KEY` |
| ChatGPT plan | Account-scoped catalog and streaming Responses at `https://api.openai.com/v1` | Owner-authorized OAuth grant encrypted on this host |
| Anthropic | `https://api.anthropic.com/v1/messages` | `ANTHROPIC_API_KEY`, and `ANTHROPIC_WORKSPACE_ID` for a key spanning multiple workspaces |
| Synthetic demo | In-process deterministic fixtures | None |

Set server environment variables in the deployment and recreate the app container. An operator running a compatible endpoint on their Docker Desktop host might set the following in the local installation's `.env` file.

```dotenv
CLINICAL_ALLOWED_ENDPOINTS=http://host.docker.internal:11434/v1
```

Then select **Compatible** in settings and enter that exact base URL and a model ID already installed on that server. A container's loopback points back to the container. On Linux, an explicitly configured Docker host gateway or a dedicated inference service on the app's network can supply the reachable hostname. Bundle your local model and install its dependencies before disconnecting from the internet.

For a protected compatible endpoint, set `BYOM_KEY_ENDPOINT` to the same base URL, then set `BYOM_API_KEY` server-side. The key is attached only to that destination. Separate multiple allowed base URLs with commas. A vendor-specific key is usable only with the corresponding fixed vendor origin. HTTP is limited to intentionally allowed private/loopback destinations. Use HTTPS to protect traffic crossing a network you do not control. An allowlisted relay can forward content according to its own configuration.

The backend also supports `OPENAI_API_KEY_FILE`, `ANTHROPIC_API_KEY_FILE`, and `BYOM_API_KEY_FILE`. A Compose override can mount a secret read-only and pass its mounted path as the corresponding environment variable. Set a single credential source per key. Limit read access to the installation's secret files and restart the backend after rotation.

## Use a ChatGPT subscription

1. Open healthctl at `http://127.0.0.1:8437` and sign in as the installation owner. In **Model settings**, select **ChatGPT plan** and **New ChatGPT account**.
2. Check **Allow eligible model requests from this installation to use my ChatGPT plan or credits** and select **Continue with ChatGPT**. OpenAI handles sign-in, workspace selection and provider consent. Leaving the checkbox clear links the identity only. OpenAI can also return an identity without granting plan use.
3. Back in Model settings, choose the account and one of its currently available models. **Test connection** sends a fixed nonclinical probe through the plan. **Save provider** makes that choice the default for new encounters. Existing encounters retain their account and model until the patient confirms a change and reviews any affected report disclosures.
4. Manage limits and credit permissions in ChatGPT. Model selection and plan availability are account- and workspace-specific. A session interrupted by a usage limit keeps its history, urgent warnings and export. **Disconnect** clears local credentials and attempts to revoke the renewable grant at OpenAI. If remote revocation cannot be confirmed, the UI links to ChatGPT settings. [OpenAI plan usage and credit controls](https://help.openai.com/en/articles/20001542-using-your-chatgpt-plan-in-other-apps-and-sites) · [Manage usage](https://chatgpt.com/settings/usage)

**Data flow.** The browser visits OpenAI for consent and returns to the same loopback port at `/auth/callback`. A successful account connection alone sends no clinical history. healthctl’s backend obtains the selected account’s model list and sends approved interview or assessment inputs directly to OpenAI. The selected account’s registration remains part of the encounter destination. Two workspaces using the same email get distinct local registrations. Only the opaque registration identifier is used in exported provider metadata. [Registration and sign-in](https://developers.openai.com/siwc/token-sharing-open-source/sign-in) · [Account management](https://developers.openai.com/siwc/token-sharing-open-source/profiles-and-sessions)

**Installation and release scope.** This adapter is enabled on a personal HTTP loopback installation using an explicit `127.0.0.1` port. A user-managed VM can use SSH port forwarding to the user’s own loopback address. Public-origin mode disables the adapter. OpenAI directs paid or remotely hosted apps to its integration program. Confirm eligibility before distributing healthctl as a paid BUSL product or offering a managed deployment. Anthropic reserves personal Claude plan credentials for its own supported products. Its Claude Platform OAuth program bills usage to an authorized API organization. healthctl’s Claude option uses its existing API integration. [OpenAI local integration](https://developers.openai.com/siwc/token-sharing-open-source) · [Self-hosted VMs](https://developers.openai.com/siwc/token-sharing-open-source/self-hosted-vms) · [Anthropic subscription policy](https://code.claude.com/docs/en/legal-and-compliance) · [Claude Platform OAuth](https://platform.claude.com/docs/en/manage-claude/oauth-apps/overview)

**Credential lifecycle.** An install-specific host ID survives restarts. Each browser attempt uses a fresh state, nonce and PKCE verifier. The backend validates the signed identity and registration and checks the granted plan scope before model use. Rotating refresh credentials are serialized under a database row lock and encrypted with the installation key. The manual and scheduled backup procedures exclude the host ID and subscription vault. After restore or owner recovery, connect the account again and confirm any change to an existing encounter. [Operations](operations.md)

## Capability choices and privacy

**Auto** requests native JSON Schema from OpenAI and Anthropic. For ChatGPT plan and compatible servers it uses schema-constrained instructions. Compatible servers receive Chat Completions requests. Choose **Native** for a compatible server with documented `response_format.json_schema` support. Choose **Prompt** explicitly when the selected official model lacks native structured output. A capability error is reported to the owner. The same server-side schema and domain validation run in all formats.

The payload includes the selected task, age, chief concern, relevant background, current evidence and its patient wording, current warnings, contradictions, and outstanding questions. A remote provider receives this content under its own processing and retention terms. Both OpenAI billing paths set `store: false`. On the ChatGPT plan route, OpenAI requires HTTP streaming. healthctl buffers and validates the completed reply before presenting a result. All providers receive text-only requests without tools or browsing. The backend ignores private reasoning blocks, drops the response envelope after parsing and accepts only a complete validated proposal. The owner can export a saved history after a provider failure.

The backend checks an exact canonical compatible base URL, disables redirects and HTTP proxies, and resolves and verifies addresses at connection time. Loopback and private addresses are supported for deliberate local configuration. Link-local, known metadata-service and unsuitable network addresses are blocked. Provider, model and destination appear in the encounter history. Each accepted model result records the requested and returned model identifiers, API/output format settings, prompt and schema versions, application version and request count. A blank returned identifier means the endpoint didn't provide one.

## Wire contracts and limits

The bundled `internal/provider/contracts/proposal-v1.schema.json` is the wire schema. The versioned clinical and repair instructions are beside it. A single application proposal shape covers the interview and assessment tasks. The task-specific validator checks allowed actions, active sources, exact quotes, supporting and conflicting facts, content lengths and other clinical invariants. A schema-valid response can still be medically wrong.

API-key operations and connection tests have a 75-second deadline. A ChatGPT plan interview or assessment can take up to four minutes, including any repair attempt. A guided ChatGPT plan interview starts with a locally chosen question about onset, change over time, impact or the patient’s main concern. The first answer and the chief concern reach the model together. Choosing review immediately asks the model to extract the history before confirmation. Scope restrictions, explicit emergency answers, recognized urgent wording and report-first intake take precedence. In the browser, starting an interview opens the saved encounter immediately. You can return to the journal and reopen the encounter while the same request runs. If the request fails, **Retry provider request** uses the saved history and its selected destination.

Each model input is limited to 48 KiB. API adapters request up to 4,096 output tokens. The ChatGPT plan route accepts a stream of up to 2 MiB, with bounded events and a 32 KiB final proposal. On completion, healthctl checks up to 4,096 answer tokens for a clinical proposal. It counts reported reasoning separately for that check and records the full reported usage. With no reasoning breakdown, the check uses the total output count. OpenAI determines actual plan usage, including replies that fail validation. [Subscription preview limits](https://developers.openai.com/siwc/token-sharing-open-source/preview-limitations)

API-key adapters allow one short retry for selected transient HTTP status codes. After malformed or semantically invalid output, the backend can request one fresh proposal using the same evidence and destination. Incomplete streams and refusals leave the encounter available for review, export or explicit retry.

## Reproduce adapter checks

Run `go test -race ./internal/provider` for deterministic transport and semantic-validation callback tests. These fixture tests require no provider, keys, or network. They exercise the API adapters and the ChatGPT subscription transport, strict parsing, retries, cancellation, per-endpoint credential scoping and refusal/error handling.

To run a separate live remote integration check, configure your own provider credentials in the process environment. Set `MODEL_LIVE_KIND` to `openai`, `anthropic`, or `compatible`, and `MODEL_LIVE_MODEL` to the exact model ID. For compatible servers also set `MODEL_LIVE_ENDPOINT` to a permitted public HTTPS base URL and include that URL in `CLINICAL_ALLOWED_ENDPOINTS`. Optional `MODEL_LIVE_STRUCTURED` selects the same output-format setting used by the app.

```sh
MODEL_LIVE=1 go test -count=1 -v ./internal/provider -run '^TestLiveRemoteSynthetic$'
```

For a real ChatGPT account, use **Test connection** after consent and record the selected model and result using fictional data. Keep tokens and authorization callback URLs private. The headless API-key check uses a separate billing path.

The live check sends a nonclinical probe followed by a synthetic adult interview and assessment, and validates the replies using the application's real clinical contract. Provider charges may apply. A passing run establishes that the selected endpoint served that case. Diagnostic accuracy requires separate evaluation. A normal test run skips this check.

## Provider references

The OpenAI adapter uses the Responses API with `text.format` for native structured output. The compatibility adapter targets the Chat Completions request and response envelope. [OpenAI Responses API](https://developers.openai.com/api/reference/resources/responses/methods/create) · [OpenAI structured outputs](https://developers.openai.com/api/docs/guides/structured-outputs) · [OpenAI Chat Completions](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create)

The Anthropic adapter pins API version `2026-10-21` for direct Messages requests, uses bearer authentication and sets `output_config.format` for native structured output. [Claude Messages API](https://platform.claude.com/docs/en/api/messages/create) · [Claude structured outputs](https://platform.claude.com/docs/en/build-with-claude/structured-outputs) · [Claude authentication](https://platform.claude.com/docs/en/manage-claude/authentication) · [Claude API versioning](https://platform.claude.com/docs/en/api/versioning)

## Combined release qualification and usage

The authenticated qualification panel distinguishes deterministic wire fixtures from observed operations at a configured installation. An accepted proposal means the endpoint responded using a permitted format and evidence references. It does not identify whether the endpoint's underlying implementation is a model, qualify its clinical performance, or certify a model family. The original release used `clinical-v1`. Version `clinical-v2` introduced disclosure, source attribution and report-guided questioning. New calls use `clinical-v3`, which prioritizes focused follow-ups, earlier answers, explicit handling of skipped questions and locally guided health background. Existing results retain their own prompt versions.

The panel aggregates completed and failed model operations, actual HTTP requests, retries (including a repair), reported input/output tokens, and median and 95th-percentile latency. It displays the number of operations with incomplete token accounting. A timeout or rejection can be billable even if the provider returns no usage. Anthropic cache read/write input tokens are included when reported. These data remain local and exclude patient text, encounter IDs, provider response text and secret keys. Probe traffic and embedded demos are separate from clinical model operations.

Set optional server variable `CLINICAL_MODEL_PRICES` to a JSON array of operator-supplied rate objects. A rate requires `provider`, `model`, `inputUSDPerMillion`, and `outputUSDPerMillion`. For a compatible provider, `endpoint` must also match its canonical base URL. The backend never contacts a pricing service. A flat-rate estimate is produced only for operations with token counts for all attempts and matching prices. Unpriced operations and known priced subtotal are shown separately. Cached-token discounts, variable pricing, taxes, infrastructure and editorial work can change actual operating costs. The local scorecard describes which operating and clinical comparisons have evidence.
