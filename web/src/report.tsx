import { ClipboardList } from 'lucide-react'
import { AttachedLiterature } from './evidence'
import { factSource } from './records'
import { Notice } from './shared'
import { Destination } from './chatgpt'
import { date, human, kinds } from './types'
import type { Assessment, Encounter, Fact, Report } from './types'
export function BulletList({ values }: { values: string[] | null }) {
  return values?.length ? (
    <ul className="bullets">
      {values.map((v, i) => (
        <li key={i}>{v}</li>
      ))}
    </ul>
  ) : null
}
const subjectLabels: Record<string, string> = {
  self: 'Personal history',
  family: 'Family history',
  other: 'Another person',
}
export function factDetail(f: Fact) {
  return [
    factSource(f),
    human(f.status),
    subjectLabels[f.subject] || f.subject,
    f.timeContext,
    f.origin &&
      [
        f.origin.date,
        human(f.origin.reconciliationStatus || 'reconciled'),
        f.origin.note,
      ]
        .filter(Boolean)
        .join(' · '),
    f.value && [f.value, f.units].filter(Boolean).join(' '),
  ]
    .filter(Boolean)
    .join(' · ')
}
export function Evidence({
  ids,
  facts,
  onFact,
}: {
  ids: string[]
  facts: Fact[]
  onFact?: (f: Fact) => void
}) {
  return (
    <ul className="evidence">
      {(ids || []).map((id) => {
        const f = facts.find((x) => x.id === id)
        return f ? (
          <li key={id}>
            <strong>{f.concept}</strong> <span>“{f.quote}”</span>{' '}
            <small>{factDetail(f)}</small>{' '}
            {onFact && (
              <button className="text-button" onClick={() => onFact(f)}>
                View source
              </button>
            )}
          </li>
        ) : null
      })}
    </ul>
  )
}
export function Estimate({
  a,
  facts,
  onFact,
}: {
  a: Assessment
  facts: Fact[]
  onFact?: (f: Fact) => void
}) {
  return (
    <div className="estimate">
      <p className="metadata">
        Assessment {a.version} · History revision {a.historyRevision} ·{' '}
        {date(a.createdAt, true)}
        <br />
        {kinds[a.destination?.kind] || a.provider.provider} ·{' '}
        {a.provider.returnedModel || a.provider.requestedModel}
        {a.destination?.kind === 'chatgpt' && (
          <>
            {' '}
            · <Destination p={a.destination} />
          </>
        )}
      </p>
      {a.items?.length ? (
        <ol className="diagnoses">
          {a.items.map((item, i) => (
            <li key={i}>
              <div>
                <h3>{item.name}</h3>
                {item.clinicalTerm && item.clinicalTerm !== item.name && (
                  <p className="clinical-term">{item.clinicalTerm}</p>
                )}
                <p>{item.explanation}</p>
                {!!item.supporting?.length && (
                  <>
                    <h4>Supporting findings</h4>
                    <Evidence
                      ids={item.supporting}
                      facts={facts}
                      onFact={onFact}
                    />
                  </>
                )}
                {!!item.conflicting?.length && (
                  <>
                    <h4>Conflicting findings</h4>
                    <Evidence
                      ids={item.conflicting}
                      facts={facts}
                      onFact={onFact}
                    />
                  </>
                )}
                {(a.literature || [])
                  .filter((s) => s.itemIndex === i)
                  .map((s, j) => (
                    <AttachedLiterature key={j} result={s} facts={facts} />
                  ))}
                {!!item.limitations?.length && (
                  <>
                    <h4>What remains uncertain</h4>
                    <BulletList values={item.limitations} />
                  </>
                )}
              </div>
            </li>
          ))}
        </ol>
      ) : (
        <h3>More information is needed for a useful estimate</h3>
      )}
      {a.outstanding?.length > 0 && (
        <>
          <h4>Outstanding questions</h4>
          <BulletList values={a.outstanding} />
        </>
      )}
    </div>
  )
}
export function UrgencyReminder({
  warnings,
}: {
  warnings: Encounter['warnings']
}) {
  const messages = [
    ...new Set(warnings.filter((w) => w.active).map((w) => w.message)),
  ]
  if (!messages.length) return null
  return (
    <aside className="report-alert-summary" aria-label="Active urgent warning">
      <strong>Active urgent warning</strong>
      {messages.map((message) => (
        <p key={message}>{message}</p>
      ))}
    </aside>
  )
}
export function ReportBody({
  e,
  name,
}: {
  e: Encounter | Report
  name: string
}) {
  const active = e.facts.filter((f) => f.active && !f.sourceOutdated),
    current = e.assessments
      .filter((a) => a.historyRevision === e.historyRevision)
      .at(-1),
    answers = e.answers.filter((a) => a.active && !a.sourceOutdated)
  return (
    <article className="report-document">
      <div className="report-masthead">
        <span>
          <ClipboardList size={20} />
          {name}
        </span>
        <span>Patient visit report</span>
      </div>
      <h1>Clinical history report</h1>
      <p className="metadata">
        Created {date(e.createdAt, true)} · History revision {e.historyRevision}
        {'exportedAt' in e && (
          <>
            {' '}
            · Exported {date(e.exportedAt, true)} · {e.exportVersion}
          </>
        )}
      </p>
      {e.demo && (
        <p className="demo-report">Synthetic demo · Scripted fictional case</p>
      )}
      <Notice text={e.notice} />
      {!!e.warnings?.length && (
        <section className="report-warnings">
          <h2>Urgency notices shown</h2>
          {e.warnings.map((w) => (
            <div key={w.id}>
              <strong>
                {w.active
                  ? 'Active warning'
                  : 'Earlier warning, since corrected'}
              </strong>
              <p>{w.message}</p>
              <p>
                Reported wording: “{w.quote}” · {date(w.createdAt, true)}
              </p>
            </div>
          ))}
        </section>
      )}
      <section>
        <h2>Chief concern</h2>
        <p className="report-concern">{e.concern}</p>
        <p>
          Age {e.age} ·{' '}
          {e.confirmedRevision === e.historyRevision
            ? 'Patient confirmed this history revision'
            : 'Patient has not confirmed this history revision'}
        </p>
        {e.background && (
          <>
            <h3>Reported background</h3>
            <p>{e.background}</p>
          </>
        )}
        {e.scopeReason && <p>{e.scopeReason}</p>}
      </section>
      <section>
        <h2>Patient-reported history</h2>
        <div className="print-timeline">
          {answers.map((a) => (
            <div key={a.id}>
              <h4>{a.question}</h4>
              <p>{a.text}</p>
              <small>Recorded {date(a.createdAt, true)}</small>
            </div>
          ))}
        </div>
      </section>
      <section>
        <h2>Captured findings</h2>
        {active.length ? (
          active.map((f) => (
            <div key={f.id} className="report-fact">
              <h4>{f.concept}</h4>
              <p>{factDetail(f)}</p>
              <q>{f.quote}</q>
            </div>
          ))
        ) : (
          <p>No structured findings were captured for this revision.</p>
        )}
      </section>
      <section>
        <h2>Unresolved information</h2>
        {e.outstanding?.length ? (
          <BulletList values={e.outstanding} />
        ) : (
          <p>No outstanding questions were recorded.</p>
        )}
        {!!e.contradictions?.length && (
          <>
            <h3>Conflicting reports</h3>
            {e.contradictions.map((c, i) => (
              <div key={i}>
                <strong>{c.concept}</strong>
                <Evidence ids={c.factIds} facts={e.facts} />
              </div>
            ))}
          </>
        )}
      </section>
      <section className="assessment-section">
        <h2>Provisional explanations</h2>
        {current && <UrgencyReminder warnings={e.warnings} />}
        {current && (
          <p className="metadata">
            Experimental AI assessment · Not reviewed by a healthcare
            professional
          </p>
        )}
        {current ? (
          <Estimate a={current} facts={e.facts} />
        ) : (
          <p>
            {e.assessments.length
              ? 'An earlier estimate is outdated. No estimate is available for this history revision.'
              : 'No estimate has been generated for this history revision.'}
          </p>
        )}
        {e.failure && <p>{e.failure}</p>}
      </section>
      <footer>
        Experimental · Not reviewed by a healthcare professional
        <br />
        Encounter provider: {kinds[e.provider.kind]}
        {e.provider.kind === 'chatgpt' && (
          <>
            {' '}
            · <Destination p={e.provider} />
          </>
        )}
        {e.provider.model && ' · ' + e.provider.model}
      </footer>
    </article>
  )
}
