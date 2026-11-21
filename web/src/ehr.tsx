import { useEffect, useState } from 'react'
import { ArrowLeft, Hospital } from 'lucide-react'
import { api, RequestError } from './api'
import { Brand, Callout } from './shared'
import type { Encounter } from './types'
import type { Work } from './forms'

type Portal = {
  enabled: boolean
  name: string
  fhirBase: string
  authorizationEndpoint: string
}
type Lab = {
  key: string
  label: string
  value: string
  unit: string
  referenceRange: string
  flag: string
  date: string
  issued: string
  source: string
  status: string
  version: string
  panel?: string
  component?: string
  context?: string[]
  absent?: boolean
  critical?: boolean
}
type Preview = {
  ticket: string
  encounterId: string
  name: string
  rows: Lab[]
  skipped: number
  more: boolean
  skippedByReason?: Record<string, number>
  authorizationEndpoint?: string
  fhirBase?: string
}
type Return = {
  code: string
  state: string
  issuer?: string
  denied: boolean
  declined: boolean
}

export function capturePortalReturn(): Return | null {
  if (window.location.pathname !== '/ehr/callback') return null
  const query = new URLSearchParams(window.location.search)
  window.history.replaceState(null, '', '/ehr/callback')
  const value = (name: string) =>
    query.getAll(name).length === 1 ? query.get(name) || '' : ''
  return {
    code: value('code'),
    state: value('state'),
    issuer: value('iss') || undefined,
    denied: query.has('error'),
    declined: value('error') === 'access_denied',
  }
}
export function encounterSourceLink(id: string) {
  return '/#encounter/' + encodeURIComponent(id) + '/sources'
}
export function linkedSourceEncounter() {
  return window.location.hash.match(
    /^#encounter\/([A-Za-z0-9_-]{1,128})\/sources$/,
  )?.[1]
}

export function EhrConnect({ e, run, busy }: { e: Encounter } & Work) {
  const [portal, setPortal] = useState<Portal | null>(null),
    [error, setError] = useState(''),
    [attempt, setAttempt] = useState(0)
  useEffect(() => {
    let active = true
    setError('')
    void api<Portal>('/ehr')
      .then((p) => {
        if (active) setPortal(p)
      })
      .catch(() => {
        if (active)
          setError('The local portal connection settings could not be read.')
      })
    return () => {
      active = false
    }
  }, [attempt])
  const start = async () => {
    if (!portal?.enabled) return
    const { authorizationUrl } = await api<{ authorizationUrl: string }>(
      '/encounters/' + e.id + '/ehr/start',
      'POST',
      {},
    )
    const target = new URL(authorizationUrl),
      configured = new URL(portal.authorizationEndpoint)
    if (
      !['https:', 'http:'].includes(target.protocol) ||
      target.username ||
      target.password ||
      target.origin !== configured.origin ||
      target.pathname !== configured.pathname
    )
      throw new Error(
        'The authorization destination differs from the configured portal. Check the installation settings and retry.',
      )
    window.location.assign(target.href)
  }
  return (
    <section className="panel portal-entry">
      <div className="topic-mark">
        <Hospital size={17} />
        Patient portal
      </div>
      <h2>Import lab results</h2>
      {error ? (
        <p>
          {error}{' '}
          <button
            className="text-button"
            onClick={() => setAttempt((a) => a + 1)}
          >
            Retry portal settings
          </button>
        </p>
      ) : !portal ? (
        <p role="status">Checking this installation’s portal settings…</p>
      ) : !portal.enabled ? (
        <p>
          A patient portal hasn’t been connected to this installation. You can
          upload a report or paste its text.
        </p>
      ) : (
        <>
          <p>
            Sign in at {portal.name} to read lab results. The portal may describe
            this permission as access to observations. This app searches only
            laboratory results. You’ll choose which to attach after you return.
            Sending an extract to your model requires your separate approval.
          </p>
          <dl className="metric-list">
            <div>
              <dt>Portal sign-in</dt>
              <dd>{portal.authorizationEndpoint}</dd>
            </div>
            <div>
              <dt>Health record server</dt>
              <dd>{portal.fhirBase}</dd>
            </div>
          </dl>
          {e.demo && (
            <p className="metadata">
              This is a synthetic demo encounter. Use a test patient record.
            </p>
          )}
          <button
            className="secondary"
            disabled={busy}
            onClick={() => run(start)}
          >
            Connect to patient portal
          </button>
        </>
      )}
    </section>
  )
}
function LabRow({
  r,
  index,
  selected,
  toggle,
}: {
  r: Lab
  index: number
  selected: boolean
  toggle: () => void
}) {
  const fields = [
    ['Panel', r.panel],
    ['Component', r.component],
    ['Result status', r.status],
    ['Effective date or period', r.date],
    ['Issued', r.issued],
    ['Printed reference range', r.referenceRange],
    ['Source flag', r.flag],
    ['Source observation', r.source],
    ['Version', r.version],
  ].filter(([, v]) => v)
  return (
    <div className="portal-row">
      <label className="check">
        <input
          type="checkbox"
          checked={selected}
          onChange={toggle}
          aria-describedby={'portal-result-' + index}
        />
        <span>
          <strong>{r.label || 'Unnamed laboratory result'}</strong>
        </span>
      </label>
      <p id={'portal-result-' + index}>
        {[r.value, r.unit].filter(Boolean).join(' ') ||
          (r.absent ? 'Source reports no result value.' : 'No value supplied.')}
        {r.absent && r.value ? ' · Source data absent' : ''}
      </p>
      <dl className="metric-list">
        {fields.map(([k, v]) => (
          <div key={k}>
            <dt>{k}</dt>
            <dd>{v}</dd>
          </div>
        ))}
      </dl>
      {!!r.context?.length && (
        <ul className="bullets">
          {r.context.map((line, i) => (
            <li key={i}>{line}</li>
          ))}
        </ul>
      )}
    </div>
  )
}
export function PortalReturn({
  callback,
  name,
  onAttach,
  onExit,
}: {
  callback: Return
  name: string
  onAttach: (e: Encounter) => void
  onExit: (id?: string) => void
}) {
  const [preview, setPreview] = useState<Preview | null>(null),
    [failure, setFailure] = useState(''),
    [reconnect, setReconnect] = useState(false),
    [attempt, setAttempt] = useState(0),
    [busy, setBusy] = useState(true),
    [selected, setSelected] = useState<string[]>([]),
    [mine, setMine] = useState(false),
    [expired, setExpired] = useState(false)
  useEffect(() => {
    let active = true
    if (!callback.state || (!callback.code && !callback.denied)) {
      setFailure(
        'This return link is incomplete or has already been used. Reopen your encounter to connect again.',
      )
      setBusy(false)
      return
    }
    setBusy(true)
    setFailure('')
    setReconnect(false)
    const { code, state, issuer, denied } = callback
    void api<Preview>('/ehr/preview', 'POST', { code, state, issuer, denied })
      .then((data) => {
        if (!active) return
        if (!data?.ticket || !data.encounterId) {
          setFailure(
            callback.declined
              ? 'You declined the connection at the portal. Your encounter is still available here.'
              : 'The portal connection ended without an import.',
          )
          return
        }
        setPreview(data)
      })
      .catch((err) => {
        if (active) {
          setFailure(
            callback.declined
              ? 'You declined the connection at the portal. Your encounter is still available here.'
              : err instanceof Error
                ? err.message
                : 'The portal results could not be read.',
          )
          setReconnect(
            !callback.denied && err instanceof RequestError && err.status === 0,
          )
        }
      })
      .finally(() => {
        if (active) setBusy(false)
      })
    return () => {
      active = false
    }
  }, [callback, attempt])
  const finish = async (importing: boolean) => {
    if (
      !preview ||
      busy ||
      (importing && (!selected.length || !mine || expired))
    )
      return
    setBusy(true)
    setFailure('')
    try {
      if (importing) {
        const e = await api<Encounter>('/ehr/import', 'POST', {
          ticket: preview.ticket,
          keys: selected,
          confirmedPatient: true,
        })
        onAttach(e)
      } else {
        await api('/ehr/cancel', 'POST', { ticket: preview.ticket })
        onExit(preview.encounterId)
      }
    } catch (err) {
      setFailure(
        err instanceof Error
          ? err.message
          : 'The request could not be completed.',
      )
      if (
        err instanceof RequestError &&
        [401, 403, 404, 410].includes(err.status)
      )
        setExpired(true)
    } finally {
      setBusy(false)
    }
  }
  const criticalRows =
    preview?.rows?.filter((row) => row.critical === true) || []
  return (
    <div className="auth portal-return">
      <header className="auth-header">
        <Brand name={name} />
      </header>
      <main id="main-content" className="portal-content">
        <div className="topic-mark">
          <Hospital size={17} />
          Patient portal
        </div>
        <h1>{preview ? 'Choose lab results' : 'Return from your portal'}</h1>
        {failure && (
          <Callout
            title={
              callback.declined
                ? 'Connection declined'
                : 'Portal import incomplete'
            }
            alert
          >
            {failure}
          </Callout>
        )}
        {!preview ? (
          <>
            {busy ? (
              <p role="status">Reading available results from your portal…</p>
            ) : (
              <div className="actions">
                {reconnect && (
                  <button
                    className="primary"
                    onClick={() => setAttempt((n) => n + 1)}
                  >
                    Retry preview
                  </button>
                )}
                <button className="secondary" onClick={() => onExit()}>
                  <ArrowLeft size={16} />
                  Return to journal
                </button>
              </div>
            )}
          </>
        ) : (
          <>
            {criticalRows.length > 0 && (
              <Callout title="Critical flag in portal results" alert>
                <p>
                  {preview.name} flagged a result or its panel as critical.
                  Check the source details and any instructions from your care
                  team.
                </p>
                <ul className="bullets">
                  {criticalRows.map((row) => (
                    <li key={row.key}>
                      {[row.panel, row.label].filter(Boolean).join(' · ') ||
                        'Unnamed laboratory result'}
                      {row.flag && ' · Source result flag: ' + row.flag}
                    </li>
                  ))}
                </ul>
              </Callout>
            )}
            <p>
              Results from <strong>{preview.name}</strong>. Choose the entries
              to save in this encounter. You can review the attached
              transcriptions and decide separately which extracts to send to
              your model.
            </p>
            {preview.fhirBase && (
              <p className="metadata">
                Health record server · {preview.fhirBase}
              </p>
            )}
            {preview.more && (
              <p>
                More results may be available in the portal beyond this preview.
              </p>
            )}
            {preview.skipped > 0 && (
              <details>
                <summary>
                  {preview.skipped} source{' '}
                  {preview.skipped === 1 ? 'entry was' : 'entries were'} omitted
                </summary>
                <ul>
                  {Object.entries(preview.skippedByReason || {}).map(
                    ([reason, count]) => (
                      <li key={reason}>
                        {reason.replaceAll('_', ' ')} · {count}
                      </li>
                    ),
                  )}
                </ul>
              </details>
            )}
            <form
              onSubmit={(ev) => {
                ev.preventDefault()
                void finish(true)
              }}
            >
              <fieldset disabled={busy || expired}>
                <legend>Lab results</legend>
                {preview.rows?.length ? (
                  preview.rows.map((r, index) => (
                    <LabRow
                      r={r}
                      index={index}
                      key={r.key}
                      selected={selected.includes(r.key)}
                      toggle={() =>
                        setSelected((keys) =>
                          keys.includes(r.key)
                            ? keys.filter((k) => k !== r.key)
                            : [...keys, r.key],
                        )
                      }
                    />
                  ))
                ) : (
                  <p>This preview contains no eligible lab results.</p>
                )}
                <label className="check portal-owner">
                  <input
                    type="checkbox"
                    checked={mine}
                    onChange={(ev) => setMine(ev.target.checked)}
                  />
                  <span>These results belong to me.</span>
                </label>
              </fieldset>
              <div className="actions">
                <button
                  className="primary"
                  disabled={busy || expired || !mine || !selected.length}
                >
                  Import selected results ({selected.length})
                </button>
                {expired ? (
                  <button
                    className="secondary"
                    type="button"
                    onClick={() => onExit(preview.encounterId)}
                  >
                    Return to encounter
                  </button>
                ) : (
                  <button
                    type="button"
                    className="secondary"
                    disabled={busy}
                    onClick={() => void finish(false)}
                  >
                    Cancel import
                  </button>
                )}
              </div>
              {failure && !expired && (
                <button
                  type="button"
                  className="text-button"
                  onClick={() => onExit(preview.encounterId)}
                >
                  Leave preview and return to encounter
                </button>
              )}
              {busy && <p role="status">Working…</p>}
            </form>
          </>
        )}
      </main>
    </div>
  )
}
