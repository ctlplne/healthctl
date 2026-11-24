#!/bin/bash
set -euo pipefail
umask 077
export PATH="/Applications/Docker.app/Contents/Resources/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
resources="$(cd "$(dirname "$0")" && pwd)"
state="${HEALTHCTL_DESKTOP_STATE:-$HOME/Library/Application Support/healthctl}"
mkdir -p "$state"
exec 3<&0
fail() { echo "$*" >&2; exit 1; }
command -v docker >/dev/null || fail "Install Docker Desktop, then reopen healthctl."
if ! docker info >/dev/null 2>&1; then
 /usr/bin/open -gja Docker || fail "Start Docker Desktop, then try again."
 for ((n=0;n<60;n++)); do docker info >/dev/null 2>&1 && break; sleep 2; done
 docker info >/dev/null 2>&1 || fail "Docker Desktop did not start."
fi
# An OS advisory lock is released even if the launcher is interrupted.
if [[ "${HEALTHCTL_DESKTOP_LOCKED:-}" != yes ]]; then
 export HEALTHCTL_DESKTOP_LOCKED=yes
 exec /usr/bin/perl -MFcntl=:flock -e 'open my $lock, ">>", shift or die "Cannot lock workspace\n"; flock($lock, LOCK_EX|LOCK_NB) or die "Another desktop task is running\n"; $^F=fileno($lock); $lock->fcntl(Fcntl::F_SETFD(),0); exec @ARGV or die "Cannot start desktop helper\n"' "$state/operation.lock" "$0" "$@"
fi
read -r appimage dbimage appid dbid < "$resources/images.txt"
export HEALTHCTL_APP_IMAGE="$appimage" HEALTHCTL_DB_IMAGE="$dbimage"
unset CLINICAL_PORT CLINICAL_DATA_KEY POSTGRES_PASSWORD CLINICAL_PUBLIC_ORIGIN OPENAI_API_KEY
# Prebuilt images come first; a source build is only a fallback for bundles
# without them. Prebuilt images must match the IDs recorded at release.
if [[ -f "$resources/images.tar.gz" ]]; then
 docker image inspect "$appimage" "$dbimage" >/dev/null 2>&1 || docker load -i "$resources/images.tar.gz" >/dev/null
 "$resources/verify-images.sh" "$appimage" "$appid" "$dbimage" "$dbid" || fail "The bundled images don't match this release. Reinstall healthctl."
elif [[ -f "$resources/healthctl-source.tar.gz" ]]; then
 if ! docker image inspect "$appimage" >/dev/null 2>&1; then
  docker build --tag "$appimage" - < "$resources/healthctl-source.tar.gz" >&2
 fi
 if ! docker image inspect "$dbimage" >/dev/null 2>&1; then docker pull "$dbimage" >&2; fi
else
 fail "This healthctl bundle has no images."
fi
compose(){ docker compose --env-file "$folder/settings.env" -p "$instance" -f "$resources/compose.yaml" -f "$resources/desktop.yaml" "$@"; }
allocate(){
 folder="$(mktemp -d "$state/healthctl-desktop-XXXXXXXX")"; instance="$(basename "$folder" | tr '[:upper:]' '[:lower:]')"
 port="$(/usr/bin/perl -MIO::Socket::INET -e 'my $s=IO::Socket::INET->new(LocalAddr=>"127.0.0.1",LocalPort=>0,Listen=>1) or die; print $s->sockport')"
 printf 'CLINICAL_PORT=%s\nPOSTGRES_PASSWORD=%s\n' "$port" "$(openssl rand -hex 32)" > "$folder/settings.env"
}
active(){
 [[ -r "$state/active" ]] || fail "Start a workspace first."
 read -r name < "$state/active"
 [[ "$name" =~ ^healthctl-desktop-[[:alnum:]]+$ && -d "$state/$name" ]] || fail "The active workspace cannot be opened."
 folder="$state/$name";instance="$(echo "$name" | tr '[:upper:]' '[:lower:]')"
 port="$(sed -n 's/^CLINICAL_PORT=//p' "$folder/settings.env")"
}
select_workspace(){ basename "$folder" > "$state/active.new"; mv -f "$state/active.new" "$state/active"; }
start(){ compose up -d --no-build --pull never --wait app >&2; }
case "${1:-}" in
 start)
 if [[ -f "$state/active" ]]; then active;start
 else allocate;printf 'CLINICAL_DATA_KEY=%s\n' "$(openssl rand -base64 32)" >> "$folder/settings.env"; start; select_workspace;fi
 printf 'http://127.0.0.1:%s/\n' "$port";;
 platform-key)
 active
 IFS= read -r api_key || fail "Enter a platform API key."
 [[ -z "$api_key" || ( "$api_key" =~ ^sk-[a-zA-Z0-9_-]+$ && ${#api_key} -ge 15 && ${#api_key} -le 515 ) ]] || fail "Check the platform API key."
 pending="$(mktemp "$folder/settings-XXXXXXXX")";trap 'rm -f "$pending"' EXIT
 sed '/^OPENAI_API_KEY=/d' "$folder/settings.env" > "$pending"
 if [[ -n "$api_key" ]];then printf 'OPENAI_API_KEY=%s\n' "$api_key" >> "$pending";fi
 mv "$pending" "$folder/settings.env";start;;
 backup)
 active; target="${2:?Choose a backup filename}"; [[ ! -e "$target" ]] || fail "Choose a new backup filename."
 tmp="$(mktemp "$(dirname "$target")/.healthctl-backup-XXXXXXXX")";trap 'rm -f "$tmp"' EXIT
 { sed -n 's/^CLINICAL_DATA_KEY=//p' "$folder/settings.env"; compose exec -T db pg_dump -Fc -U clinicalhistory -d clinicalhistory --no-owner --no-acl --exclude-table-data=public.sessions --exclude-table-data=public.brief_access --exclude-table-data=public.chatgpt_vault </dev/null; } | "$resources/archive" seal 3<&3 > "$tmp"
 ln "$tmp" "$target" || fail "The backup could not be installed at that location."
 # Let the journal show when the last backup was made. A failure here never undoes the backup.
 compose exec -T db psql -U clinicalhistory -d clinicalhistory -q -c "INSERT INTO backup_status(id,last_attempt,last_success,last_error,destination_kind) VALUES(1,now(),now(),'','desktop_manual') ON CONFLICT (id) DO UPDATE SET last_attempt=now(),last_success=now(),last_error='',destination_kind='desktop_manual'" </dev/null >/dev/null 2>&1 || true;;
 stop)
 active; compose stop >&2;;
 logs)
 active; target="${2:?Choose a log filename}"; [[ ! -e "$target" ]] || fail "Choose a new log filename."
 # Service logs carry events, never clinical text.
 compose logs --no-color --tail 300 app db backup > "$target" 2>/dev/null || fail "The logs could not be saved.";;
 delete)
 active; compose down -v --remove-orphans >&2; rm -rf "$folder"; rm -f "$state/active";;
 uninstall)
 for found in "$state"/healthctl-desktop-*; do
  [[ -d "$found" ]] || continue
  folder="$found"; instance="$(basename "$found" | tr '[:upper:]' '[:lower:]')"
  compose down -v --remove-orphans >&2 || true
 done
 docker image rm "$appimage" "$dbimage" >/dev/null 2>&1 || true
 rm -rf "$state";;
 restore)
 file="${2:?Choose a backup}";[[ -f "$file" ]] || fail "Backup missing."
 previous=""; if [[ -r "$state/active" ]]; then read -r previous < "$state/active" || true; fi
 allocate
 # Restore only into the newly allocated workspace. Failure keeps the previous selection.
 cleanup_stage(){ if [[ "$restored" != yes ]];then compose down -v >/dev/null 2>&1 || true;fi; }
 restored=no
 # Start the DB using a temporary key that will be replaced by the authenticated header.
 printf 'CLINICAL_DATA_KEY=%s\n' "$(openssl rand -base64 32)" >> "$folder/settings.env"
 trap cleanup_stage EXIT
 compose up -d --pull never --wait db >&2
 "$resources/archive" open 3<&3 < "$file" | {
  IFS= read -r key && [[ "$key" =~ ^[A-Za-z0-9+/]{43}=$ ]] || fail "Invalid installation key in backup."
  sed '/^CLINICAL_DATA_KEY=/d' "$folder/settings.env" > "$folder/restore.env";printf 'CLINICAL_DATA_KEY=%s\n' "$key" >> "$folder/restore.env";mv "$folder/restore.env" "$folder/settings.env"
  compose exec -T db pg_restore -U clinicalhistory -d clinicalhistory --exit-on-error --single-transaction --no-owner --no-acl
  cat >/dev/null
 }
 start
 compose run --rm -T --no-deps app check-key </dev/null >&2
 select_workspace;restored=yes
 # The previous workspace stops so only the restored one is running. Its data is kept.
 if [[ "$previous" =~ ^healthctl-desktop-[[:alnum:]]+$ && -d "$state/$previous" ]]; then
  ( folder="$state/$previous"; instance="$(echo "$previous" | tr '[:upper:]' '[:lower:]')"; compose stop >&2 ) || true
 fi
 printf 'http://127.0.0.1:%s/\n' "$port";;
 *) fail "Unknown desktop action.";;
esac
