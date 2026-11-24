# Operator guide

This guide is for the person who installs and runs healthctl. The [user guide](user-guide.md) covers using the app, and the [verification log](verification-log.md) records what the automated checks show.

## Install and secrets

Run `python3 scripts/init.py` once. It refuses to create a new installation key when data already exists. The database reads its password from a Compose secret file. To keep the installation key and password out of the app and backup containers' configuration as well, run `python3 scripts/secret_files.py` and start with `docker compose -f compose.yaml -f compose.secrets.yaml up -d`. Write the recovery kit with `python3 scripts/recovery_kit.py` and confirm its fingerprint in the app. [Operations reference](operations.md#access-and-secrets)

## Public addresses

Setting `CLINICAL_PUBLIC_ORIGIN` requires HTTPS and secure cookies. The owner account can then be created only with the one-time setup code printed in the server log (`event=setup_code`). Sign-in attempts are counted per client; behind a reverse proxy, set `CLINICAL_TRUSTED_PROXY` to the proxy's address so each client gets its own budget. A connection from the machine itself is never locked out; after ten failures its attempts are slowed and let through one at a time. A tunnel or proxy that reaches the app over loopback (an SSH forward, a sidecar sharing the app's network) shares that one slow queue, so set `CLINICAL_TRUSTED_PROXY` for it too.

## Backups, keys and upgrades

- Scheduled backups go to `CLINICAL_BACKUP_DIRECTORY`, are kept by age (`CLINICAL_BACKUP_KEEP_DAYS`, default 30) with a floor of the newest `CLINICAL_BACKUP_KEEP`, and every failure logs `backup: FAILED`. [Scheduled backups](operations.md#scheduled-backups)
- Records are encrypted under a data key that the installation key wraps. `scripts/maintenance.py rotate-key` replaces the installation key, issues a fresh data key and re-encrypts every record in one transaction, so the old key and older backups' keys no longer read current data. [Key rotation](operations.md#owner-recovery-and-key-rotation)
- `python3 scripts/upgrade.py` snapshots before it changes anything, and `--rollback` restores the snapshot and the previous image. [Upgrades and rollback](operations.md#upgrades-and-rollback)

## Models

Connect a model in **Model settings**. **Test connection** sends a short synthetic interview through the real checks. For OpenAI or Anthropic reasoning models, turn on the reasoning setting so the app sends a reasoning effort by task and streams the answer. Model calls run as durable jobs (`CLINICAL_MODEL_WORKERS`, default 2); a newer submission cancels an older call. The encounter details show each encounter's calls, tokens, waiting time and, when known, cost. [Model providers](model-providers.md)

## Local readers

Document text recognition and dictation run locally with a minimal environment and time, CPU and memory limits. On Linux they also run without network where the system allows a separate network namespace; Docker's default profile doesn't, and the installation safety panel says which applies. Uploads and dictation are rate-limited per owner.

## Settings checked at startup

`CLINICAL_LOCALE` must be English, `CLINICAL_UNITS` one of `as-reported`, `metric` or `us-customary`, and `CLINICAL_EMERGENCY_CONTACT` and `CLINICAL_CARE_DESTINATION` one line of plain text; the contact must include the local emergency number. Every export records the settings in effect.

## Releases

See the [release notes](release.md), the [CI and release process](ci.md) and the [state machine](state-machine.md). Open work that needs a person is listed in [HARDENING.md](../HARDENING.md) under HANDOFF.
