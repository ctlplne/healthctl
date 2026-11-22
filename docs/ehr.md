# Connecting a health portal

healthctl is self-hosted. A user can use a compatible hospital portal to bring selected laboratory results into the same review and correction workflow as an uploaded report. The connector is off on a fresh installation. Opening an encounter or its Source reports tab only reads local settings. No healthctl service brokers the connection.

## Which integrations are worth pursuing

| Integration | Fit for a personal deployment | Qualification |
|---|---|---|
| Epic / MyChart | Epic describes users authorizing third-party apps to retrieve lab results. This is a good first US sandbox target for the laboratory workflow. | Register a user-facing application and its exact redirect URI. Test against the intended organization's R4 endpoints with synthetic records. [Epic user authorization](https://open.epic.com/Tutorial/PatientAuthentication) · [Epic on FHIR documentation](https://fhir.epic.com/Documentation?docId=epiconfhir) |
| Oracle Health Millennium | Its R4 Observation API represents laboratory measurements. DiagnosticReport covers other report material. A second sandbox qualification tests whether the same connector travels between vendors. | Register the SMART application with Oracle and check the tenant's supported user scopes and search capabilities. [Oracle registration](https://docs.oracle.com/en/industries/health/health-ai-application-suite/aidpu/index.html) · [Observation endpoints](https://docs.oracle.com/en/industries/health/millennium-platform-apis/mfrap/api-observation.html) |
| athenaOne | athenahealth documents user-facing, end-user-authorized access to its certified APIs and category-filtered laboratory scopes. | Verify the registered client type, granted scopes and whether the organization accepts the connector's Observation search request. [athenahealth authorization overview](https://docs.athenahealth.com/api/guides/authorization-overview) |
| Other SMART R4 portals | An operator can configure a compatible registered server directly. Labs make a useful first dataset because users can compare values, units and dates with the original result. | Match standalone public-client launch, S256, user context and read/search scopes. Each endpoint needs verification. [HL7 launch](https://hl7.org/fhir/smart-app-launch/app-launch.html) · [HL7 scopes](https://hl7.org/fhir/smart-app-launch/scopes-and-launch-context.html) |
| India and England | Use locally uploaded portal reports today. India's ABDM record exchange involves HIP/HIU services and a consent manager. NHS England restricts access to GP Connect services for people requesting their own records. | Pursue each country's onboarding with an eligible partner before building a national connector. [National Health Authority HIP/HIU guidance](https://abdm.gov.in/strapicms/uploads/HIP_HIU_Guidelines_f85df336ec.pdf) · [NHS user-facing APIs](https://digital.nhs.uk/services/gp-connect/implementation-and-integration/patient-facing-apis) |

After laboratory observations, prioritize full diagnostic reports for panels and written impressions, then user-authorized medication and allergy reconciliation when a partner can supply representative charts. Those records would extend both the existing report and returning-user workflows. Medication orders, user-reported use and dispensing events have different meanings. Chart write-back would need a receiving clinical workflow, a responsible reviewer and separate authorization. A clinician can already receive a user-shared brief, PDF or FHIR document. [Epic diagnostic reporting](https://fhir.epic.com/Specifications?api=10)

## Where requests go

| Action | Origin and destination | Data exchanged |
|---|---|---|
| Open healthctl, read a saved visit or upload a report | Browser → your installation | Local history, files, review and OCR. Bundled fonts and the starter evidence index are served locally. |
| Choose **Connect to health portal** | Installation backend → configured EHR discovery endpoint. Browser → configured hospital sign-in and consent page | The portal receives the registered application ID, callback, requested scopes, FHIR audience, random state and PKCE challenge. The user enters portal credentials at the hospital. |
| Approve access | Hospital redirects the browser → your installation. Installation backend → pinned token endpoint → configured FHIR server | The browser carries a one-use code back. The backend exchanges it and reads a bounded page of laboratory observations for the authorized user. The hospital sees the requests. No encounter narrative goes to the hospital. |
| Choose results to import | Browser → your installation | Unselected entries stay outside the saved encounter. The owner confirms identity. The app stores an encrypted text extract of the selected results and adds unreviewed rows with provenance. The temporary preview expires after ten minutes. |
| Use a remote model | Installation backend → selected model endpoint | Only separately authorized report extracts enter model input alongside the disclosed encounter context. An owner can use the synthetic demo or a local compatible model. Selecting a portal authorizes no model disclosure. |

If Docker runs on a laptop, the backend requests originate from that laptop's Docker network. If it runs on a home server, the requests originate from that server. The browser can be on a different device. The portal login and callback use the browser's network. A loopback callback on a phone points to the phone. For a separate browser device use a registered, operator-controlled HTTPS origin reachable from that device. Container builds and dependency installation may download public packages. Opening a publisher's source link navigates the browser to that publisher.

## Configure a registered portal

Obtain a public-client registration for standalone user access. Register the exact local callback with the portal. Use an accepted HTTPS callback or a loopback redirect explicitly supported by that registration. For a reverse proxy, configure the installation's existing `CLINICAL_PUBLIC_ORIGIN` and secure cookie settings. For a Compose installation, copy the following settings to its private `.env`. Replace the example hosts and client ID with your registration details and match the callback port to your installation.

```dotenv
CLINICAL_EHR_NAME=My hospital
CLINICAL_EHR_FHIR_BASE=https://records.example.org/fhir/r4
CLINICAL_EHR_AUTHORIZE_URL=https://login.example.org/oauth/authorize
CLINICAL_EHR_TOKEN_URL=https://login.example.org/oauth/token
CLINICAL_EHR_CLIENT_ID=registered-public-client-id
CLINICAL_EHR_REDIRECT_URI=http://127.0.0.1:8437/ehr/callback
CLINICAL_EHR_AUTH_ISSUER=
```

Pin the authorization server issuer in `CLINICAL_EHR_AUTH_ISSUER` when the portal includes the OAuth authorization-response `iss` parameter. The authorization server issuer can differ from the FHIR resource base. Discovery checks the pinned endpoints before sending the browser to the portal. A new registration or change of callback requires updating both sides.

```sh
docker compose up -d --build --wait
```

1. Open an encounter and choose **Source reports**. Inspect the hospital and both destinations, then choose **Connect to health portal**.
2. Sign in at the hospital and approve access. On return, choose individual test results and confirm that they belong to you. Check the count of omitted entries and any partial-results notice.
3. Import the selection. Compare and correct each row locally. Open the extract preview to choose what may be sent to the encounter's model. Imported effective dates describe the observation; specimen collection dates require their own source.

The connector requests `launch/patient patient/Observation.rs` and sends a single user-specific laboratory search. It retrieves at most 30 Observation resources from one page. A panel can contain multiple component values. Unsupported statuses, other users and unsupported representations are omitted with reasons. The preview shows when there may be more results at the portal. Sessions and previews expire, and restarting the app requires a fresh portal launch. There is no scheduled sync, offline access request, intermediary account, or write operation against a chart.

## Reproduce the local verification

```sh
python3 scripts/init.py
python3 scripts/verify.py backend
python3 scripts/verify.py browser
```

The browser suite runs a fictional hospital inside the application's disposable QA network namespace. Hospital consent is a separate browser origin and the local fixture validates the PKCE exchange. Keep personal portal registrations out of QA. A real vendor qualification still requires its registered client, accepted callback, synthetic sandbox user and compatible portal. Record vendor, tenant, scope, advertised FHIR version, authorization result, row matching, units, omissions and consent behavior before claiming compatibility.
