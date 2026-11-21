#!/bin/sh
set -eu
umask 077
cd /var/lib/postgresql/data
case "${BACKUP_KEEP:-}" in ''|*[!0-9]*) exit 2;; esac
case "${BACKUP_INTERVAL_HOURS:-}" in ''|*[!0-9]*) exit 2;; esac
[ "$BACKUP_KEEP" -ge 1 ] && [ "$BACKUP_KEEP" -le 365 ] || exit 2
[ "$BACKUP_INTERVAL_HOURS" -ge 1 ] && [ "$BACKUP_INTERVAL_HOURS" -le 8760 ] || exit 2
while :; do
    stamp=$(date -u '+%Y%m%dT%H%M%SZ')
    tmp=$(mktemp .clinical-XXXXXX)
    trap 'rm -f "$tmp"' EXIT HUP INT TERM
    pg_dump -Fc --no-owner --no-acl --exclude-table-data=public.sessions --exclude-table-data=public.brief_access > "$tmp"
    pg_restore --list "$tmp" >/dev/null
    mv "$tmp" "clinical-$stamp.dump"
    trap - EXIT HUP INT TERM
    # Only files created by this scheduler have this fixed name format.
    ls -1r clinical-????????T??????Z.dump | tail -n "+$((BACKUP_KEEP + 1))" | while IFS= read -r oldest; do rm -f "./$oldest"; done
    echo "Backup completed $stamp"
    [ "${1:-}" != once ] || exit 0
    sleep "$((BACKUP_INTERVAL_HOURS * 3600))"
done
