import { useEffect, useState } from 'react'
import { api } from './api'
import { EvidenceManager } from './evidence'
import { ManageUsage } from './chatgpt'
import type { Work } from './forms'
import type { Region } from './records'
type Configuration = {
  provider: string
  model: string
  status: string
  detail: string
}
type Qualification = {
  configurations: Configuration[]
  budget: Record<string, unknown>
  rates: Record<string, unknown>
  usage: Record<string, unknown>
}
const label = (s: string) =>
  s.replace(/([a-z])([A-Z])/g, '$1 $2').replaceAll('_', ' ')
function Metrics({ items }: { items: Record<string, unknown> }) {
  return (
    <dl className="metric-list">
      {Object.entries(items || {}).map(([name, value]) => (
        <div key={name}>
          <dt>{label(name)}</dt>
          <dd>
            {value == null ? (
              'Unavailable'
            ) : typeof value === 'object' ? (
              <pre>{JSON.stringify(value, null, 2)}</pre>
            ) : (
              String(value)
            )}
          </dd>
        </div>
      ))}
    </dl>
  )
}
export function Operator({
  busy,
  run,
  region,
}: Work & { region: Region | null }) {
  const [qualification, setQualification] = useState<Qualification | null>(
      null,
    ),
    [error, setError] = useState('')
  useEffect(() => {
    void api<Qualification>('/qualification')
      .then(setQualification)
      .catch((err) => setError(err.message))
  }, [])
  return (
    <div className="operator-stack">
      <details className="panel">
        <summary>Model qualification and local usage</summary>
        {error && <p role="alert">{error}</p>}
        {qualification && (
          <>
            <h3>Verified configurations</h3>
            {qualification.configurations?.map((c, i) => (
              <div key={i} className="history-row">
                <strong>
                  {c.provider} · {c.model || 'Configuration'}
                </strong>
                <p>{c.status}</p>
                <p className="metadata">{c.detail}</p>
              </div>
            ))}
            <h3>Request limits</h3>
            <Metrics items={qualification.budget} />
            <h3>Operator-supplied pricing</h3>
            <Metrics items={qualification.rates} />
            <h3>Local usage</h3>
            <Metrics items={qualification.usage} />
            <p>
              ChatGPT plan limits and credits · <ManageUsage />
            </p>
          </>
        )}
      </details>
      <details className="panel">
        <summary>Region and installation operations</summary>
        {region && (
          <dl className="metric-list">
            <div>
              <dt>Dates</dt>
              <dd>{region.locale || 'English default'}</dd>
            </div>
            <div>
              <dt>Units</dt>
              <dd>{region.units || 'As reported'}</dd>
            </div>
            <div>
              <dt>Emergency contact</dt>
              <dd>{region.emergency || 'Generic local-emergency guidance'}</dd>
            </div>
            <div>
              <dt>Care destination</dt>
              <dd>{region.careDestination || 'Unspecified'}</dd>
            </div>
          </dl>
        )}
        <p>
          Configure region and care information on the deployment host. Original
          report measurements retain their printed units.
        </p>
        <h3>Operator tools</h3>
        <p>
          Use the installation’s local recovery, encrypted backup, restore and
          key rotation commands on the host. The operations guide in{' '}
          <code>docs/operations.md</code> explains the required key material and
          checks.
        </p>
      </details>
      <EvidenceManager busy={busy} run={run} />
    </div>
  )
}
