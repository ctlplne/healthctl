# Continuous integration and releases

A push to `main` or a pull request targeting it runs the backend, browser, evaluation and documentation jobs on GitHub-hosted Linux runners. Each job gets a fresh checkout and read-only repository access. CI uses synthetic patient records, generated installation credentials and provider fixtures. The patient journal and linked subscriptions stay on the owner's installation. [CI workflow](../.github/workflows/ci.yml)

The backend runs race-enabled Go and PostgreSQL integration tests. The browser job builds the production frontend and exercises desktop and mobile Chromium, provider failures and printed exports. The offline evaluator compares each case and interview arm with the versioned baseline. Its Actions summary lists fabricated findings, stale assessments, lost warnings, and newly missed history targets by case. The scorecard and synthetic failure screenshots expire after seven days. [Case baseline](../evaluation/release-fixture-results.json)

## Recovery and package checks

**Release qualification** runs weekly, when invoked from the Actions tab, and on release tags. It exercises a clean Compose installation, encrypted restore, upgrade and rollback from the first public commit, offline FHIR validation, and the Orca screen-reader journey. The original pre-public MVP image remains available only to installations that retained it. [Qualification workflow](../.github/workflows/qualification.yml) · [Original MVP upgrade drill](combined-verification.md#upgrade-from-the-original-mvp)

Native Linux x86-64 and Arm64 runners test the actual packaged images with a synthetic encounter, offline transcription and an encrypted desktop backup and restore. An Apple silicon macOS runner assembles the disk image, mounts it, verifies the ad-hoc signature, and compares its images with the tested Arm payload. Download the ad-hoc DMG from the workflow run for testing. Release tags can also sign and notarize the tested application on GitHub. The complete installed-launcher test with Docker Desktop remains available on a Mac. [macOS distribution and verification](desktop.md#distribution-and-verification)

## Publish a version

1. Choose a release commit on `main` and set its application and installer versions. Push a matching tag in `vMAJOR.MINOR.PATCH` form. The **Prepare release** workflow verifies the tag, reruns CI and release qualification for that commit, then creates a draft GitHub Release with tested Linux image archives, checksums and build attestations.
2. Review the release notes and distribution decisions. When hosted signing is enabled, the draft includes the notarized Mac installer and its attestation. Otherwise a maintainer can attach a separately signed, notarized and tested DMG. The draft workflow keeps the ad-hoc test package in Actions. [Release decisions](release.md)
3. Publish the draft on GitHub when ready. **Publish release images** verifies both archives against their tag and build attestations, then promotes those images to `ghcr.io/ctlplne/healthctl:vMAJOR.MINOR.PATCH`. On the first publication, check the package's visibility and enable public read access for public distribution. Each operator chooses when to update their installation. [Prepare release](../.github/workflows/release.yml) · [Publish images](../.github/workflows/publish.yml)

To exercise everything up to packaging without creating a tag or a release, choose **Run workflow** in **Release qualification** on `main`. For an individual code change, **CI** also supports a manual run. Live model qualification is a separate operator-initiated check. [Model verification](model-providers.md#reproduce-adapter-checks)


## Enable hosted Mac signing

A maintainer with an Apple Developer ID Application identity can configure the GitHub environment `macos-signing` for release tags. Store the base64-encoded PKCS#12 identity in `APPLE_CERTIFICATE_P12` and its password in `APPLE_CERTIFICATE_PASSWORD`. Store the PEM contents of an App Store Connect API key in `APPLE_NOTARY_KEY_P8`, its ID in `APPLE_NOTARY_KEY_ID`, and the issuer ID in `APPLE_NOTARY_ISSUER` for a team key. Set the repository variable `HEALTHCTL_MACOS_SIGNING=true` when the environment is ready. The tag workflow imports the identity into a temporary keychain, signs the already tested app, submits the DMG to Apple, staples the ticket and checks Gatekeeper acceptance. A signing failure blocks that draft. [Apple notarization](https://developer.apple.com/documentation/security/notarizing-macos-software-before-distribution)
