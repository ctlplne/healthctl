# Intended use and release decisions

healthctl gathers a text-based symptom history from English-speaking adults and estimates possible explanations for the reporting patient to discuss with a qualified professional. A person may view the result before visiting a clinician. The application has no clinical signing or prescribing workflow. Source code, self-hosting, warnings and successful synthetic tests do not establish safety, clinical accuracy, regulatory status, or privacy compliance.

The initial emergency policy is development content. Have qualified clinicians review its handling of missed and excessive alerts, uncertainty, correction and scope, and evaluate the patient-facing presentation before real-world release. Local policies cover only a restricted set of phrases. An LLM can misread evidence, suggest unsupported explanations, or neglect important questions. The evidence validator cannot establish clinical truth. Qualify actual provider/model versions, including abstention, across realistic histories and accessibility needs. Clinician review of this software is distinct from a clinician reviewing any individual patient report.

Establish intended audience, distribution, intended-use labeling, and jurisdiction-specific review for the US and each intended market. The FDA's clinical decision support guidance explains the criteria for the US non-device CDS exclusion and addresses the applicability of other digital health policies to patient-facing functions. Patient-facing estimates and urgency messaging warrant specific qualified regulatory review. No classification, approval, exemption, clinical validation or HIPAA status is asserted here. [FDA clinical decision support guidance](https://www.fda.gov/regulatory-information/search-fda-guidance-documents/clinical-decision-support-software)

## Source-available license

healthctl uses `BUSL-1.1` with a four-year conversion to `MPL-2.0`. The conversion clock runs separately for each version. [Business Source License 1.1](https://mariadb.com/bsl11/) · [Mozilla Public License 2.0](https://www.mozilla.org/en-US/MPL/2.0/)

| Parameter | Decision |
|---|---|
| Licensor | Legal rights holder pending. No identity has been supplied. |
| Licensed Work | healthctl, with an explicit version or release identifier and corresponding copyright notice. |
| Additional Use Grant | Proposed policy would permit personal and internal self-hosted production use. Decide whether to reserve hosting for third parties for separate commercial terms. Final grant language requires the rights holder's approval. |
| Change Date | The fourth anniversary of the first public distribution of this version. Record both calendar dates in its release notice. |
| Change License | Mozilla Public License 2.0 (`MPL-2.0`), with the standard secondary-license compatibility provisions. |

Resolve release parameters, ownership, third-party dependency notices, and commercial terms before public distribution. Complete a model/provider retention and data-processing review against the operator's intended use and region. Medical notices are separate from the software license.


## ChatGPT subscription qualification

The direct integration is implemented for a personal loopback installation. Before a paid or managed release, establish the applicable integration terms with OpenAI for that distribution and complete an account-authorized synthetic probe. Record the model actually offered, the result of the probe and the applicable workspace credit controls. A successful probe qualifies the connection for that account at that time. Clinical review and deployment-specific handling of health information remain separate release decisions. [OpenAI integration scope](https://developers.openai.com/siwc/token-sharing-open-source) · [Provider qualification](model-providers.md#use-a-chatgpt-subscription)
