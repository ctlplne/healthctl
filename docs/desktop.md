# healthctl for macOS

The local desktop build packages healthctl, PostgreSQL, and the English whisper.cpp model for the architecture of the Mac that builds it. The tested package targets Apple silicon. Docker Desktop must be installed and running. The launcher can open Docker Desktop or its official download page.

## Build and install

The public repository currently distributes the source. To build the disk image, install Git, Go 1.26 or later and Xcode Command Line Tools, and start Docker Desktop. The first build downloads dependencies, images and the speech model. To start from source, run the following commands.

```sh
git clone https://github.com/ctlplne/healthctl.git
cd healthctl
./scripts/build-desktop.sh
open "build/desktop/healthctl-$(uname -m).dmg"
```

Drag **healthctl** to **Applications**. Open it and choose **Open journal or try the demo**. Create the installation owner, open **Model settings**, select the synthetic demo, save it, and start a new encounter. **Connect a model** opens settings for a real connection. A ChatGPT plan can be linked through the browser. The optional **Set or clear an OpenAI API key** button configures separately billed platform features. The launcher stores the journal in its local workspace and binds the web server to loopback on its own persistent port. The provider you select receives the history approved for that destination.

The installed launcher uses the images and speech model bundled in the disk image. Docker Desktop is the runtime requirement. Open the journal from the launcher to use its assigned address. A separate Compose checkout has its own port, owner and database.

## Backup and recovery

Choose **Back up workspace**, a new filename and a passphrase of at least 12 characters. The backup includes the installation encryption key, encrypted journal, diary and original report files. The launcher streams the database dump through authenticated encryption. Keep the passphrase separately. Sessions, active handoff access codes and ChatGPT credentials are excluded. Platform keys in local settings are also excluded.

**Restore a backup into a new workspace** creates a fresh database, verifies the archive and installation key, and makes the restored workspace active after it passes health checks. Sign in with the original owner account and reconnect any model credentials. The previous workspace and data remain on the Mac. Archives created by the Compose database scheduler use its separate documented restore procedure. [Compose recovery](operations.md)

## Distribution and verification

The default build has a local ad-hoc signature. For distribution use an Apple Developer ID Application identity and a configured notarytool keychain profile.

```sh
HEALTHCTL_SIGN_IDENTITY="Developer ID Application: YOUR REGISTERED NAME (TEAMID)" \
HEALTHCTL_NOTARY_PROFILE="your-notary-profile" ./scripts/build-desktop.sh
```

To verify an Apple silicon build, use Python 3 and the reference recording from the same whisper.cpp version used in the image. The drill mounts the image, starts a synthetic workspace, transcribes the clip offline, saves a diary entry, exercises a failed restore and checks the restored record after a fresh login.

```sh
curl --fail --location --output build/desktop/jfk.wav https://raw.githubusercontent.com/ggml-org/whisper.cpp/v1.9.5/samples/jfk.wav
python3 scripts/verify_desktop.py build/desktop/healthctl-arm64.dmg build/desktop/jfk.wav
```
