# Model providers

Each encounter pins its provider configuration. The application controls the interview state, emergency messaging, review status, permitted actions, and exports. Model results are proposals. The backend accepts a proposal only after JSON structure and clinical evidence validation.

## Select an endpoint

In the authenticated provider settings, select a kind and enter a model identifier from your deployment or provider account. The application has no default live model. Test the connection before beginning an encounter. The test sends a fixed, nonclinical request for a small JSON response.

Use the best model you have access to and select its highest available reasoning effort. Choose the model in provider settings. Where the endpoint supports a reasoning-effort setting, configure it through the serving deployment. The app displays this recommendation throughout the workspace and in exported reports. It does not set or verify an effort level on individual requests.

| Kind | Destination and protocol | Server credential |
|---|---|---|
| Compatible | Operator-selected base URL plus `/chat/completions` | Optional `BYOM_API_KEY` bound to `BYOM_KEY_ENDPOINT` |
| OpenAI | `https://api.openai.com/v1/responses` | `OPENAI_API_KEY` |
| Anthropic | `https://api.anthropic.com/v1/messages` | `ANTHROPIC_API_KEY`, and `ANTHROPIC_WORKSPACE_ID` for a key spanning multiple workspaces |
| Synthetic demo | In-process deterministic fixtures | None |

Set server environment variables in the deployment and recreate the app container. An operator running a compatible endpoint on their Docker Desktop host might set the following in the local installation's `.env` file.

```dotenv
CLINICAL_ALLOWED_ENDPOINTS=http://host.docker.internal:11434/v1
```

Then select **Compatible** in settings and enter that exact base URL and a model ID already installed on that server. A container's loopback points back to the container. On Linux, an explicitly configured Docker host gateway or a dedicated inference service on the app's network can supply the reachable hostname. Bundle your local model and install its dependencies before disconnecting from the internet.

For a protected compatible endpoint, set `BYOM_KEY_ENDPOINT` to the same base URL, then set `BYOM_API_KEY` server-side. The key is attached only to that destination. Separate multiple allowed base URLs with commas. A vendor-specific key is usable only with the corresponding fixed vendor origin. HTTP is limited to intentionally allowed private/loopback destinations. Use HTTPS to protect traffic crossing a network you do not control. An allowlisted relay can forward content according to its own configuration.

The backend also supports `OPENAI_API_KEY_FILE`, `ANTHROPIC_API_KEY_FILE`, and `BYOM_API_KEY_FILE`. A Compose override can mount a secret read-only and pass its mounted path as the corresponding environment variable. Set a single credential source per key. Limit read access to the installation's secret files and restart the backend after rotation.

## Capability choices and privacy

**Auto** requests native JSON Schema from OpenAI and Anthropic. For compatible servers it uses schema-constrained instructions with ordinary Chat Completions. Choose **Native** for a compatible server with documented `response_format.json_schema` support. Choose **Prompt** explicitly when the selected official model lacks native structured output. A capability error is reported to the owner. The same server-side schema and domain validation run in all formats.

The payload includes the selected task, age, chief concern, relevant background, current evidence and its patient wording, current warnings, contradictions, and outstanding questions. A remote provider receives this content under its own processing and retention terms. The OpenAI adapter sets `store: false`. No adapter requests external tools, web browsing or streaming. The backend ignores private reasoning blocks, drops the response envelope after parsing and accepts only a complete validated proposal. The owner can export a saved history after a provider failure.

The backend checks an exact canonical compatible base URL, disables redirects and HTTP proxies, and resolves and verifies addresses at connection time. Loopback and private addresses are supported for deliberate local configuration. Link-local, known metadata-service and unsuitable network addresses are blocked. Provider, model and destination appear in the encounter history. Each accepted model result records the requested and returned model identifiers, API/output format settings, prompt and schema versions, application version and request count. A blank returned identifier means the endpoint didn't provide one.

## Wire contracts and limits

The bundled `internal/provider/contracts/proposal-v1.schema.json` is the wire schema. The versioned clinical and repair instructions are beside it. A single application proposal shape covers the interview and assessment tasks. The task-specific validator checks allowed actions, active sources, exact quotes, supporting and conflicting facts, content lengths and other clinical invariants. A schema-valid response can still be medically wrong.

Each call has a 75-second overall deadline. The model context has a 48 KiB serialized clinical-input cap. Output is capped at 4,096 tokens, 256 KiB per response envelope, and 32 KiB per proposal. Exact input tokenization and the supported context window depend on the operator's model. The call returns a recoverable failure when provider limits are exceeded. A short retry is available once per operation for selected retryable provider status codes. The backend attempts one fresh proposal after malformed or semantically invalid output, using the same input and destination. It discards prior malformed content. Truncation, refusal and persistent failure return to the caller. Provider switching is explicit.

## Reproduce adapter checks

Run `go test -race ./internal/provider` for deterministic transport and semantic-validation callback tests. These fixture tests require no provider, keys, or network. They exercise all three vendor envelopes, strict parsing, retries, cancellation, per-endpoint credential scoping and refusal/error handling.

To run a separate live remote integration check, configure your own provider credentials in the process environment. Set `MODEL_LIVE_KIND` to `openai`, `anthropic`, or `compatible`, and `MODEL_LIVE_MODEL` to the exact model ID. For compatible servers also set `MODEL_LIVE_ENDPOINT` to a permitted public HTTPS base URL and include that URL in `CLINICAL_ALLOWED_ENDPOINTS`. Optional `MODEL_LIVE_STRUCTURED` selects the same output-format setting used by the app.

```sh
MODEL_LIVE=1 go test -count=1 -v ./internal/provider -run '^TestLiveRemoteSynthetic$'
```

The live check sends a nonclinical probe followed by a synthetic adult interview and assessment, and validates the replies using the application's real clinical contract. Provider charges may apply. A passing run establishes that the selected endpoint served that case. Diagnostic accuracy requires separate evaluation. A normal test run skips this check.

## Provider references

The OpenAI adapter uses the Responses API with `text.format` for native structured output. The compatibility adapter targets the Chat Completions request and response envelope. [OpenAI Responses API](https://developers.openai.com/api/reference/resources/responses/methods/create) · [OpenAI structured outputs](https://developers.openai.com/api/docs/guides/structured-outputs) · [OpenAI Chat Completions](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create)

The Anthropic adapter pins API version `2026-10-21` for direct Messages requests, uses bearer authentication and sets `output_config.format` for native structured output. [Claude Messages API](https://platform.claude.com/docs/en/api/messages/create) · [Claude structured outputs](https://platform.claude.com/docs/en/build-with-claude/structured-outputs) · [Claude authentication](https://platform.claude.com/docs/en/manage-claude/authentication) · [Claude API versioning](https://platform.claude.com/docs/en/api/versioning)

## Combined release qualification and usage

The authenticated qualification panel distinguishes deterministic wire fixtures from observed operations at a configured installation. An accepted proposal means the endpoint responded using a permitted format and evidence references. It does not identify whether the endpoint's underlying implementation is a model, qualify its clinical performance, or certify a model family. The original release used `clinical-v1`. New model calls use `clinical-v2`, which includes disclosure, source attribution and report-guided questioning. Existing results retain their own prompt versions.

The panel aggregates completed and failed model operations, actual HTTP requests, retries (including a repair), reported input/output tokens, and median and 95th-percentile latency. It displays the number of operations with incomplete token accounting. A timeout or rejection can be billable even if the provider returns no usage. Anthropic cache read/write input tokens are included when reported. These data remain local and exclude patient text, encounter IDs, provider response text and secret keys. Probe traffic and embedded demos are separate from clinical model operations.

Set optional server variable `CLINICAL_MODEL_PRICES` to a JSON array of operator-supplied rate objects. A rate requires `provider`, `model`, `inputUSDPerMillion`, and `outputUSDPerMillion`. For a compatible provider, `endpoint` must also match its canonical base URL. The backend never contacts a pricing service. A flat-rate estimate is produced only for operations with token counts for all attempts and matching prices. Unpriced operations and known priced subtotal are shown separately. Cached-token discounts, variable pricing, taxes, infrastructure and editorial work can change actual operating costs. The local scorecard describes which operating and clinical comparisons have evidence.
