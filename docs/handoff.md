# Visit briefs and local integrations

Open **Report** to preview the clinician brief, full appendix, or a structured export. The authenticated owner can preview incomplete histories, including active urgent warnings. Confirmed by the user, document findings, AI hypotheses and visit outcomes have separate fields. The clinician brief also carries the user’s main question and reviewed visit-preparation details. Witness observations keep their attribution. The full appendix contains more personal information, including draft and older source material. Review the chosen export before manually sharing a downloaded copy.

## Local read-only API

For inbound laboratory results, the owner can authorize an optional health portal under Source reports. That permission is separate from the short-lived local grant below. [Health portal configuration and scope](ehr.md)

The full JSON export and scoped brief both expose the current recommendation in `modelGuidance` and the current beta and product notice in `previewNotice`. The FHIR Composition carries it in the human-readable narrative. The encounter’s original medical notice, consent version and recorded review status retain their historical values.

Use the report's access panel to grant access to the current confirmed encounter. The application shows a randomly generated token once. Store it only in the local consumer that needs the brief. A new grant replaces the prior token. The grant expires in 24 hours, on any subsequent encounter revision, on explicit revocation, or when the owner recovers their password or deletes the record. A downloaded copy stays with its recipient.

```sh
python3 scripts/read_brief.py --base http://127.0.0.1:8437
```

Use the address shown by your launcher or your configured Compose port for `--base`. The consumer privately prompts for its token and writes the authorized JSON to standard output. Its single network operation is `GET /api/integration/brief` with `Authorization: Bearer <token>`. Tokens in URLs are unsupported. The consumer accepts loopback origins and disables redirects and proxies. The token grants no access to uploads, timelines, other encounters, writing, or user sign-in. Owner routes use an authenticated session and CSRF protection for mutations.

| Owner endpoint | Operation |
|---|---|
| GET /api/encounters/{id}/brief | Current handoff in clinical-history/brief-1 format |
| GET /api/encounters/{id}/export | Complete versioned encounter JSON |
| GET /api/encounters/{id}/fhir | FHIR R4 document |
| GET, POST, DELETE /api/encounters/{id}/access | Inspect, issue or revoke the encounter grant |
| POST /api/encounters/{id}/documents | Local multipart upload with revision and requestId |
| GET /api/encounters/{id}/documents/{doc}/page/{number} | Local PNG rendering of an original PDF or photograph page |
| GET /api/encounters/{id}/documents/{doc}/original | Authenticated download of an original file |
| POST /api/encounters/{id}/documents/{doc}/preview | View the selected reviewed extracts for the pinned destination |

## FHIR R4 mapping

The export targets base profiles under `http://hl7.org/fhir/StructureDefinition/` at version 4.0.1. It is an exchange document for the described subset.

| Resource | Source and meaning |
|---|---|
| Bundle | Document identifier, timestamp and entries, with the Composition first |
| Composition | Preliminary user-prepared brief, urgency notices, scope, uncertainty, user-reported facts, document context, questions, provisional possibilities and follow-up |
| User | Installation-scoped anonymous subject for this encounter |
| Device | Software author of the assembled document |
| Observation | Reviewed user-specific report entries, printed values, qualifiers, units and reference ranges as text. Unknown clinical finality remains explicit |
| OperationOutcome | Human-readable mapping and omission details in each issue |

An AI hypothesis is represented within the preliminary narrative. The mapper assigns no confirmed condition, lab terminology code or normalized measurement that the source did not establish. Earlier encounters, full original documents, corrections and detailed provenance remain in the full local workspace. Relative or third-party findings belong in labeled narrative, separate from observations about the user. The local checker uses the unmodified official R4 JSON Schema, document reference and composition rules, and narrative XML checks. A separate network-isolated profile test runs the HL7 FHIR Validator against R4 4.0.1. Explicit data-absent reasons capture an unmapped performer or a collection date that only exists as free text. [HL7 R4 document specification](https://hl7.org/fhir/R4/documents.html) · [HL7 R4 validation](https://hl7.org/fhir/R4/validation.html)
