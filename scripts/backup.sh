#!/bin/sh
# Scheduled PostgreSQL snapshots. Runs by default with docker compose up.
set -eu
# The database password arrives as a Compose secret file.
if [ -n "${PGPASSWORD_FILE:-}" ] && [ -r "$PGPASSWORD_FILE" ] && [ -z "${PGPASSWORD:-}" ]; then PGPASSWORD=$(cat "$PGPASSWORD_FILE"); export PGPASSWORD; fi
umask 077
cd /var/lib/postgresql/data
fail_config() { echo "backup: $1" >&2; exit 2; }
case "${BACKUP_KEEP:-}" in ''|*[!0-9]*) fail_config "BACKUP_KEEP must be a whole number";; esac
case "${BACKUP_INTERVAL_HOURS:-}" in ''|*[!0-9]*) fail_config "BACKUP_INTERVAL_HOURS must be a whole number";; esac
[ "$BACKUP_KEEP" -ge 1 ] && [ "$BACKUP_KEEP" -le 365 ] || fail_config "BACKUP_KEEP must be 1 to 365"
[ "$BACKUP_INTERVAL_HOURS" -ge 1 ] && [ "$BACKUP_INTERVAL_HOURS" -le 8760 ] || fail_config "BACKUP_INTERVAL_HOURS must be 1 to 8760"
# Record the outcome where the app can show it. The values are fixed codes and
# times, never clinical content. A failure to record never stops a backup.
status() {
    kind=host
    case "${BACKUP_DESTINATION:-clinical_backups}" in /*) ;; *) kind=docker_volume;; esac
    # The app creates the status table on start, so retry briefly on a new install.
    for attempt in 1 2 3 4 5 6; do
        record "$1" "$kind" && return 0
        sleep 10
    done
    echo "backup: could not record backup status" >&2
}
record() {
    psql -q -v ON_ERROR_STOP=1 -v outcome="$1" -v kind="$2" >/dev/null 2>&1 <<'SQL'
INSERT INTO backup_status(id, last_attempt, last_success, last_error, destination_kind)
VALUES (1, now(), CASE WHEN :'outcome' = 'ok' THEN now() END, CASE WHEN :'outcome' = 'ok' THEN '' ELSE :'outcome' END, :'kind')
ON CONFLICT (id) DO UPDATE SET last_attempt = now(),
  last_success = CASE WHEN :'outcome' = 'ok' THEN now() ELSE backup_status.last_success END,
  last_error = CASE WHEN :'outcome' = 'ok' THEN '' ELSE :'outcome' END,
  destination_kind = :'kind';
SQL
}
while :; do
    stamp=$(date -u '+%Y%m%dT%H%M%SZ')
    tmp=$(mktemp .clinical-XXXXXX)
    trap 'rm -f "$tmp"' EXIT HUP INT TERM
    if pg_dump -Fc --no-owner --no-acl --exclude-table-data=public.sessions --exclude-table-data=public.brief_access --exclude-table-data=public.chatgpt_vault > "$tmp" && pg_restore --list "$tmp" >/dev/null; then
        mv "$tmp" "clinical-$stamp.dump"
        trap - EXIT HUP INT TERM
        # Only files created by this scheduler have this fixed name format.
        ls -1r clinical-????????T??????Z.dump | tail -n "+$((BACKUP_KEEP + 1))" | while IFS= read -r oldest; do rm -f "./$oldest"; done
        echo "Backup completed $stamp"
        status ok
    else
        rm -f "$tmp"
        trap - EXIT HUP INT TERM
        echo "backup: FAILED at $stamp; the previous snapshots are unchanged" >&2
        status dump_failed
    fi
    [ "${1:-}" != once ] || exit 0
    sleep "$((BACKUP_INTERVAL_HOURS * 3600))"
done
