# healthctl for macOS

The public beta launcher runs on Apple silicon Macs with Docker Desktop. On first use, Docker fetches dependencies and builds healthctl from the source included in the download. PostgreSQL and the English speech model run locally. Subsequent launches reuse the local images.

## Download the Apple silicon beta

1. Install and start Docker Desktop. Use a macOS version supported by your Docker Desktop installation.
2. Download the [unnotarized beta disk image](https://github.com/ctlplne/healthctl/releases/download/macos-0.2.0-beta.1/healthctl-0.2.0-beta.1-macos-arm64.dmg). The [release page](https://github.com/ctlplne/healthctl/releases/tag/macos-0.2.0-beta.1) includes the SHA-256 checksum. This preview is ad-hoc signed. Apple has not verified its publisher or notarized it, and Gatekeeper may block opening it. The [Docker Compose installation](../README.md#docker-compose) is also available.
3. Open the DMG, drag **healthctl** into **Applications**, and open healthctl. Choose **Open journal or try the demo**. Allow several minutes and keep Docker and your internet connection running during the first setup.
4. When your browser opens the local journal, create your owner account. Choose the scripted demo to explore with fictional information, or configure your own model. A cloud model receives the relevant history directly from your installation.

## Build the bundled installer from source

The default source build bundles the Linux images inside the disk image for an offline first launch. To build it, install Git, Go 1.26 or later and Xcode Command Line Tools, and start Docker Desktop. The first build downloads dependencies, images and the speech model.

```sh
git clone https://github.com/ctlplne/healthctl.git
cd healthctl
./scripts/build-desktop.sh
open "build/desktop/healthctl-$(uname -m).dmg"
```

Drag **healthctl** to **Applications**. Open it and choose **Open journal or try the demo**. Create the installation owner, open **Model settings**, select the synthetic demo, save it, and start a new encounter. **Connect a model** opens settings for a real connection. A ChatGPT plan can be linked through the browser. The optional **Set or clear an OpenAI API key** button configures separately billed platform features. The launcher stores the journal in its local workspace and binds the web server to loopback on its own persistent port. The provider you select receives the history approved for that destination.

Open the journal from the launcher to use its assigned address. A separate Compose checkout has its own port, owner and database.

## Backup and recovery

Choose **Back up workspace**, a new filename and a passphrase of at least 12 characters. The backup includes the installation encryption key, encrypted journal, diary and original report files. The launcher streams the database dump through authenticated encryption. Keep the passphrase separately. Sessions, active handoff access codes and ChatGPT credentials are excluded. Platform keys in local settings are also excluded.

**Restore a backup into a new workspace** creates a fresh database, verifies the archive and installation key, and makes the restored workspace active after it passes health checks. Sign in with the original owner account and reconnect any model credentials. The previous workspace and data remain on the Mac. Archives created by the Compose database scheduler use its separate documented restore procedure. [Compose recovery](operations.md)

## Distribution and verification

The default build has a local ad-hoc signature. To reproduce the smaller public preview, check out its tagged revision and run `./scripts/build-desktop.sh --build-on-first-use`. For an Apple-notarized release use a Developer ID Application identity and a configured notarytool keychain profile.

```sh
HEALTHCTL_SIGN_IDENTITY="Developer ID Application: YOUR REGISTERED NAME (TEAMID)" \
HEALTHCTL_NOTARY_PROFILE="your-notary-profile" ./scripts/build-desktop.sh
```

To verify an Apple silicon build, use Python 3 and the reference recording from the same whisper.cpp version used in the image. The drill mounts the image, starts a synthetic workspace, transcribes the clip offline, saves a diary entry, exercises a failed restore and checks the restored record after a fresh login.

```sh
curl --fail --location --output build/desktop/jfk.wav https://raw.githubusercontent.com/ggml-org/whisper.cpp/v1.9.5/samples/jfk.wav
python3 scripts/verify_desktop.py build/desktop/healthctl-arm64.dmg build/desktop/jfk.wav
```
