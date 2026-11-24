import { detailLine } from './visitPrep'
import { SafetyNet, safetyCheckDue } from './safetynet'
import { Gaps, RedFlags } from './redflags'
import { useState, useEffect } from 'react'
import { Check, Pencil, Quote } from 'lucide-react'
import type { ReactNode } from 'react'
import type { AssessmentNotice, Answer, Encounter, Fact, Fields } from './types'
import { date, human } from './types'
import { BulletList, Evidence, factDetail } from './report'
import { PlanUsage } from './chatgpt'
import { Radios } from './forms'
type EditorProps = {
  busy: boolean
  onAction: (cmd: Fields) => Promise<boolean>
  onCancel: () => void
}
function AnswerEditor({
  a,
  onAction,
  onCancel,
  busy,
}: EditorProps & { a: Answer }) {
  const [text, setText] = useState(a.text),
    [screen, setScreen] = useState('')
  return (
    <form
      className="inline-editor"
      onSubmit={(ev) => {
        ev.preventDefault()
        void onAction(
          a.kind === 'screen'
            ? {
                action: 'correct_answer',
                answerId: a.id,
                urgentNow: screen === 'yes',
              }
            : { action: 'correct_answer', answerId: a.id, text },
        )
      }}
    >
      {a.kind === 'screen' ? (
        <fieldset>
          <legend>Are you reporting a possible emergency right now?</legend>
          <Radios value={screen} name="correct-screen" change={setScreen} />
        </fieldset>
      ) : (
        <label>
          Corrected answer
          <textarea
            required
            rows={3}
            maxLength={4000}
            value={text}
            onChange={(e) => setText(e.target.value)}
          />
        </label>
      )}
      <div className="actions">
        <button disabled={busy} className="primary">
          Save correction
        </button>
        <button type="button" className="secondary" onClick={onCancel}>
          Cancel
        </button>
      </div>
    </form>
  )
}
function FactEditor({
  f,
  onAction,
  onCancel,
  busy,
}: EditorProps & { f: Fact }) {
  const [text, setText] = useState(f.quote),
    [concept, setConcept] = useState(f.concept),
    [status, setStatus] = useState(f.status),
    [subject, setSubject] = useState(f.subject),
    [timing, setTiming] = useState(f.timeContext)
  return (
    <form
      className="inline-editor"
      onSubmit={(ev) => {
        ev.preventDefault()
        void onAction({
          action: 'correct_fact',
          factId: f.id,
          text,
          concept,
          status,
          subject,
          timeContext: timing,
        })
      }}
    >
      <label>
        Corrected wording
        <textarea
          required
          rows={2}
          maxLength={4000}
          value={text}
          onChange={(e) => setText(e.target.value)}
        />
      </label>
      <label>
        Finding
        <input
          required
          maxLength={120}
          value={concept}
          onChange={(e) => setConcept(e.target.value)}
        />
      </label>
      <div className="paired">
        <label>
          Finding status
          <select value={status} onChange={(e) => setStatus(e.target.value)}>
            {[
              'reported_present',
              'explicitly_denied',
              'unknown',
              'not_asked',
              'uncertain',
              'declined',
            ].map((s) => (
              <option key={s} value={s}>
                {human(s)}
              </option>
            ))}
          </select>
        </label>
        <label>
          Whose history?
          <select value={subject} onChange={(e) => setSubject(e.target.value)}>
            <option value="self">My own</option>
            <option value="family">Family history</option>
            <option value="other">Another person</option>
          </select>
        </label>
      </div>
      <label>
        When did this apply?
        <input
          maxLength={180}
          value={timing}
          onChange={(e) => setTiming(e.target.value)}
        />
      </label>
      <div className="actions">
        <button disabled={busy} className="primary">
          Save correction
        </button>
        <button type="button" className="secondary" onClick={onCancel}>
          Cancel
        </button>
      </div>
    </form>
  )
}
function IntakeEditor({
  e,
  busy,
  onAction,
  onCancel,
}: EditorProps & { e: Encounter }) {
  const [age, setAge] = useState(String(e.age)),
    [pregnancy, setPregnancy] = useState(e.pregnancyRelated ? 'yes' : 'no')
  return (
    <form
      className="inline-editor"
      onSubmit={(ev) => {
        ev.preventDefault()
        void onAction({
          action: 'correct_intake',
          age: Number(age),
          pregnancyRelated: pregnancy === 'yes',
        })
      }}
    >
      <label>
        Corrected age in years
        <input
          type="number"
          required
          min={1}
          max={125}
          step={1}
          value={age}
          onChange={(ev) => setAge(ev.target.value)}
        />
      </label>
      <fieldset>
        <legend>Is the concern related to pregnancy?</legend>
        <Radios
          name="correct-pregnancy"
          value={pregnancy}
          change={setPregnancy}
        />
      </fieldset>
      <div className="actions">
        <button className="primary" disabled={busy}>
          Save intake correction
        </button>
        <button type="button" className="secondary" onClick={onCancel}>
          Cancel
        </button>
      </div>
    </form>
  )
}
export function AssessmentRequest({ notice, retry, busy, onAction, onOpenReport }: {
  notice: AssessmentNotice
  retry: boolean
  busy: boolean
  onAction: (cmd: Fields) => Promise<boolean>
  onOpenReport: () => void
}) {
  const [acknowledged, setAcknowledged] = useState(false)
  const [stability, setStability] = useState(false)
  return (
    <div className="assessment-request">
      <h2>Before you request an AI assessment</h2>
      <p>{notice.risks}</p>
      <p>{notice.liability}{' '}
        <a href="/use-notice.html" target="_blank" rel="noreferrer">Read the beta use notice (opens in a new tab)</a>.
      </p>
      <label className="check">
        <input type="checkbox" checked={acknowledged} disabled={busy}
          onChange={(ev) => setAcknowledged(ev.target.checked)} />
        {notice.choice}
      </label>
      {!retry && (
        <label className="check">
          <input type="checkbox" checked={stability} disabled={busy}
            onChange={(ev) => setStability(ev.target.checked)} />
          Run the assessment twice and mark possibilities that change between runs (uses two model calls)
        </label>
      )}
      <div className="actions">
        <button className="primary" disabled={busy || !acknowledged} onClick={() => void onAction({
          action: retry ? 'retry' : 'assess',
          ...(!retry && stability ? { stabilityCheck: true } : {}),
          assessmentAcknowledged: acknowledged,
          assessmentNoticeVersion: notice.version,
        })}>
          {retry ? 'Retry assessment' : 'Generate provisional estimate'}
        </button>
        <button className="secondary" disabled={busy} onClick={onOpenReport}>Open my report instead</button>
      </div>
    </div>
  )
}

export function History({
  assessmentRequest,
  e,
  busy,
  onAction,
  onSource,
  focusFact,
}: {
  e: Encounter
  assessmentRequest: ReactNode
  onSource?: (f: Fact) => void
  focusFact?: string
  busy: boolean
  onAction: (cmd: Fields) => Promise<boolean>
}) {
  const [edit, setEdit] = useState('')
  useEffect(() => {
    if (focusFact) {
      const row = document.getElementById('finding-' + focusFact)
      const archive = row?.closest('details')
      if (archive) archive.open = true
      row?.focus()
      row?.scrollIntoView({ block: 'center' })
    }
  }, [focusFact])
  const action = async (cmd: Fields) => {
    const ok = await onAction(cmd)
    if (ok) setEdit('')
    return ok
  }
  const facts = e.facts.filter((f) => f.active && !f.sourceOutdated)
  const earlierFacts = e.facts.filter((f) => !f.active || f.sourceOutdated)
  const earlierAnswers = e.answers.filter((a) => a.sourceOutdated)
  return (
    <>
      <section className="panel">
        <div className="row-heading">
          <h2>Intake details</h2>
          <button
            className="icon-btn"
            aria-label="Correct intake details"
            onClick={() => setEdit(edit === 'intake' ? '' : 'intake')}
          >
            <Pencil size={16} />
          </button>
        </div>
        <p>
          Age {e.age} · Pregnancy-related concern:{' '}
          {e.pregnancyRelated ? 'Yes' : 'No'}
        </p>
        {edit === 'intake' && (
          <IntakeEditor
            e={e}
            busy={busy}
            onCancel={() => setEdit('')}
            onAction={action}
          />
        )}
      </section>
      <section className="panel">
        <h2>Captured findings</h2>
        <p className="lede">
          Compare each finding with your original words and correct anything
          that was misunderstood.
        </p>
        {facts.length ? (
          facts.map((f) => (
            <div
              className="history-row"
              key={f.id}
              id={'finding-' + f.id}
              tabIndex={-1}
            >
              <div className="row-heading">
                <div>
                  <h3>{f.concept}</h3>
                  <small>
                    {factDetail(f)}
                    {f.corrected ? ' · Corrected' : ''}
                  </small>
                </div>
                {f.sourceType !== 'witness_observation' && (
                  <button
                    className="icon-btn"
                    aria-label={`Correct finding: ${f.concept}`}
                    onClick={() =>
                      f.sourceType === 'report_entry' && onSource
                        ? onSource(f)
                        : setEdit(edit === f.id ? '' : f.id)
                    }
                  >
                    <Pencil size={16} />
                  </button>
                )}
              </div>
              <blockquote>
                <Quote size={14} />
                {f.quote}
              </blockquote>
              {edit === f.id && (
                <FactEditor
                  f={f}
                  busy={busy}
                  onCancel={() => setEdit('')}
                  onAction={action}
                />
              )}
            </div>
          ))
        ) : (
          <p className="muted">
            Your original responses are recorded below. No structured findings
            have been captured yet.
          </p>
        )}
      </section>
      {!!earlierFacts.length && (
        <details className="panel archive">
          <summary>Earlier findings ({earlierFacts.length})</summary>
          {earlierFacts.map((f) => (
            <article
              key={f.id}
              className="history-row"
              id={'finding-' + f.id}
              tabIndex={-1}
            >
              <h3>{f.concept}</h3>
              <p>{factDetail(f)}</p>
              <blockquote>{f.quote}</blockquote>
              <small>
                Earlier history revision {f.historyRevision} · Superseded
              </small>
            </article>
          ))}
        </details>
      )}
      <section className="panel">
        <h2>Reported answers</h2>
        {e.answers
          .filter((a) => a.active && !a.sourceOutdated)
          .map((a) => (
            <div className="history-row" key={a.id}>
              <div className="row-heading">
                <h3>{a.question}</h3>
                {a.detail && <small>{detailLine(a)}</small>}
                {a.kind !== 'witness' && (
                  <button
                    className="icon-btn"
                    aria-label={`Correct answer: ${a.question}`}
                    onClick={() => setEdit(edit === a.id ? '' : a.id)}
                  >
                    <Pencil size={16} />
                  </button>
                )}
              </div>
              <p>{a.text}</p>
              {edit === a.id && (
                <AnswerEditor
                  a={a}
                  busy={busy}
                  onCancel={() => setEdit('')}
                  onAction={action}
                />
              )}
            </div>
          ))}
      </section>
      {!!earlierAnswers.length && (
        <details className="panel archive">
          <summary>Earlier answers linked to superseded report entries</summary>
          {earlierAnswers.map((a) => (
            <article key={a.id} className="history-row">
              <h3>{a.question}</h3>
              {a.detail && <small>{detailLine(a)}</small>}
              <blockquote>{a.text}</blockquote>
              <small>
                Earlier answer linked to a superseded report entry ·{' '}
                {date(a.createdAt, true)}
              </small>
            </article>
          ))}
        </details>
      )}
      {!!e.contradictions?.length && (
        <section className="panel">
          <h2>Conflicting reports</h2>
          {e.contradictions.map((c, i) => (
            <div key={i}>
              <h3>{c.concept}</h3>
              <Evidence ids={c.factIds} facts={e.facts} />
            </div>
          ))}
        </section>
      )}
      {!!e.outstanding?.length && (
        <section className="panel">
          <h2>Outstanding questions</h2>
          <BulletList values={e.outstanding} />
        </section>
      )}
      {!e.scopeReason && !e.pending && (
        <RedFlags
          due={e.redFlagsDue || []}
          busy={busy}
          onAnswer={(key, answer) => void onAction({ action: 'red_flag', category: key, status: answer })}
        />
      )}
      {!e.scopeReason && <Gaps gaps={e.gaps || []} />}
      {!e.scopeReason && (
        <section className="panel confirm-panel">
          {e.provider.kind === 'chatgpt' && <PlanUsage />}
          {e.confirmedRevision !== e.historyRevision && !e.pending && safetyCheckDue(e, 'review') ? (
            <SafetyNet
              point="review"
              busy={busy}
              onAnswer={(answer) => void onAction({ action: 'safety_check', category: 'review', status: answer })}
            />
          ) : e.confirmedRevision !== e.historyRevision ? (
            <>
              <h2>Does this reflect what you reported?</h2>
              <p>
                Confirm this revision when you’re satisfied that the history
                reflects your account.
              </p>
              <button
                className="primary"
                disabled={busy}
                onClick={() => void onAction({ action: 'confirm' })}
              >
                <Check size={17} />
                Confirm my history
              </button>
            </>
          ) : ['history_review', 'needs_clarification', 'assessment_ready'].includes(e.state) && (
            assessmentRequest
          )}
          {e.state === 'history_review' &&
            !e.warnings.some((w) => w.active) && (
              <button
                className="text-button resume"
                disabled={busy}
                onClick={() => void onAction({ action: 'continue' })}
              >
                Continue the interview
              </button>
            )}
        </section>
      )}
    </>
  )
}
