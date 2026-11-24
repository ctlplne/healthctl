# Operating a personal installation

These commands operate on a Docker Compose checkout. For a workspace created by the macOS launcher, use the launcher’s encrypted backup and restore controls. [macOS backup and recovery](desktop.md#backup-and-recovery)

## Access and secrets

The default display name is `healthctl`. The Compose project still uses `clinical-history` so existing installations open the same PostgreSQL volume after an upgrade. Owners can override the display name with `CLINICAL_DISPLAY_NAME`.

One owner account controls one installation. Choose a unique password and store it in a password manager. Passwords use bcrypt at cost 12. Session cookies are HTTP-only, same-site strict, expiring after 12 hours. The database stores session token hashes. Authenticated writes require a session-bound CSRF value and the HTTP server checks request origin and hostname. Sessions, exports and UI responses use no-store cache controls. Login attempts share a persistent budget of ten per ten-minute window on this single-owner installation.

The app encrypts each clinical document with AES-256-GCM and a fresh, library-generated nonce, under a data key. The installation key in `.env` only wraps the data key and a separate, random key for submission receipts, so rotating the installation key re-wraps those two keys and never rewrites the records. An installation created before this key hierarchy keeps its original key as its data key. Associated data binds the owner and encounter IDs. A key verifier detects incorrect restore keys before the service starts. Metadata such as row IDs, timestamps, dataset size, owner username, password hash and nonclinical provider configuration remains visible to the database operator. Use an encrypted host volume and encrypted backups for this metadata, the private environment file, and downloaded reports. Access to the host, its Docker daemon, or both the data and its key gives access to the history. The key also encrypts uploaded originals using owner, encounter and document identifiers as associated data. Use the offline rotation command below to change keys while preserving the database and retry receipts.

ChatGPT OAuth credentials use AES-256-GCM with the owner and host ID as associated data. Offline data-key rotation rewraps the credential vault. Owner recovery removes all subscription links.

Secrets reside on the server. A local `.env` is created with mode 0600 and excluded from the build and repository. Compose passes the installation key and the database password to the containers as secret files under `/run/secrets` (this needs Docker Compose 2.23 or later), so they don't appear in the containers' configuration. A production operator may use the provider adapter's `*_API_KEY_FILE` variables with explicit Docker secret mounts in a private Compose override. The local document and speech readers run with a fixed, minimal environment, so no key, password or URL reaches them, and on Linux in their own network namespace when the system allows one. Docker's default security profile doesn't, so inside the standard container they share the app's network; the installation safety panel shows which applies. Supply one source for each key. Keep state, backups and secrets outside the source tree when distributing software. Backup storage keys should be managed separately from the database archive.

## Locale and TLS

The installation defaults to generic local-emergency wording. To supply verified local contact details, set `CLINICAL_EMERGENCY_CONTACT`. Set `CLINICAL_DISPLAY_NAME` for a personal display name. Recreate the app to apply environment changes. Standard setup supports English-speaking adults. The operator chooses locale information independently of their own travel location.

For an intentional HTTPS deployment, finish owner setup on loopback first. Keep the app bound to loopback, terminate TLS at a reverse proxy on the host, preserve the request Host header, and forward to the configured loopback port (default `127.0.0.1:8437`). Set `CLINICAL_PUBLIC_ORIGIN=https://your-actual-host` and `CLINICAL_SECURE_COOKIE=true`, then recreate the app and log in over HTTPS. Keep PostgreSQL on the private network. Configure the proxy to avoid logging bodies, query strings, cookies and authorization headers; disable its response caching. Proxies, system crash dumps, host snapshots and browser extensions are outside application control.

## Backup

From the project root, set `archive` to a new file on an encrypted backup drive you control. The example refuses to overwrite an existing file. A successful dump and archive listing are the first checks before a restore drill.

```sh
archive="/absolute/path/to/encrypted/backups/clinical-history-$(date +%Y%m%d-%H%M%S).dump"
(
  umask 077
  set -C
  docker compose exec -T db pg_dump -U clinicalhistory -d clinicalhistory -Fc --no-owner --no-acl --exclude-table-data=public.sessions --exclude-table-data=public.brief_access --exclude-table-data=public.chatgpt_vault > "$archive"
) && docker compose exec -T db pg_restore --list < "$archive" > /dev/null
```

Keep an independently protected recovery copy of `CLINICAL_DATA_KEY` and the installation configuration. Restore the matching source version first. The database archive contains encrypted clinical rows and other sensitive metadata. Session cookies from the original installation cannot be reused with a session-free restore. The documented manual and scheduled backups also exclude the ChatGPT host ID and account credentials. After restore, reconnect in Model settings and confirm any account change on each existing encounter. Full raw database or VM snapshots can contain live credentials.

## Restore into a fresh installation

Use a separate checkout directory and **a new Compose project name** to get an empty volume. Run `python3 scripts/init.py` in that checkout. In its private `.env`, replace `CLINICAL_DATA_KEY` with the key that matches the backup and set `CLINICAL_PORT=18438` (or another free port). Keep the newly generated database password. Set `archive` to the absolute path of the dump you verified. Keep the old installation and backups intact while verifying the restored copy.

```sh
docker compose -p clinical-history-restore up -d --wait db
docker compose -p clinical-history-restore exec -T db pg_restore -U clinicalhistory -d clinicalhistory --exit-on-error --single-transaction --no-owner --no-acl < "$archive"
docker compose -p clinical-history-restore up -d --build --wait
```

Open the restored installation on the port you chose, sign in with the backed-up owner credentials and open an encounter and its export. A wrong key causes startup to fail. No browser or cloud action can recover a lost key. Deleting an encounter removes its primary application records. Previous exports, provider-held copies, backups and PostgreSQL recovery history follow their respective retention policies.

## Scheduled backups

Scheduled backups run by default with `docker compose up`. `scripts/init.py` writes `CLINICAL_BACKUP_DIRECTORY` for a new installation, pointing to a `healthctl-backups` folder in your home directory, outside Docker and outside this checkout, so `docker compose down -v`, a Docker reset or deleting the checkout doesn't remove the backups too. Choose another folder with `python3 scripts/init.py --backup-directory /path/on/another/drive`. On Linux, init also sets `CLINICAL_BACKUP_USER` to your user so the worker can write there. An older `.env` without `CLINICAL_BACKUP_DIRECTORY` keeps using the `clinical_backups` Docker volume, and the journal says so.

```sh
docker compose logs --tail 10 backup
```

The worker writes a PostgreSQL custom-format snapshot immediately, verifies its archive listing and repeats every `CLINICAL_BACKUP_INTERVAL_HOURS` (default 24). It retains `CLINICAL_BACKUP_KEEP` snapshots (default seven) and limits cleanup to its own dated filenames. Session tokens, brief access grants and the ChatGPT subscription vault are excluded. Each run records its time and outcome, never any clinical content, so the journal can show when the last backup finished or that one failed. A failed run logs `backup: FAILED` and leaves earlier snapshots unchanged. To pause scheduled backups, run `docker compose stop backup`.

The snapshots don't contain the installation key. Save a recovery kit so a backup can be opened after this computer is lost:

```sh
python3 scripts/recovery_kit.py
```

The script writes a private file with the key and database password to your home folder (or `--output PATH`) and prints a short fingerprint. Move the file to a password manager or encrypted drive, then enter the fingerprint in the journal. The journal keeps asking until the fingerprint matches the key in use, asks again after a key rotation, and asks you to check the kit every six months.

`scripts/init.py` refuses to create a new key when this Compose project already has a data volume, because a new key cannot read the existing journal. Restore the `.env` from your recovery kit instead.

## Owner recovery and key rotation

These commands require local Docker administration and the installation configuration. Run from the project root. Use an encrypted backup made with the current key before maintenance.

```sh
python3 scripts/maintenance.py recover-owner
python3 scripts/maintenance.py rotate-key
```

Recovery prompts privately for a new password, stops the HTTP service, updates the bcrypt hash, invalidates sessions and external brief grants, and restarts. It preserves the owner and clinical history. To look up the owner username from the local administrator account, run `docker compose exec -T db psql -U clinicalhistory -d clinicalhistory -Atc "SELECT username FROM owners WHERE id=1"`. Rotation verifies and re-encrypts encounters, diary entries and original uploads in a single database transaction. It preserves idempotency receipts through a wrapped signing key. The script keeps a protected pre-rotation configuration in `.secrets/` for older backups and atomically replaces the active key. If interrupted, rerun the same rotate command. A staged key lets it resume after a database commit. Move pre-rotation configuration copies into controlled backup custody. The database and matching keys are both required for recovery. Use `--project NAME --env-file PATH` for a named local installation. The operating-system wrapper supports macOS and Linux.

## Upgrades and rollback

Upgrade with `scripts/upgrade.py`. It writes a verified pre-upgrade snapshot to the backup folder first (`pre-upgrade-<time>.dump`), keeps the current image as `clinical-history-app:previous`, and only then rebuilds and restarts. If the snapshot fails, nothing changes. For a clean checkout tracking `main`, run these commands; for a pinned installation, check out the intended revision in place of the pull.

```sh
git pull --ff-only origin main
python3 scripts/upgrade.py
```

To go back, `python3 scripts/upgrade.py --rollback` stops the app, recreates the database from the pre-upgrade snapshot and starts the previous image. Anything recorded after the upgrade is not kept, so export it first.

**Desktop apps.** Choose **Back up now** before installing a new version. To roll back, reinstall the previous version and restore that backup with the same key.

**PostgreSQL major versions.** The image pins PostgreSQL 16. A move to a newer major version is never automatic: take a snapshot with `scripts/upgrade.py` (or **Back up now**), start the new major version on an empty volume in a separate Compose project, restore the snapshot there with the same installation key, check a report and an original upload, and only then switch users over and retire the old volume. Keep the old volume until the new one has run through a full backup cycle.

Migrations run transactionally. Older encounters retain their recorded model, policy and history revision. For an application rollback, restore the **pre-upgrade** snapshot into a separate empty Compose project using the matching key, and start its retained pre-upgrade image there. Confirm its report and original records before switching user access. A database created by the new release is intended for the new service. A local evidence collection can be rolled back separately in the operator interface, and its historical citations remain inspectable.

## Regional settings and local readers

Set `CLINICAL_LOCALE` (such as `en-US` or `en-IN`), `CLINICAL_UNITS` (`as-reported`, `metric` or `us-customary`), `CLINICAL_EMERGENCY_CONTACT` and `CLINICAL_CARE_DESTINATION` in the installation environment, then recreate the app. Use reviewed local wording for the actual deployment. Printed laboratory units, dates and reference intervals retain their original text.

The runtime bundles Poppler and English Tesseract for reading reports and whisper.cpp for offline English dictation. It runs as uid 65532 with a read-only root, and uses a temporary, size-limited memory filesystem for document reading. The upload limit is 12 MiB, with up to 24 PDF pages, 320 extracted rows, 1,800 bytes per line and 24 megapixels for an individual photo. Split larger reports into excerpts. Password-protected, invalid or unsupported documents return a recoverable error. Images yielding no legible text can be transcribed against the retained original. Opening an original downloads a private copy. Clinical contents are omitted from ordinary operational logs.
