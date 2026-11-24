# Intended use and release decisions

healthctl is a ctlplne studio product distributed as a free public preview (beta). It gathers a text-based symptom history from English-speaking adults and estimates possible explanations for the reporting user to discuss with a qualified professional. A person may view the result before visiting a clinician. The application has no clinical signing or prescribing workflow. Source code, self-hosting, warnings and successful synthetic tests do not establish safety, clinical accuracy, regulatory status, or privacy compliance.

The initial emergency policy is development content. Have qualified clinicians review its handling of missed and excessive alerts, uncertainty, correction and scope, and evaluate the user-facing presentation before real-world release. Local policies cover only a restricted set of phrases. An LLM can misread evidence, suggest unsupported explanations, or neglect important questions. The evidence validator cannot establish clinical truth. Qualify actual provider/model versions, including abstention, across realistic histories and accessibility needs. Clinician review of this software is distinct from a clinician reviewing any individual user report.

Establish intended audience, distribution, intended-use labeling, and jurisdiction-specific review for the US and each intended market. The FDA's clinical decision support guidance explains the criteria for the US non-device CDS exclusion and addresses the applicability of other digital health policies to user-facing functions. Individualized suggestions and urgency messaging warrant specific qualified regulatory review. Labels such as “beta” and “user” do not determine which rules apply. Review the actual outputs, marketing claims and whether people may rely on them. US advertising claims should reflect the available evidence. [FTC health products guidance](https://www.ftc.gov/business-guidance/resources/health-products-compliance-guidance)

No classification, approval, exemption, clinical validation or HIPAA status is asserted here. [FDA clinical decision support guidance](https://www.fda.gov/regulatory-information/search-fda-guidance-documents/clinical-decision-support-software)

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

#### 0.2.0-beta.6 · November 24, 2026

The Apple silicon DMG (bundle build 8), Windows x64 EXE (file version 0.2.0.6) and prebuilt Linux AMD64 and ARM64 images share revision `ab34ecd5bab0886b63da29f85df9eb8a5b1c4ca7`. The public container tag `ghcr.io/ctlplne/healthctl:v0.2.0-beta.6` resolves to multi-architecture digest `sha256:b40b250544afe109f03eb5b1474607db004197141446331fc0d8017474518a1a`. This beta carries the hardening work recorded in [HARDENING.md](../HARDENING.md): the app asks its own safety questions, downloads and other sensitive actions ask for the password again, an owner can reset a forgotten password from the launchers, and the app is built with Go 1.26.11. The [changelog](../CHANGELOG.md) lists what an upgrade changes. The DMG is ad-hoc signed and unnotarized. The EXE is unsigned. [Release files and SHA-256 checksums](https://github.com/ctlplne/healthctl/releases/tag/v0.2.0-beta.6)

GitHub Actions exercised the release from its tag. The run covered backend, browser, safety-corpus and synthetic-case checks; the dependency vulnerability scan; the Windows launcher on Windows and against Linux containers; both Linux image bundles and Mac packaging; and upgrade from the first public version with rollback and FHIR validation, encrypted restore and the screen-reader journey. `SHA256SUMS` matched GitHub's digest of every release file. The tag was first pushed on `749ad01`. That run failed when exporting the tested image, before any draft existed, so the tag was moved to the fixed revision. Still open: a first launch of the exact downloads on a Mac and on a consumer Windows PC, clinical review, and jurisdiction-specific legal and provider review. [Tagged verification](https://github.com/ctlplne/healthctl/actions/runs/45380707766)

#### 0.2.0-beta.5 · November 22, 2026

The Apple silicon DMG (bundle build 7), Windows x64 EXE (file version 0.2.0.5) and prebuilt Linux AMD64 and ARM64 images share revision `0e70f49c69f33b2a951d5787af3882c4f50498a9`. The public container tag `ghcr.io/ctlplne/healthctl:v0.2.0-beta.5` resolves to multi-architecture digest `sha256:13b48be205aa1ab56efe785ca9440e7cc921737c2ab9fb61d2930a4fd2202b22`. The user confirms the reviewed history and separately acknowledges the limitations before requesting or retrying an optional assessment. Immediate warnings and history-only export remain available. The DMG is ad-hoc signed and unnotarized. The EXE is unsigned. [Release files and SHA-256 checksums](https://github.com/ctlplne/healthctl/releases/tag/v0.2.0-beta.5)

GitHub Actions exercised both Linux architectures, the Windows launcher, Mac packaging, synthetic cases, encrypted restore, upgrade and rollback, screen-reader navigation and all 50 browser tests. All 176 source files embedded in the two desktop packages matched the tag. The exact Mac download also completed first launch with Docker Desktop, offline transcription and encrypted backup and restore on Apple silicon. Clinical review, jurisdiction-specific legal and provider review, and a consumer Windows PC check with Docker Desktop, WSL 2 and SmartScreen remain open. [Tagged verification](https://github.com/ctlplne/healthctl/actions/runs/45014396186)

#### 0.2.0-beta.4 · November 22, 2026

The paired 0.2.0-beta.4 downloads and the equivalent Docker Compose checkout use commit `6af2f63498445c8e6f062ef513908a9d0f6b3fb3`. The Apple silicon DMG (bundle build 6) is ad-hoc signed and unnotarized. The Windows x64 EXE (file version 0.2.0.4) is unsigned. Both build the same journal from included source on first use. Each release includes an SPDX inventory and SHA-256 checksums. The Windows launcher starts and reopens a workspace. The Mac launcher also provides native backup and restore controls. See the dated clinical and legal review record below. [Mac release](https://github.com/ctlplne/healthctl/releases/tag/macos-0.2.0-beta.4) · [Windows release](https://github.com/ctlplne/healthctl/releases/tag/windows-0.2.0-beta.4)

November 22, 2026. The release adds the same experimental-use statement to both launchers, the browser journal, offline invitations and printed reports. The offline use notice explains the research, the publisher’s warranty disclaimer, the limited scope of its liability wording and users’ non-waivable rights. The unshipped beta 3 candidate failed the multipage PDF text check. Beta 4 keeps the verification sentence together on the page. Software testing and publication do not constitute clinical or legal sign-off.

All 176 files embedded in the two source archives were compared with each other and with the tagged Docker build inputs. The DMG SHA-256 is `41baa15e57318b18a02ebc4c3db831298d3a79871fc89d952d514b4d66110d17`. The EXE SHA-256 is `38869e1ee79490e20e412c06be0fbb648e876d995ac0143165f7affd466d60e7`. The native Windows GUI check and the Linux service drill exercise different parts of the Windows install. A fresh Windows PC with Docker Desktop, WSL 2 and SmartScreen remains a manual installation check. [Windows package and release checks](https://github.com/ctlplne/healthctl/actions/runs/45011448560)

For each release keep a dated record with the commit, artifacts and digests, intended users and markets, enabled models, user-visible claims, clinical reviewer, legal reviewer, known case failures, decision and responsible maintainer. Record outstanding reviews explicitly. Keep private legal records and contracts outside the repository.

| November 21, 2026 · 0.2.0 preview | Current record |
|---|---|
| Audience and distribution | English-speaking adults, US and international interest. Reddit is the planned announcement channel for the source repository. The installation operator chooses where to run it. |
| Clinical review | Qualified clinical and regulatory review of real-user differentials and warnings is outstanding. The user can see individualized possibilities before clinician review. Synthetic cases measure scripted software behavior. No clinical accuracy study or endorsement is claimed. |
| Provider terms | Review each enabled provider, subscription route, use case, geography, and any requirement for licensed professional involvement. Record the decision for user-facing delivery and how it is enforced in the API and exports before claiming approval. [OpenAI service terms](https://openai.com/policies/service-terms/) · [OpenAI usage policies](https://openai.com/policies/usage-policies/) · [Anthropic usage policy](https://www.anthropic.com/legal/aup) |
| Privacy and geography | Counsel should assess the actual operator, data flows and business relationships, including US HIPAA, FTC health breach and state health privacy rules, and relevant Indian or EU rules where the product is offered or operated. Free access, self-hosting and a Reddit launch do not determine applicability. [HHS health app resources](https://www.hhs.gov/hipaa/for-professionals/special-topics/health-apps/index.html) · [FTC health breach rule](https://www.ftc.gov/business-guidance/resources/health-breach-notification-rule-basics-business) |
| Ownership and risk | Maintain evidence of company ownership, assignments as applicable and any employment/IP obligations. Counsel should review claims, warranty and liability terms, non-waivable rights, and coverage for the actual functionality. |
| Binary redistribution | Review the exact build inventories, licenses, corresponding-source delivery and territorial scope of the bundled excerpts. Record who completed each review and how recipients can obtain required source. [Third-party notices](../THIRD_PARTY_NOTICES.md) |
| Incident owner and withdrawal | The repository maintainer triages private fictional reports, records affected revisions, preserves the failing synthetic case and assesses affected users. For a harmful build, stop promotion, mark the release and affected tags in a public advisory without user data, identify a tested fixed version or safe recovery steps, and update the README and release notes. Self-hosted users control their own updates. [Reporting procedure](../SECURITY.md) |

## ChatGPT subscription qualification

November 24, 2026: on a throwaway loopback installation, the maintainer connected one ChatGPT account through Sign in with ChatGPT. The account offered GPT-6.2-Sol among its models. **Test connection** succeeded with that model, and synthetic interviews, history reviews and an assessment with the stability check completed through the plan. The account's credit settings were not inspected.

The direct integration is implemented for a personal loopback installation. Before a paid or managed release, establish the applicable integration terms with OpenAI for that distribution and complete an account-authorized synthetic probe. Record the model actually offered, the result of the probe and the applicable workspace credit controls. A successful probe qualifies the connection for that account at that time. Clinical review and deployment-specific handling of health information remain separate release decisions. [OpenAI integration scope](https://developers.openai.com/siwc/token-sharing-open-source) · [Provider qualification](model-providers.md#use-a-chatgpt-subscription)
