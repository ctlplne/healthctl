import { useEffect, useState } from 'react'
import { api } from './api'
import { VoiceInput } from './voice'
import { ScreenAlert, type ScreenResult } from './screen'
import type { Encounter, EncounterRow } from './types'


export type DiaryEntry = {
  id: string
  text: string
  onset: string
  end?: string
  approximate: boolean
  recordedAt: string
  warning?: ScreenResult
}
export type ReturnSelection = {
  clarifications: { index: number; words: string }[]
  reviewIds: string[]
  source: string
  revision: number
  facts: { id: string; status: string; note: string }[]
  entries: string[]
  diaryIds: string[]
  consent: boolean
}
export const emptySelection = (): ReturnSelection => ({
  source: '',
  revision: 0,
  facts: [],
  clarifications: [],
  reviewIds: [],
  entries: [],
  diaryIds: [],
  consent: false,
})
function localNow() {
  const d = new Date()
  return new Date(d.valueOf() - d.getTimezoneOffset() * 60000)
    .toISOString()
    .slice(0, 16)
}
export function Diary({ onClose, onVisit }: { onClose: () => void; onVisit?: (id: string) => void }) {
  const [items, setItems] = useState<DiaryEntry[]>([]),
    [text, setText] = useState(''),
    [start, setStart] = useState(localNow),
    [end, setEnd] = useState(''),
    [approx, setApprox] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(''),
    [key, setKey] = useState(() => crypto.randomUUID()),
    [confirm, setConfirm] = useState(''),
    [warning, setWarning] = useState<ScreenResult>()
  useEffect(() => {
    api<DiaryEntry[]>('/diary')
      .then(setItems)
      .catch(() => setError('Could not load earlier episodes.'))
  }, [])
  return (
    <section className="panel form-panel">
      <h2>It happened again</h2>
      <p>
        Save an episode on this installation. You can choose which entries to
        bring to a visit. If you believe this is an emergency, seek local
        emergency care now.
      </p>
      <form
        onSubmit={async (ev) => {
          ev.preventDefault()
          if (busy) return
          setBusy(true)
          setError('')
          try {
            const item = await api<DiaryEntry>('/diary', 'POST', {
              id: key,
              text,
              onset: new Date(start).toISOString(),
              end: end ? new Date(end).toISOString() : '',
              approximate: approx,
            })
            setItems((old) => [item, ...old.filter((x) => x.id !== item.id)])
            setWarning(item.warning)
            setText('')
            setStart(localNow())
            setEnd('')
            setApprox(false)
            setKey(crypto.randomUUID())
          } catch (e) {
            setError(e instanceof Error ? e.message : 'Could not save episode.')
          } finally {
            setBusy(false)
          }
        }}
      >
        <label>
          What happened?
          <textarea
            required
            maxLength={3000}
            value={text}
            onChange={(ev) => setText(ev.target.value)}
          />
        </label>
        <VoiceInput onUse={setText} limit={3000} />
        <label>
          When did it start?
          <input
            required
            type="datetime-local"
            value={start}
            onChange={(ev) => setStart(ev.target.value)}
          />
        </label>
        <label>
          When did it end, if known?
          <input
            type="datetime-local"
            min={start}
            value={end}
            onChange={(ev) => setEnd(ev.target.value)}
          />
        </label>
        <label>
          <input
            type="checkbox"
            checked={approx}
            onChange={(ev) => setApprox(ev.target.checked)}
          />{' '}
          These times are approximate
        </label>
        <div className="actions">
          <button className="primary" disabled={busy}>
            Save episode locally
          </button>
          <button className="secondary" type="button" onClick={onClose}>
            Back to journal
          </button>
        </div>
      </form>
      {error && <p role="alert">{error}</p>}
      {warning && <ScreenAlert warning={warning} />}
      {items.map((x) => (
        <article className="history-row" key={x.id}>
          {x.warning && <p className={'diary-warning ' + x.warning.level}>{x.warning.level === 'check' ? 'Check promptly' : 'Urgent warning'} · {x.warning.message}</p>}
          <p>{x.text}</p>
          <small>
            Started {new Date(x.onset).toLocaleString()}
            {x.approximate ? ' (approximate)' : ''} · Recorded{' '}
            {new Date(x.recordedAt).toLocaleString()}
          </small>
          {onVisit && <button className="secondary" onClick={() => onVisit(x.id)}>Start a visit with this episode</button>}
          <button
            className="text-button"
            onClick={async () => {
              if (confirm !== x.id) {
                setConfirm(x.id)
                return
              }
              try {
                await api('/diary/' + x.id, 'DELETE')
                setItems((old) => old.filter((y) => y.id !== x.id))
                setConfirm('')
              } catch {
                setError('Could not delete the episode.')
              }
            }}
          >
            {confirm === x.id
              ? 'Confirm deletion from diary'
              : 'Delete diary entry'}
          </button>
        </article>
      ))}
    </section>
  )
}

export function ReturnPicker({
  rows,
  value,
  change,
}: {
  rows: EncounterRow[]
  value: ReturnSelection
  change: (
    v: ReturnSelection | ((old: ReturnSelection) => ReturnSelection),
  ) => void
}) {
  const [prior, setPrior] = useState<Encounter>(),
    [diary, setDiary] = useState<DiaryEntry[]>([]),
    [error, setError] = useState('')
  useEffect(() => {
    let active = true
    api<DiaryEntry[]>('/diary')
      .then((v) => {
        if (active) setDiary(v)
      })
      .catch(() => {
        if (active) setError('Could not load the diary.')
      })
    return () => {
      active = false
    }
  }, [])
  useEffect(() => {
    let active = true
    setPrior(undefined)
    if (value.source)
      api<Encounter>('/encounters/' + value.source)
        .then((v) => {
          if (active) {
            setPrior(v)
            change((old) => ({ ...old, revision: v.revision }))
          }
        })
        .catch(() => {
          if (active) setError('Could not load that visit.')
        })
    return () => {
      active = false
    }
  }, [value.source])
  const toggle = (key: 'entries' | 'diaryIds' | 'reviewIds', id: string) =>
    change({
      ...value,
      [key]: value[key].includes(id)
        ? value[key].filter((i) => i !== id)
        : [...value[key], id],
    })
  return (
    <fieldset>
      <legend>History to bring to this visit</legend>
      <label>
        Earlier visit
        <select
          value={value.source}
          onChange={(ev) =>
            change({
              ...emptySelection(),
              source: ev.target.value,
              diaryIds: value.diaryIds,
            })
          }
        >
          <option value="">
            Choose an earlier visit, or use diary entries only
          </option>
          {rows
            .filter((x) => !x.demo)
            .map((r) => (
              <option key={r.id} value={r.id}>
                {r.concern} · {new Date(r.createdAt).toLocaleDateString()}
              </option>
            ))}
        </select>
      </label>
      {prior &&
        (prior.confirmedRevision !== prior.historyRevision ? (
          <p role="alert">
            Confirm this visit’s current history before using it for a return
            visit.
          </p>
        ) : (
          <>
            <p>Recorded at the earlier visit · {prior.concern}</p>
            {prior.warnings
              .filter((w) => w.active)
              .map((w) => (
                <p key={w.id} role="alert">
                  Warning recorded in the earlier visit · {w.message} Answer the
                  current emergency question for how you feel now.
                </p>
              ))}
            {prior.reportReviews?.some((r) => r.active && !r.withdrawn) && (
              <p>
                Select passages to share with the destination shown. A local
                snapshot of each selected original stays available in this
                visit.
              </p>
            )}
            {prior.reportReviews
              ?.filter(
                (r) =>
                  r.active &&
                  !r.withdrawn &&
                  r.row.subject === 'self' &&
                  prior.documents?.some(
                    (d) => d.id === r.documentId && d.belongsTo === 'self',
                  ),
              )
              .map((r) => (
                <label key={r.id}>
                  <input
                    type="checkbox"
                    checked={value.reviewIds.includes(r.id)}
                    onChange={() => toggle('reviewIds', r.id)}
                  />
                  {prior.documents?.find((d) => d.id === r.documentId)?.title} ·{' '}
                  {r.row.locator} · {r.row.excerpt} · {r.row.name} {r.row.value}{' '}
                  {r.row.units} {r.row.referenceRange} {r.row.flag}{' '}
                  {r.row.collectionDate} {r.row.issueDate} {r.row.note}
                </label>
              ))}
            {prior.feedback?.map((f, index) => {
              if (f.revision !== prior.historyRevision) return null
              const entry = value.clarifications.find((x) => x.index === index)
              return (
                <div key={index}>
                  <label>
                    <input
                      type="checkbox"
                      checked={!!entry}
                      onChange={() =>
                        change({
                          ...value,
                          clarifications: entry
                            ? value.clarifications.filter(
                                (x) => x.index !== index,
                              )
                            : [...value.clarifications, { index, words: '' }],
                        })
                      }
                    />
                    Clarify earlier feedback from {f.speaker} · {f.text}
                  </label>
                  {entry && (
                    <label>
                      How would you describe it in your own words?
                      <input
                        required
                        value={entry.words}
                        maxLength={800}
                        onChange={(ev) =>
                          change({
                            ...value,
                            clarifications: value.clarifications.map((x) =>
                              x.index === index
                                ? { index, words: ev.target.value }
                                : x,
                            ),
                          })
                        }
                      />
                    </label>
                  )}
                </div>
              )
            })}
            {prior.facts
              .filter(
                (f) =>
                  f.active &&
                  f.confirmed &&
                  !f.sourceOutdated &&
                  f.sourceType !== 'report_entry' &&
                  f.sourceType !== 'witness_observation' &&
                  !prior.answers.find((a) => a.id === f.answerId)?.reviewIds
                    ?.length,
              )
              .map((f) => {
                const chosen = value.facts.find((i) => i.id === f.id)
                return (
                  <div className="history-row" key={f.id}>
                    <label>
                      {f.concept} · “{f.quote}”
                      <select
                        value={chosen?.status || ''}
                        onChange={(ev) => {
                          const status = ev.target.value
                          change({
                            ...value,
                            facts: [
                              ...value.facts.filter((i) => i.id !== f.id),
                              ...(status
                                ? [
                                    {
                                      id: f.id,
                                      status,
                                      note: chosen?.note || '',
                                    },
                                  ]
                                : []),
                            ],
                          })
                        }}
                      >
                        <option value="">Leave in earlier visit</option>
                        <option value="still_applies">Ongoing</option>
                        <option value="no_longer_applies">Resolved</option>
                        <option value="changed">Changed</option>
                        <option value="uncertain">Uncertain</option>
                      </select>
                    </label>
                    {chosen?.status === 'changed' && (
                      <label>
                        What changed?
                        <input
                          required
                          maxLength={500}
                          value={chosen.note}
                          onChange={(ev) =>
                            change({
                              ...value,
                              facts: value.facts.map((x) =>
                                x.id === f.id
                                  ? { ...x, note: ev.target.value }
                                  : x,
                              ),
                            })
                          }
                        />
                      </label>
                    )}
                  </div>
                )
              })}
            {prior.answers
              .filter(
                (a) =>
                  a.active &&
                  !a.sourceOutdated &&
                  !a.reviewIds?.length &&
                  ['episode_log', 'witness'].includes(a.kind),
              )
              .map((a) => (
                <label key={a.id}>
                  <input
                    type="checkbox"
                    checked={value.entries.includes(a.id)}
                    onChange={() => toggle('entries', a.id)}
                  />
                  {a.question} · {a.text}
                </label>
              ))}
          </>
        ))}
      {diary.map((d) => (
        <label key={d.id}>
          <input
            type="checkbox"
            checked={value.diaryIds.includes(d.id)}
            onChange={() => toggle('diaryIds', d.id)}
          />
          {new Date(d.onset).toLocaleString()} · {d.text}
        </label>
      ))}
      {(value.source || value.diaryIds.length > 0) && (
        <label>
          <input
            type="checkbox"
            required
            checked={value.consent}
            onChange={(ev) => change({ ...value, consent: ev.target.checked })}
          />{' '}
          Include my selections for the model destination shown above. This permission is required to bring these entries into the visit.
        </label>
      )}
      {error && <p role="alert">{error}</p>}
    </fieldset>
  )
}
