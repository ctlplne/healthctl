# Intended use and release decisions

healthctl is a ctlplne studio product distributed as a free public preview (beta). It gathers a text-based symptom history from English-speaking adults and estimates possible explanations for the reporting patient to discuss with a qualified professional. A person may view the result before visiting a clinician. The application has no clinical signing or prescribing workflow. Source code, self-hosting, warnings and successful synthetic tests do not establish safety, clinical accuracy, regulatory status, or privacy compliance.

The initial emergency policy is development content. Have qualified clinicians review its handling of missed and excessive alerts, uncertainty, correction and scope, and evaluate the patient-facing presentation before real-world release. Local policies cover only a restricted set of phrases. An LLM can misread evidence, suggest unsupported explanations, or neglect important questions. The evidence validator cannot establish clinical truth. Qualify actual provider/model versions, including abstention, across realistic histories and accessibility needs. Clinician review of this software is distinct from a clinician reviewing any individual patient report.

Establish intended audience, distribution, intended-use labeling, and jurisdiction-specific review for the US and each intended market. The FDA's clinical decision support guidance explains the criteria for the US non-device CDS exclusion and addresses the applicability of other digital health policies to patient-facing functions. Patient-facing estimates and urgency messaging warrant specific qualified regulatory review. No classification, approval, exemption, clinical validation or HIPAA status is asserted here. [FDA clinical decision support guidance](https://www.fda.gov/regulatory-information/search-fda-guidance-documents/clinical-decision-support-software)

## Source-available license

healthctl uses `BUSL-1.1` with a four-year conversion to `MPL-2.0`. The conversion clock runs separately for each version. [Business Source License 1.1](https://mariadb.com/bsl11/) · [Mozilla Public License 2.0](https://www.mozilla.org/en-US/MPL/2.0/)

| Parameter | Decision |
|---|---|
| Licensor | Certctl LLC is the legal publisher and licensor. healthctl is a ctlplne studio product. |
| Licensed Work | The healthctl 0.2.0 public-preview source revision bearing the distributed notice. |
| Additional Use Grant | Free personal and internal organizational use, including clinics. Third-party hosting and embedding in a commercial service require separate permission. |
| Change Date | November 21, 2030 for this preview. An earlier first distribution starts the four-year limit earlier. |
| Change License | Mozilla Public License 2.0 with its standard secondary-license provisions. |

The [distributed license](../LICENSE) gives the terms. Third-party components retain their own terms. The software is provided as is to the extent applicable law permits. The publisher and maintainers do not provide medical care, monitor emergencies, or warrant the accuracy or completeness of model output. Users retain rights and remedies that applicable law makes mandatory.

### Distribution record

The first public GitHub source history runs from `32b5104` through `e9153cd` on November 21, 2026 (UTC). All eleven revisions identified themselves as 0.2.0. Their four-year anniversary is November 21, 2030. There were no version tags at that check. Earlier copies carried an incomplete BUSL parameter block. Preserve that history and have counsel confirm the treatment of those grants and any earlier distribution outside GitHub. Each later version needs its own first distribution date, revision, and change date. [Public revision history](https://github.com/ctlplne/healthctl/commits/e9153cd1b72a003b1c6a7f1860878157598eec71/)

### Public preview and release review

The Apple silicon installer prerelease `macos-0.2.0-beta.1` distributes the native launcher, its backup helper and 0.2.0 build source. First launch builds the Linux services on the user's machine. Release assets include the native inventory and disk-image checksum. The software remains a public preview for visit preparation. The download is ad-hoc signed and unnotarized. The outstanding clinical and provider reviews below apply to this build.

For each release keep a dated record with the commit, artifacts and digests, intended users and markets, enabled models, patient-visible claims, clinical reviewer, legal reviewer, known case failures, decision and responsible maintainer. Record outstanding reviews explicitly. Keep private legal records and contracts outside the repository.

| November 21, 2026 · 0.2.0 preview | Current record |
|---|---|
| Audience and distribution | English-speaking adults, US and international interest. Reddit is the planned announcement channel for the source repository. The installation operator chooses where to run it. |
| Clinical review | Qualified clinical and regulatory review of real-patient differentials and warnings is outstanding. The patient can see individualized possibilities before clinician review. Synthetic cases measure scripted software behavior. No clinical accuracy study or endorsement is claimed. |
| Provider terms | Review each enabled provider, subscription route, use case, geography, and any requirement for licensed professional involvement. Record the decision for patient-facing delivery and how it is enforced in the API and exports before claiming approval. [OpenAI service terms](https://openai.com/policies/service-terms/) · [OpenAI usage policies](https://openai.com/policies/usage-policies/) · [Anthropic usage policy](https://www.anthropic.com/legal/aup) |
| Privacy and geography | Counsel should assess the actual operator, data flows and business relationships, including US HIPAA, FTC health breach and state health privacy rules, and relevant Indian or EU rules where the product is offered or operated. Free access, self-hosting and a Reddit launch do not determine applicability. [HHS health app resources](https://www.hhs.gov/hipaa/for-professionals/special-topics/health-apps/index.html) · [FTC health breach rule](https://www.ftc.gov/business-guidance/resources/health-breach-notification-rule-basics-business) |
| Ownership and risk | Maintain evidence of company ownership, assignments as applicable and any employment/IP obligations. Counsel should review claims, warranty and liability terms, non-waivable rights, and coverage for the actual functionality. |
| Binary redistribution | Review the exact build inventories, licenses, corresponding-source delivery and territorial scope of the bundled excerpts. Record who completed each review and how recipients can obtain required source. [Third-party notices](../THIRD_PARTY_NOTICES.md) |
| Incident owner and withdrawal | The repository maintainer triages private fictional reports, records affected revisions, preserves the failing synthetic case and assesses affected users. For a harmful build, stop promotion, mark the release and affected tags in a public advisory without patient data, identify a tested fixed version or safe recovery steps, and update the README and release notes. Self-hosted users control their own updates. [Reporting procedure](../SECURITY.md) |

## ChatGPT subscription qualification

The direct integration is implemented for a personal loopback installation. Before a paid or managed release, establish the applicable integration terms with OpenAI for that distribution and complete an account-authorized synthetic probe. Record the model actually offered, the result of the probe and the applicable workspace credit controls. A successful probe qualifies the connection for that account at that time. Clinical review and deployment-specific handling of health information remain separate release decisions. [OpenAI integration scope](https://developers.openai.com/siwc/token-sharing-open-source) · [Provider qualification](model-providers.md#use-a-chatgpt-subscription)
