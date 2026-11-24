import { useState, type FormEvent } from 'react'
import { api, RequestError } from './api'

// ReauthDialog asks for the password before export, sharing, deletion or a
// provider change when the last sign-in was more than a few minutes ago.
export function ReauthDialog({ onDone }: { onDone: (ok: boolean) => void }) {
  const [password, setPassword] = useState(''),
    [error, setError] = useState(''),
    [busy, setBusy] = useState(false)
  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    try {
      await api('/reauth', 'POST', { password })
      onDone(true)
    } catch (err) {
      // A wrong password is 401. Anything else (too many attempts, no
      // connection) is reported as the server or the network said it.
      setError(
        err instanceof RequestError && err.status !== 401
          ? err.message
          : 'That password did not match. Try again.',
      )
    } finally {
      setBusy(false)
      setPassword('')
    }
  }
  return (
    <div className="dialog-backdrop">
      <form className="panel reauth" role="dialog" aria-modal="true" aria-labelledby="reauth-heading" onSubmit={submit}>
        <h2 id="reauth-heading">Enter your password to continue</h2>
        <p>Exporting, sharing, deleting and changing where your history is sent need your password again.</p>
        <label>
          Password
          <input type="password" autoComplete="current-password" required autoFocus value={password} onChange={(e) => setPassword(e.target.value)} />
        </label>
        {error && <p role="alert">{error}</p>}
        <div className="actions">
          <button className="primary" disabled={busy}>Continue</button>
          <button type="button" className="secondary" disabled={busy} onClick={() => onDone(false)}>Cancel</button>
        </div>
      </form>
    </div>
  )
}
