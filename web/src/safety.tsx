import { useEffect, useState } from 'react'
import { api } from './api'

type Safety = {
  recoveryKit: {
    confirmed: boolean
    needsRecheck: boolean
    keyChanged?: boolean
    confirmedAt?: string
    command: string
  }
  backup: {
    recorded: boolean
    lastSuccess?: string
    lastAttempt?: string
    lastError?: string
    destinationKind?: string
  }
}

const day = 24 * 60 * 60 * 1000

// InstallationSafety asks the owner to save a recovery kit and shows whether
// backups are running. Without the installation key nobody can open the
// journal, so the reminder stays until the owner confirms the kit's fingerprint.
export function InstallationSafety() {
  const [safety, setSafety] = useState<Safety>(),
    [code, setCode] = useState(''),
    [error, setError] = useState(''),
    [busy, setBusy] = useState(false)
  useEffect(() => {
    const controller = new AbortController()
    api<Safety>('/installation/safety', 'GET', undefined, { signal: controller.signal })
      .then(setSafety)
      .catch(() => undefined)
    return () => controller.abort()
  }, [])
  if (!safety) return null
  const kit = safety.recoveryKit,
    backup = safety.backup
  const backupAge = backup.lastSuccess ? Date.now() - new Date(backup.lastSuccess).getTime() : Infinity
  const backupProblem = !backup.recorded || !!backup.lastError || backupAge > 3 * day
  if (kit.confirmed && !kit.needsRecheck && !backupProblem) return null
  return (
    <section className="panel installation-safety" aria-labelledby="installation-safety-title">
      <h2 id="installation-safety-title">Protect your journal</h2>
      {(!kit.confirmed || kit.needsRecheck) && (
        <div>
          <p>
            {kit.keyChanged
              ? 'The installation key changed, so your earlier recovery kit only opens older backups. Make a new kit.'
              : kit.needsRecheck
                ? 'It has been a while since you confirmed your recovery kit. Check that you can still find it.'
                : 'Save a recovery kit. Without the installation key, nobody can open this journal if this computer or its Docker data is lost.'}{' '}
            On the computer running healthctl, run <code>{kit.command}</code> from the healthctl folder, move the file somewhere safe, and enter the fingerprint it prints.
          </p>
          <form
            className="inline-form"
            onSubmit={async (ev) => {
              ev.preventDefault()
              if (busy) return
              setBusy(true)
              setError('')
              try {
                setSafety(await api<Safety>('/installation/recovery', 'POST', { fingerprint: code }))
                setCode('')
              } catch (err) {
                setError(err instanceof Error ? err.message : 'Could not confirm the recovery kit.')
              } finally {
                setBusy(false)
              }
            }}
          >
            <label>
              Recovery kit fingerprint
              <input value={code} onChange={(ev) => setCode(ev.target.value)} placeholder="xxxx-xxxx-xxxx" autoComplete="off" spellCheck={false} />
            </label>
            <button className="secondary" disabled={busy || !code.trim()}>
              Confirm recovery kit
            </button>
          </form>
          {error && <p role="alert">{error}</p>}
        </div>
      )}
      {backupProblem && (
        <p className={backup.lastError ? 'form-error' : ''} role={backup.lastError ? 'alert' : undefined}>
          {!backup.recorded
            ? 'No backup has been recorded yet. Scheduled backups start with docker compose up. Desktop installations back up from the launcher.'
            : backup.lastError
              ? 'The last scheduled backup failed. Earlier snapshots are unchanged. Check docker compose logs backup.'
              : `The last backup finished ${Math.floor(backupAge / day)} days ago. Check that the backup worker is running.`}
          {backup.destinationKind === 'docker_volume' && ' Backups are in a Docker volume on this computer. Set CLINICAL_BACKUP_DIRECTORY to a folder on another drive.'}
        </p>
      )}
    </section>
  )
}
