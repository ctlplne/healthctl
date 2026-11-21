# Operating a personal installation

## Access and secrets

The default display name is `healthctl`. The Compose project still uses `clinical-history` so existing installations open the same PostgreSQL volume after an upgrade. Owners can override the display name with `CLINICAL_DISPLAY_NAME`.

One owner account controls one installation. Choose a unique password and store it in a password manager. Passwords use bcrypt at cost 12. Session cookies are HTTP-only, same-site strict, expiring after 12 hours. The database stores session token hashes. Authenticated writes require a session-bound CSRF value and the HTTP server checks request origin and hostname. Sessions, exports and UI responses use no-store cache controls. Login attempts share a persistent budget of ten per ten-minute window on this single-owner installation.

The app encrypts each clinical document with AES-256-GCM and a fresh, library-generated nonce. Associated data binds the owner and encounter IDs. A key verifier detects incorrect restore keys before the service starts. Metadata such as row IDs, timestamps, dataset size, owner username, password hash and nonclinical provider configuration remains visible to the database operator. Use an encrypted host volume and encrypted backups for this metadata, the private environment file, and downloaded reports. Access to the host, its Docker daemon, or both the data and its key gives access to the history. The key also encrypts uploaded originals using owner, encounter and document identifiers as associated data. Use the offline rotation command below to change keys while preserving the database and retry receipts.

Secrets reside on the server. A local `.env` is created with mode 0600 and excluded from the build and repository. A production operator may use the provider adapter's `*_API_KEY_FILE` variables with explicit Docker secret mounts in a private Compose override. Supply one source for each key. Keep state, backups and secrets outside the source tree when distributing software. Backup storage keys should be managed separately from the database archive.

## Locale and TLS

The installation defaults to generic local-emergency wording. To supply verified local contact details, set `CLINICAL_EMERGENCY_CONTACT`. Set `CLINICAL_DISPLAY_NAME` for a personal display name. Recreate the app to apply environment changes. Standard setup supports English-speaking adults. The operator chooses locale information independently of their own travel location.

For an intentional HTTPS deployment, finish owner setup on loopback first. Keep the app bound to loopback, terminate TLS at a reverse proxy on the host, preserve the request Host header, and forward to `127.0.0.1:8437`. Set `CLINICAL_PUBLIC_ORIGIN=https://your-actual-host` and `CLINICAL_SECURE_COOKIE=true`, then recreate the app and log in over HTTPS. Keep PostgreSQL on the private network. Configure the proxy to avoid logging bodies, query strings, cookies and authorization headers; disable its response caching. Proxies, system crash dumps, host snapshots and browser extensions are outside application control.

## Backup

Back up to an owner-controlled, encrypted directory. Set a private umask, choose a new destination filename, and run from the project root:

```sh
umask 077
docker compose exec -T db pg_dump -U clinicalhistory -d clinicalhistory -Fc --no-owner --no-acl --exclude-table-data=public.sessions --exclude-table-data=public.brief_access > clinical-history.dump
```

Keep an independently protected recovery copy of `CLINICAL_DATA_KEY` and the installation configuration. Restore the matching source version first. The database archive contains encrypted clinical rows and other sensitive metadata. Session cookies from the original installation cannot be reused with a session-free restore. A database backup alone cannot decrypt encounters.

## Restore into a fresh installation

Use a separate checkout directory and **a new Compose project name** to get an empty volume. Copy the matching key into the new private `.env` (using the initializer first for a new database password). Keep the old installation and backups intact while verifying the restored copy.

```sh
docker compose -p clinical-history-restore up -d --wait db
docker compose -p clinical-history-restore exec -T db pg_restore -U clinicalhistory -d clinicalhistory --exit-on-error --single-transaction --no-owner --no-acl < clinical-history.dump
CLINICAL_PORT=18438 docker compose -p clinical-history-restore up -d --build
```

Open the restored installation on port 18438, sign in with the backed-up owner credentials and open an encounter and its export. A wrong key causes startup to fail. No browser or cloud action can recover a lost key. Deleting an encounter removes its primary application records. Previous exports, provider-held copies, backups and PostgreSQL recovery history follow their respective retention policies.

## Scheduled backups

From the application directory, enable the local backup worker.

```sh
docker compose --profile backups up -d --wait backup
docker compose logs --tail 10 backup
```

The worker writes a PostgreSQL custom-format snapshot immediately, verifies its archive listing and repeats every `CLINICAL_BACKUP_INTERVAL_HOURS` (default 24). It retains `CLINICAL_BACKUP_KEEP` snapshots (default seven) and limits cleanup to its own dated filenames. Session and brief access tokens are excluded. Defaults use the separate `clinical_backups` named volume. To store snapshots on another encrypted drive, set `CLINICAL_BACKUP_DIRECTORY` to an existing absolute path writable by the PostgreSQL container account. Back up the matching installation key independently. To pause scheduled backups, run `docker compose --profile backups stop backup`.

## Owner recovery and key rotation

These commands require local Docker administration and the installation configuration. Run from the project root. Use an encrypted backup made with the current key before maintenance.

```sh
python3 scripts/maintenance.py recover-owner
python3 scripts/maintenance.py rotate-key
```

Recovery prompts privately for a new password, stops the HTTP service, updates the bcrypt hash, invalidates sessions and external brief grants, and restarts. It preserves the owner and clinical history. To look up the owner username from the local administrator account, run `docker compose exec -T db psql -U clinicalhistory -d clinicalhistory -Atc "SELECT username FROM owners WHERE id=1"`. Rotation verifies and re-encrypts every encounter and original upload in a single database transaction. It preserves idempotency receipts through a wrapped signing key. The script keeps a protected pre-rotation configuration in `.secrets/` for older backups and atomically replaces the active key. If interrupted, rerun the same rotate command. A staged key lets it resume after a database commit. Move pre-rotation configuration copies into controlled backup custody. The database and matching keys are both required for recovery. Use `--project NAME --env-file PATH` for a named local installation. The operating-system wrapper supports macOS and Linux.

## Upgrades and rollback

Make a snapshot and retain the current image before a release upgrade. On the default project:

```sh
docker image tag clinical-history-app:latest clinical-history:before-upgrade
docker compose up -d --build --wait
docker compose ps
```

Migrations run transactionally. Older encounters retain their recorded model, policy and history revision. For an application rollback, restore the **pre-upgrade** snapshot into a separate empty Compose project using the matching key, and start its retained pre-upgrade image there. Confirm its report and original records before switching user access. A database created by the new release is intended for the new service. A local evidence collection can be rolled back separately in the operator interface, and its historical citations remain inspectable.

## Regional settings and local readers

Set `CLINICAL_LOCALE` (such as `en-US` or `en-IN`), `CLINICAL_UNITS` (`as-reported`, `metric` or `us-customary`), `CLINICAL_EMERGENCY_CONTACT` and `CLINICAL_CARE_DESTINATION` in the installation environment, then recreate the app. Use reviewed local wording for the actual deployment. Printed laboratory units, dates and reference intervals retain their original text.

The runtime bundles Poppler and English Tesseract, runs as uid 65532 with a read-only root, and uses a temporary, size-limited memory filesystem for document reading. The upload limit is 12 MiB, with up to 24 PDF pages, 320 extracted rows, 1,800 bytes per line and 24 megapixels for an individual photo. Split larger reports into excerpts. Password-protected, invalid or unsupported documents return a recoverable error. Images yielding no legible text can be transcribed against the retained original. Opening an original downloads a private copy. Clinical contents are omitted from ordinary operational logs.
