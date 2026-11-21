# Continuous integration and releases

A push to `main` or a pull request targeting it runs the backend, browser, evaluation and documentation jobs on GitHub-hosted Linux runners. Each job gets a fresh checkout and read-only repository access. CI uses synthetic patient records, generated installation credentials and provider fixtures. The patient journal and linked subscriptions stay on the owner's installation. [CI workflow](../.github/workflows/ci.yml)

The backend runs race-enabled Go and PostgreSQL integration tests. The browser job builds the production frontend and exercises desktop and mobile Chromium, provider failures and printed exports. The offline evaluator compares each case and interview arm with the versioned baseline. Its Actions summary lists fabricated findings, stale assessments, lost warnings, and newly missed history targets by case. The scorecard and synthetic failure screenshots expire after seven days. [Case baseline](../evaluation/release-fixture-results.json)

## Recovery and package checks

**Release qualification** runs weekly, when invoked from the Actions tab, and on release tags. It exercises a clean Compose installation, encrypted restore, upgrade and rollback from the first public commit, offline FHIR validation, and the Orca screen-reader journey. The original pre-public MVP image remains available only to installations that retained it. [Qualification workflow](../.github/workflows/qualification.yml) · [Original MVP upgrade drill](combined-verification.md#upgrade-from-the-original-mvp)

Native Linux x86-64 and Arm64 runners test the actual packaged images with a synthetic encounter, offline transcription and an encrypted desktop backup and restore. An Apple silicon macOS runner assembles the disk image, mounts it, verifies the ad-hoc signature, and compares its images with the tested Arm payload. Download the ad-hoc DMG from the workflow run for testing. Developer ID signing, Apple notarization, and the complete installed-launcher test with Docker Desktop use the documented Mac procedure. [macOS distribution and verification](desktop.md#distribution-and-verification)

## Publish a version

1. Choose a release commit on `main` and set its application and installer versions. Push a matching tag in `vMAJOR.MINOR.PATCH` form. The **Prepare release** workflow verifies the tag, reruns CI and release qualification for that commit, then creates a draft GitHub Release with tested Linux image archives, checksums and build attestations.
2. Review the release notes and distribution decisions. To include a public Mac installer, attach the Developer ID signed, notarized and tested DMG. The draft workflow keeps the ad-hoc test package in Actions. [Release decisions](release.md)
3. Publish the draft on GitHub when ready. **Publish release images** verifies both archives against their tag and build attestations, then promotes those images to `ghcr.io/ctlplne/healthctl:vMAJOR.MINOR.PATCH`. On the first publication, check the package's visibility and enable public read access for public distribution. Each operator chooses when to update their installation. [Prepare release](../.github/workflows/release.yml) · [Publish images](../.github/workflows/publish.yml)

To exercise everything up to packaging without creating a tag or a release, choose **Run workflow** in **Release qualification** on `main`. For an individual code change, **CI** also supports a manual run. Live model qualification is a separate operator-initiated check. [Model verification](model-providers.md#reproduce-adapter-checks)
