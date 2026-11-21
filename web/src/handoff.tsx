import { detailLine } from './visitPrep'
import { regionalMeasurement, RegionContext } from './region'
import { useContext, useEffect, useState } from 'react'
import { ClipboardList, Download, KeyRound } from 'lucide-react'
import { api } from './api'
import { Callout } from './shared'
import type { Assessment, Encounter, Fact, Fields } from './types'
import { date, human } from './types'
import { factSource } from './records'
import { factDetail } from './report'
import type { Outcome } from './records'
import type { Run } from './forms'
export type Brief = {
  agenda?: string
  changes?: Encounter['answers']
  feedback?: Encounter['feedback']
  format: string
  id: string
  historyRevision: number
  assessmentVersion: number
  confirmed: boolean
  concern: string
  age: number
  updatedAt: string
  notice: string
  reviewStatus: string
  warnings: Encounter['warnings']
  facts: Fact[]
  reportFindings: {
    documentId: string
    rowId: string
    locator: string
    name: string
    value: string
    units: string
    referenceRange: string
    flag: string
    collectionDate: string
    issueDate: string
    quote: string
    note: string
    subject: string
    uncertain: boolean
  }[]
  outstanding: string[]
  contradictions: Encounter['contradictions']
  assessment?: Assessment
  outcomes: Outcome[]
  priorities: string[]
  scopeReason: string
}
function CompactList({ values }: { values: string[] }) {
  return values.length ? (
    <ul>
      {values.map((v, i) => (
        <li key={i}>{v}</li>
      ))}
    </ul>
  ) : (
    <p className="muted">None recorded.</p>
  )
}
export function BriefBody({
  b,
  e,
  name,
}: {
  b: Brief
  e: Encounter
  name: string
}) {
  const region = useContext(RegionContext)
  const facts = (b.facts || []).filter((f) => !f.sourceOutdated),
    reported = facts.filter(
      (f) =>
        f.sourceType !== 'report_entry' &&
        f.sourceType !== 'witness_observation',
    ),
    active = b.warnings?.filter((w) => w.active) || []
  const noticeHeading = b.notice.match(/^[^.]+\.(?:\s|$)/)?.[0] || b.notice
  const discuss = (b.priorities || []).filter(Boolean)
  const timeline = reported
    .filter((f) => f.timeContext || f.origin?.date)
    .map((f) =>
      [f.origin?.date || f.timeContext, f.concept, f.origin?.note || f.quote]
        .filter(Boolean)
        .join(' · '),
    )
  const lists = [
    [
      'Reported positives',
      reported
        .filter((f) => f.status === 'reported_present' && !f.origin)
        .map(
          (f) =>
            (f.subject === 'self' ? '' : human(f.subject) + ' · ') +
            f.concept +
            ' · “' +
            f.quote +
            '”',
        ),
    ],
    [
      'Explicit negatives',
      reported
        .filter((f) => f.status === 'explicitly_denied')
        .map(
          (f) =>
            (f.subject === 'self' ? '' : human(f.subject) + ' · ') +
            f.concept +
            ' · “' +
            f.quote +
            '”',
        ),
    ],
    [
      'Medication and allergy mentions',
      reported
        .filter(
          (f) =>
            f.origin?.category === 'medication' ||
            f.origin?.category === 'allergy' ||
            /medication|medicine|allerg|dose|drug/i.test(
              f.concept + ' ' + f.quote,
            ),
        )
        .map((f) =>
          f.origin
            ? [
                f.concept,
                f.origin.date,
                human(f.origin.reconciliationStatus || 'reviewed'),
                f.origin.note,
              ].join(' · ')
            : f.concept + ' · ' + f.quote,
        ),
    ],
    ['Dated history', timeline],
  ] as [string, string[]][]
  return (
    <article className="report-document brief">
      <div className="report-masthead">
        <span>
          <ClipboardList size={18} />
          {name}
        </span>
        <span>Clinician handoff · History revision {b.historyRevision}</span>
      </div>
      <h1>Pre-visit brief</h1>
      <p className="metadata">
        {date(b.updatedAt, true)} · Age {b.age} ·{' '}
        {b.confirmed ? 'Patient-confirmed history' : 'Unconfirmed history'}
        {e.demo ? ' · Synthetic demo' : ''}
      </p>
      <aside className="brief-notice">
        <p>
          <strong>{noticeHeading}</strong>
          {b.notice.slice(noticeHeading.length)}
        </p>
      </aside>
      {!!active.length && (
        <div className="report-alert-summary">
          <strong>Active urgent warning</strong>
          {[...new Set(active.map((w) => w.message))].map((m) => (
            <p key={m}>{m}</p>
          ))}
        </div>
      )}
      {region?.careDestination && (
        <p>
          <strong>Local care information</strong> · {region.careDestination}
        </p>
      )}
      <h2>Concern and priorities</h2>
      <p>{b.concern}</p>
      {b.agenda && (
        <p>
          <strong>Patient’s main question</strong> · {b.agenda}
        </p>
      )}
      {!!b.changes?.length && (
        <section>
          <h3>What changed and who observed it</h3>
          <CompactList
            values={b.changes.map((a) =>
              [a.question, a.text, detailLine(a)].filter(Boolean).join(' · '),
            )}
          />
        </section>
      )}
      {(!!b.changes?.length || !!b.agenda || e.focused) &&
        !!b.outstanding?.length && (
          <p>
            <strong>First unresolved question</strong> · {b.outstanding[0]}
          </p>
        )}
      {!!b.feedback?.length && (
        <section>
          <h3>Patient-reviewed clinician feedback</h3>
          <CompactList
            values={b.feedback.map(
              (f) =>
                f.speaker +
                ' · ' +
                f.category +
                ' · ' +
                f.text +
                ' · revision ' +
                f.revision,
            )}
          />
        </section>
      )}
      {discuss.length > 0 && <CompactList values={discuss} />}{' '}
      {b.scopeReason && <p>{b.scopeReason}</p>}
      <div className="brief-grid">
        {lists.map(([title, values]) => (
          <section key={title}>
            <h3>{title}</h3>
            <CompactList values={values.slice(0, 4)} />
            {values.length > 4 && (
              <small>
                {values.length - 4} more in the provenance appendix.
              </small>
            )}
          </section>
        ))}
      </div>
      {!!b.reportFindings?.length && (
        <>
          <h3>Reviewed report findings</h3>
          <CompactList
            values={b.reportFindings
              .slice(0, 4)
              .map((r) =>
                [
                  r.subject === 'self'
                    ? 'Personal report'
                    : r.subject === 'family'
                      ? 'Family history'
                      : 'Other subject',
                  r.name,
                  r.value,
                  r.units,
                  r.referenceRange && 'Reference ' + r.referenceRange,
                  regionalMeasurement(r, region),
                  r.flag,
                  r.locator,
                  r.collectionDate,
                  r.note,
                  r.uncertain ? 'Uncertain transcription' : '',
                ]
                  .filter(Boolean)
                  .join(' · '),
              )}
          />
          {b.reportFindings.length > 4 && (
            <small>See the provenance appendix for all reviewed entries.</small>
          )}
        </>
      )}
      <h3>Open questions and conflicts</h3>
      <CompactList
        values={[
          ...(b.outstanding || []),
          ...(b.contradictions || []).map((c) => 'Conflict · ' + c.concept),
        ].slice(0, 5)}
      />
      <h3>Provisional AI hypotheses</h3>
      {b.assessment?.items?.length ? (
        <p>
          {b.assessment.items.map((i, n) => n + 1 + '. ' + i.name).join(' · ')}{' '}
          <small>Assessment {b.assessmentVersion}.</small>
        </p>
      ) : (
        <p>No current ranked estimate is available.</p>
      )}
      {!!b.outcomes?.length && (
        <>
          <h3>Subsequent follow-up</h3>
          <CompactList
            values={b.outcomes
              .slice(-2)
              .map((o) => o.date + ' · ' + human(o.source) + ' · ' + o.text)}
          />
        </>
      )}
    </article>
  )
}
export function Revisions({ e }: { e: Encounter }) {
  const versions = e.assessments
  const [left, setLeft] = useState(versions.at(-2)?.version || 0),
    [right, setRight] = useState(versions.at(-1)?.version || 0)
  const old = versions.find((a) => a.version === left),
    next = versions.find((a) => a.version === right)
  if (versions.length < 2) return null
  const names = (a?: Assessment) => a?.items?.map((i) => i.name) || []
  const before = names(old),
    after = names(next)
  const oldFacts =
      e.summaries?.find((s) => s.historyRevision === old?.historyRevision)
        ?.factIds || [],
    newFacts =
      e.summaries?.find((s) => s.historyRevision === next?.historyRevision)
        ?.factIds || []
  const changes = (from: string[], to: string[]) =>
    to.filter((v) => !from.includes(v))
  const factText = (id: string) => {
    const f = e.facts.find((x) => x.id === id)
    return f ? f.concept + ' · ' + factSource(f) + ' · “' + f.quote + '”' : id
  }
  return (
    <details className="archive">
      <summary>Compare assessment revisions</summary>
      <div className="paired">
        {[
          ['Earlier version', left, setLeft],
          ['Later version', right, setRight],
        ].map(([label, v, set]) => (
          <label key={String(label)}>
            {String(label)}
            <select
              value={Number(v)}
              onChange={(ev) =>
                (set as typeof setLeft)(Number(ev.target.value))
              }
            >
              {versions.map((a) => (
                <option value={a.version} key={a.version}>
                  Assessment {a.version} · History {a.historyRevision}
                </option>
              ))}
            </select>
          </label>
        ))}
      </div>
      {left >= right ? (
        <p>Select an earlier and a later version.</p>
      ) : (
        <>
          <h4>Added possibilities</h4>
          <CompactList values={changes(before, after)} />
          <h4>Removed possibilities</h4>
          <CompactList values={changes(after, before)} />
          <h4>Rank changes</h4>
          <CompactList
            values={after
              .filter(
                (n) =>
                  before.includes(n) && before.indexOf(n) !== after.indexOf(n),
              )
              .map(
                (n) =>
                  n +
                  ' · rank ' +
                  (before.indexOf(n) + 1) +
                  ' to ' +
                  (after.indexOf(n) + 1),
              )}
          />
          <h4>Added or corrected facts</h4>
          <CompactList values={changes(oldFacts, newFacts).map(factText)} />
          <h4>Superseded or removed facts</h4>
          <CompactList values={changes(newFacts, oldFacts).map(factText)} />
          <small>
            Changes are shown side by side. The model’s reason for a change in
            rank can be uncertain.
          </small>
        </>
      )}
    </details>
  )
}
export function AfterVisit({
  e,
  busy,
  act,
}: {
  e: Encounter
  busy: boolean
  act: (cmd: Fields) => Promise<boolean>
}) {
  const [source, setSource] = useState('patient_reported'),
    [when, setWhen] = useState(''),
    [text, setText] = useState(''),
    [document, setDocument] = useState('')
  return (
    <section className="panel">
      <h2>After the visit</h2>
      <p>
        Add a dated outcome or symptom change to this encounter. It stays
        separate from the earlier provisional estimate.
      </p>
      <form
        onSubmit={(ev) => {
          ev.preventDefault()
          void act({
            action: 'add_outcome',
            text,
            date: when,
            source,
            ...(source === 'documented' ? { documentId: document } : {}),
          }).then((ok) => {
            if (ok) {
              setText('')
              setWhen('')
              setDocument('')
            }
          })
        }}
      >
        <div className="paired">
          <label>
            Outcome date
            <input
              required
              type="date"
              value={when}
              onChange={(ev) => setWhen(ev.target.value)}
            />
          </label>
          <label>
            Outcome source
            <select
              value={source}
              onChange={(ev) => setSource(ev.target.value)}
            >
              <option value="patient_reported">My account of the visit</option>
              <option value="documented">A document on this encounter</option>
              <option value="symptom_change">Symptom change</option>
            </select>
          </label>
        </div>
        {source === 'documented' && (
          <label>
            Source document
            <select
              required
              value={document}
              onChange={(ev) => setDocument(ev.target.value)}
            >
              <option value="">Select a matching report</option>
              {(e.documents || [])
                .filter((d) => d.belongsTo === 'self')
                .map((d) => (
                  <option value={d.id} key={d.id}>
                    {d.title}
                  </option>
                ))}
            </select>
          </label>
        )}
        <label>
          What happened?
          <textarea
            required
            maxLength={2000}
            value={text}
            onChange={(ev) => setText(ev.target.value)}
          />
        </label>
        <button className="primary" disabled={busy}>
          Save follow-up
        </button>
      </form>
      {(e.outcomes || []).map((o) => (
        <article key={o.id} className="history-row">
          <h4>
            {o.date} · {human(o.source)}
          </h4>
          <p>{o.text}</p>
          <small>
            {o.documentId &&
              'Source: ' +
                (e.documents?.find((d) => d.id === o.documentId)?.title ||
                  o.documentId) +
                ' · '}
            {o.assessmentVersion
              ? 'Linked assessment ' + o.assessmentVersion
              : 'No assessment at the time'}
          </small>
        </article>
      ))}
    </section>
  )
}
type Grant = {
  active: boolean
  expiresAt?: string
  revision?: number
  token?: string
}
type Bundle = {
  resourceType: string
  type: string
  entry?: {
    resource: {
      resourceType: string
      issue?: { details?: { text: string }; diagnostics?: string }[]
    }
  }[]
}
export function Interop({
  e,
  busy,
  run,
}: {
  e: Encounter
  busy: boolean
  run: Run
}) {
  const base = '/encounters/' + e.id
  const [grant, setGrant] = useState<Grant | null>(null),
    [preview, setPreview] = useState<Bundle | null>(null),
    [json, setJson] = useState<unknown>(null),
    [error, setError] = useState('')
  useEffect(() => {
    setGrant(null)
    setPreview(null)
    setJson(null)
    void api<Grant>(base + '/access')
      .then(setGrant)
      .catch((err) => setError(err.message))
  }, [base, e.revision])
  const issue = async () => {
    const g = await api<Grant>(base + '/access', 'POST', {})
    setGrant({ ...g, active: true })
  }
  const revoked = async () => {
    await api(base + '/access', 'DELETE', {})
    setGrant({ active: false })
  }
  return (
    <section className="panel">
      <h2>Private exports and access</h2>
      <p>
        Preview the data on this installation before downloading or granting
        access.
      </p>
      <div className="actions">
        <button
          className="secondary"
          disabled={busy}
          onClick={() => run(async () => setJson(await api(base + '/export')))}
        >
          Preview complete JSON
        </button>
        <button
          className="secondary"
          disabled={busy}
          onClick={() =>
            run(async () => setPreview(await api<Bundle>(base + '/fhir')))
          }
        >
          Preview FHIR document
        </button>
      </div>
      {json !== null && (
        <details open className="archive">
          <summary>Complete encounter JSON</summary>
          <pre>{JSON.stringify(json, null, 2)}</pre>
        </details>
      )}
      {preview && (
        <div className="preview-box">
          <h3>FHIR R4 document</h3>
          <p>
            {preview.entry?.length || 0} entries · {preview.type}
          </p>
          <h4>Mapping limitations</h4>
          <CompactList
            values={(preview.entry || [])
              .filter((x) => x.resource.resourceType === 'OperationOutcome')
              .flatMap((x) =>
                (x.resource.issue || []).map(
                  (i) =>
                    i.details?.text ||
                    i.diagnostics ||
                    'Unspecified limitation',
                ),
              )}
          />
          <details>
            <summary>Preview bundle</summary>
            <pre>{JSON.stringify(preview, null, 2)}</pre>
          </details>
          <a
            className="secondary"
            href={'/api' + base + '/fhir'}
            download="healthctl-r4.json"
          >
            <Download size={16} />
            Download FHIR R4
          </a>
        </div>
      )}
      <details className="archive">
        <summary>
          <KeyRound size={15} /> Read-only brief access
        </summary>
        <p>
          An approved consumer can read this encounter’s confirmed brief at{' '}
          <code>/api/integration/brief</code> using the bearer token. The grant
          expires in 24 hours and can be revoked here.
        </p>
        {error && <p role="alert">{error}</p>}
        {grant?.active ? (
          <>
            <p>
              <strong>Access granted</strong> · Revision {grant.revision} ·
              Expires{' '}
              {grant.expiresAt
                ? date(grant.expiresAt, true)
                : 'within 24 hours'}
            </p>
            {grant.token && (
              <label>
                One-time token
                <input
                  readOnly
                  value={grant.token}
                  onFocus={(ev) => ev.target.select()}
                />
              </label>
            )}
            <button
              className="secondary"
              disabled={busy}
              onClick={() => run(revoked)}
            >
              Revoke brief access
            </button>
          </>
        ) : (
          <>
            <p>
              {e.confirmedRevision === e.historyRevision
                ? 'This history revision is confirmed.'
                : 'Confirm the current history revision before granting access.'}
            </p>
            <button
              className="secondary"
              disabled={
                busy ||
                e.confirmedRevision !== e.historyRevision ||
                grant === null
              }
              onClick={() => run(issue)}
            >
              Grant access to this brief
            </button>
          </>
        )}
      </details>
    </section>
  )
}
export function Appendix({ e, name }: { e: Encounter; name: string }) {
  return (
    <article className="report-document">
      <div className="report-masthead">
        <span>{name}</span>
        <span>History revision {e.historyRevision}</span>
      </div>
      <h1>Provenance appendix</h1>
      <p>{e.notice}</p>
      {e.warnings.map((w) => (
        <aside key={w.id} className="report-alert-summary">
          <strong>
            {w.active ? 'Active urgent warning' : 'Prior urgent warning'}
          </strong>
          <p>{w.message}</p>
        </aside>
      ))}
      <h2>Answer sources</h2>
      {e.answers.map((a) => (
        <section key={a.id}>
          <h3>{a.question}</h3>
          <p>
            {a.sourceOutdated
              ? 'Earlier answer linked to a superseded report entry'
              : a.active
                ? 'Current reported answer'
                : 'Superseded answer'}{' '}
            · {a.id} · {date(a.createdAt, true)}
          </p>
          <blockquote>{a.text}</blockquote>
        </section>
      ))}
      <h2>Finding sources</h2>
      {e.facts.map((f) => (
        <section key={f.id}>
          <h3>{f.concept}</h3>
          <p>
            {factDetail(f)} · {f.active ? 'Active' : 'Superseded'} · {f.id}
          </p>
          <blockquote>{f.quote}</blockquote>
          {f.origin && (
            <p>
              Earlier encounter {f.origin.encounterId}, finding{' '}
              {f.origin.factId}, revision {f.origin.historyRevision} ·
              Reconciled {f.origin.date} · {f.origin.note}
            </p>
          )}
        </section>
      ))}
      <h2>Reviewed document entries</h2>
      {(e.reportReviews || []).map((review) => {
        const doc = e.documents?.find((d) => d.id === review.documentId)
        return (
          <section key={review.id}>
            <h3>
              {doc?.title} · {review.row.locator}
            </h3>
            <p>
              {review.active
                ? 'Current review'
                : review.withdrawn
                  ? 'Withdrawn'
                  : 'Superseded'}{' '}
              · {review.id}
            </p>
            <h4>Original passage</h4>
            <blockquote>{review.row.original}</blockquote>
            <h4>Reviewed wording</h4>
            <p>{review.row.excerpt || review.row.original}</p>
            <p>
              {[
                review.row.subject === 'family'
                  ? 'Family history'
                  : review.row.subject === 'self'
                    ? 'Personal history'
                    : 'Another person',
                review.row.uncertain && 'Uncertain transcription',
                review.row.name,
                review.row.value,
                review.row.units,
                review.row.referenceRange,
                review.row.flag,
                review.row.collectionDate,
                review.row.issueDate,
                review.row.note,
              ]
                .filter(Boolean)
                .join(' · ')}
            </p>
          </section>
        )
      })}
      <h2>Assessment sources and revisions</h2>
      {e.assessments.map((a) => (
        <section key={a.version}>
          <h3>Assessment {a.version}</h3>
          <p>
            History {a.historyRevision} · {a.provider.provider} ·{' '}
            {a.provider.returnedModel || a.provider.requestedModel} ·{' '}
            {date(a.createdAt, true)}
          </p>
          {a.items.map((item, i) => (
            <div key={i}>
              <strong>{item.name}</strong>
              <p>
                Supporting references:{' '}
                {(item.supporting || []).join(', ') || 'None'}. Conflicting
                references: {(item.conflicting || []).join(', ') || 'None'}.
              </p>
              {(a.literature || [])
                .filter((s) => s.itemIndex === i)
                .map((s, j) => (
                  <div key={j}>
                    <p>{s.reason}</p>
                    {s.passages.map((p) => (
                      <p key={p.id}>
                        {p.publisher} · {p.title}, {p.section} · {p.collection}/
                        {p.version}/{p.id} · {p.status || 'Status unavailable'}{' '}
                        · {p.text}
                      </p>
                    ))}
                  </div>
                ))}
            </div>
          ))}
        </section>
      ))}
    </article>
  )
}
