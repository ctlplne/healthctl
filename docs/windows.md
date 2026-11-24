# healthctl for Windows

The launcher opens the same experimental visit-preparation journal as Docker Compose. Read **Why healthctl exists and what the beta can do** in the journal before starting. The use notice and license are available offline. Verify findings with a qualified healthcare professional. [Beta purpose and limitations](use-notice.md)

Use a supported 64-bit Windows PC with an Intel or AMD processor, hardware virtualization enabled and Docker Desktop running Linux containers. The portable launcher uses the .NET Framework included with supported Windows desktop releases. Windows on ARM is outside this release’s test coverage. [Docker Desktop requirements and WSL 2 installation](https://docs.docker.com/desktop/setup/install/windows-install/)

1. Start Docker Desktop. Wait until its Linux engine is running.
2. Download the **healthctl-windows-x64.exe** asset from the [Windows beta release](https://github.com/ctlplne/healthctl/releases/tag/v0.2.0-beta.6). This beta is unsigned. SmartScreen or your organization’s policy may block it.
3. Double-click the launcher and choose **Open journal or try the demo**. Keep Docker and your internet connection running on the first launch. The launcher builds healthctl from its bundled source and downloads the pinned PostgreSQL image. Subsequent starts reuse those images.
4. Your browser opens the journal at a loopback address. Create your local owner account and choose the synthetic demo or configure a model. [Watch the ChatGPT setup](https://healthctl.com/#chatgpt-setup)

## Reopen or update your journal

Use the same Windows account and Docker Desktop context. Replace the executable with the new verified download and open it. The launcher reuses the port, encryption key and Docker volumes. The workspace settings live under `%LOCALAPPDATA%\healthctl\workspace`; the encrypted journal lives in a Docker volume. Keep both when migrating or backing up the computer. A Docker Desktop factory reset can remove volumes. The Windows launcher currently offers startup and reopening. The Mac launcher's encrypted backup and restore buttons are specific to macOS.

If startup fails, read the launcher status. Check that Docker Desktop has started, uses Linux containers, has space for the build, and can download its dependencies. Select a local Docker context if the message mentions a remote engine. The Windows launcher requires no Git or Python installation.

## Verify or inspect the download

Compare the output of the PowerShell command below with the **SHA256SUMS** release asset. To inspect the bundled source, inventory and notices, run the second command from the download folder and choose a new empty directory.

```powershell
Get-FileHash .\healthctl-windows-x64.exe -Algorithm SHA256
.\healthctl-windows-x64.exe --extract .\healthctl-package
```

GitHub Actions builds and opens the x64 graphical launcher on a Windows runner. A separate Linux runner takes that exact Windows executable, runs its managed startup code under Mono with a real Linux Docker engine, and checks the owner, fictional interview, export and restart. The tagged beta release includes the tested executable, its inventory and SHA-256 checksum after both jobs pass. The split test leaves a consumer Windows PC with Docker Desktop, WSL 2 and SmartScreen as a separate manual check.
