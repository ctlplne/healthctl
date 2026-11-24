import { useEffect, useState } from 'react'
import { CalendarDays, History as HistoryIcon } from 'lucide-react'
import { api } from './api'
import { Callout, PageTitle } from './shared'
import { date, human } from './types'
import type { Encounter, Fact, Fields } from './types'
import { factDetail } from './report'
type Mutate = (c: Fields) => Promise<boolean>
const categories = {
  medication: 'Medication',
  allergy: 'Allergy',
  family_history: 'Family history',
  event: 'Earlier event',
  other: 'Other history',
}
const statusNames = {
  still_applies: 'Still applies',
  changed: 'Changed',
  no_longer_applies: 'No longer applies',
  uncertain: 'Uncertain',
  historical: 'Historical only',
}
function candidateCategory(f: Fact): keyof typeof categories {
  if (f.origin?.category && f.origin.category in categories)
    return f.origin.category as keyof typeof categories
  if (f.subject === 'family') return 'family_history'
  if (
    /medication|medicine|dose|drug|prescription/i.test(
      f.concept + ' ' + f.quote,
    )
  )
    return 'medication'
  if (/allerg/i.test(f.concept + ' ' + f.quote)) return 'allergy'
  return 'other'
}
function ReconcileForm({
  f,
  source,
  busy,
  act,
  onDone,
}: {
  f: Fact
  source: Encounter
  busy: boolean
  act: Mutate
  onDone: () => void
}) {
  const [category, setCategory] = useState<string>(candidateCategory(f)),
    [status, setStatus] = useState(''),
    [when, setWhen] = useState(''),
    [text, setText] = useState('')
  return (
    <form
      className="inline-editor"
      onSubmit={(ev) => {
        ev.preventDefault()
        void act({
          action: 'reuse_fact',
          sourceEncounterId: source.id,
          factId: f.id,
          date: when,
          text,
          category,
          reconciliationStatus: status,
        }).then((ok) => {
          if (ok) onDone()
        })
      }}
    >
      <div className="paired">
        <label>
          History category
          <select
            value={category}
            onChange={(ev) => setCategory(ev.target.value)}
          >
            {Object.entries(categories).map(([k, v]) => (
              <option value={k} key={k}>
                {v}
              </option>
            ))}
          </select>
        </label>
        <label>
          Current status
          <select
            required
            value={status}
            onChange={(ev) => setStatus(ev.target.value)}
          >
            <option value="">Select status</option>
            {Object.entries(statusNames).map(([k, v]) => (
              <option value={k} key={k}>
                {v}
              </option>
            ))}
          </select>
        </label>
      </div>
      <label>
        Reconciliation date
        <input
          type="date"
          required
          value={when}
          onChange={(ev) => setWhen(ev.target.value)}
        />
      </label>
      <label>
        How does this apply now?
        <textarea
 autoComplete="off"
          required
          maxLength={1000}
          value={text}
          onChange={(ev) => setText(ev.target.value)}
        />
      </label>
      <div className="actions">
        <button className="primary" disabled={busy}>
          Add to current history
        </button>
        <button className="secondary" type="button" onClick={onDone}>
          Cancel
        </button>
      </div>
    </form>
  )
}
function EpisodeControls({
  e,
  entries,
  busy,
  act,
}: {
  e: Encounter
  entries: Encounter[]
  busy: boolean
  act: Mutate
}) {
  const [mode, setMode] = useState('name'),
    [title, setTitle] = useState(''),
    [prior, setPrior] = useState('')
  const available = new Map(
    entries
      .filter((v) => v.id !== e.id && v.demo === e.demo)
      .map((v) => [v.episodeId || v.id, v]),
  )
  return (
    <details className="archive">
      <summary>Group this visit into an episode</summary>
      <p>Current episode · {e.episodeTitle || e.concern}</p>
      <form
        onSubmit={(ev) => {
          ev.preventDefault()
          void act(
            mode === 'name'
              ? { action: 'set_episode', episodeTitle: title }
              : { action: 'set_episode', sourceEncounterId: prior },
          ).then((ok) => {
            if (ok) {
              setTitle('')
              setPrior('')
            }
          })
        }}
      >
        <label>
          Episode action
          <select value={mode} onChange={(ev) => setMode(ev.target.value)}>
            <option value="name">Name a new episode</option>
            <option value="join">Join an earlier episode</option>
          </select>
        </label>
        {mode === 'name' ? (
          <label>
            Episode name
            <input
              required
              maxLength={150}
              value={title}
              onChange={(ev) => setTitle(ev.target.value)}
            />
          </label>
        ) : (
          <label>
            Earlier episode
            <select
              required
              value={prior}
              onChange={(ev) => setPrior(ev.target.value)}
            >
              <option value="">Select an episode</option>
              {[...available].map(([k, v]) => (
                <option value={v.id} key={k}>
                  {v.episodeTitle || v.concern} · {date(v.createdAt)}
                </option>
              ))}
            </select>
          </label>
        )}
        <button className="secondary" disabled={busy}>
          Save episode
        </button>
      </form>
    </details>
  )
}
export function Timeline({
  current,
  busy,
  act,
  onOpen,
}: {
  current?: Encounter
  busy: boolean
  act?: Mutate
  onOpen?: (id: string) => void
}) {
  const [entries, setEntries] = useState<Encounter[]>([]),
    [loaded, setLoaded] = useState(false),
    [error, setError] = useState(''),
    [filter, setFilter] = useState(''),
    [category, setCategory] = useState(''),
    [selected, setSelected] = useState('')
  const load = async () => {
    setError('')
    try {
      const r = await api<{ encounters: Encounter[] }>('/timeline')
      setEntries(r.encounters || [])
      setLoaded(true)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Timeline unavailable.')
    }
  }
  useEffect(() => {
    void load()
  }, [current?.revision])
  const matches = (values: (string | undefined)[]) =>
    values
      .filter(Boolean)
      .join(' ')
      .toLowerCase()
      .includes(filter.trim().toLowerCase())
  const episodes = new Map<
    string,
    { title: string; entries: { e: Encounter; facts: Fact[] }[] }
  >()
  for (const e of entries) {
    if (current && (e.id === current.id || e.demo !== current.demo)) continue
    const context = [
      e.concern,
      e.episodeTitle,
      date(e.createdAt),
      e.createdAt.slice(0, 10),
      ...(e.outcomes || []).map((o) => o.date + ' ' + o.text),
      ...(e.documents || []).map((d) => d.title + ' ' + d.purpose),
    ]
    const facts = e.facts.filter(
      (f) =>
        f.active &&
        !f.sourceOutdated &&
        (!category || candidateCategory(f) === category) &&
        matches([
          ...context,
          f.concept,
          f.quote,
          f.timeContext,
          f.origin?.note,
          f.origin?.reconciliationStatus,
        ]),
    )
    if (!facts.length && (category || !matches(context))) continue
    const key = e.episodeId || e.id,
      episode = episodes.get(key) || {
        title: e.episodeTitle || e.concern,
        entries: [],
      }
    episode.entries.push({ e, facts })
    episodes.set(key, episode)
  }
  return (
    <>
      {!current && (
        <PageTitle
          title="Personal timeline"
          description="Find a dated encounter, episode, medication or allergy."
        />
      )}
      <section className="panel">
        <div className="topic-mark">
          <CalendarDays size={17} />
          Longitudinal history
        </div>
        <h2>
          {current ? 'Reconcile earlier history' : 'Encounters over time'}
        </h2>
        {current && act && (
          <>
            <p>
              Select a confirmed earlier finding, decide whether it applies now,
              and record your own dated account.
            </p>
            <EpisodeControls
              e={current}
              entries={entries}
              busy={busy}
              act={act}
            />
          </>
        )}
        <div className="paired">
          <label>
            Search timeline
            <input
              type="search"
              value={filter}
              onChange={(ev) => setFilter(ev.target.value)}
            />
          </label>
          <label>
            Show
            <select
              value={category}
              onChange={(ev) => setCategory(ev.target.value)}
            >
              <option value="">All history</option>
              {Object.entries(categories).map(([k, v]) => (
                <option value={k} key={k}>
                  {v}
                </option>
              ))}
            </select>
          </label>
        </div>
        {error && (
          <Callout alert title="Timeline unavailable">
            {error} <button onClick={() => void load()}>Retry</button>
          </Callout>
        )}
        {!loaded && !error && <p role="status">Loading timeline…</p>}
        {loaded && !episodes.size && (
          <p className="muted">No matching earlier encounters or findings.</p>
        )}
        {[...episodes].map(([key, episode]) => (
          <section key={key} className="episode">
            <h3>{episode.title}</h3>
            <small>
              {episode.entries.length}{' '}
              {episode.entries.length === 1 ? 'encounter' : 'encounters'} in
              this episode
            </small>
            {episode.entries.map(({ e, facts }) => (
              <article className="timeline-entry" key={e.id}>
                <div className="row-heading">
                  <div>
                    <small>
                      {date(e.createdAt)}
                      {e.demo ? ' · Synthetic demo' : ''}
                    </small>
                    <h4>{e.concern}</h4>
                  </div>
                  {onOpen && (
                    <button className="secondary" onClick={() => onOpen(e.id)}>
                      Open encounter
                    </button>
                  )}
                </div>
                {facts.map((f) => {
                  const id = e.id + '-' + f.id
                  return (
                    <div className="history-row" key={f.id}>
                      <h4>{f.concept}</h4>
                      <blockquote>{f.quote}</blockquote>
                      <small>{factDetail(f)}</small>
                      {f.origin && (
                        <p>
                          {f.origin.date} ·{' '}
                          {human(f.origin.reconciliationStatus || 'reviewed')} ·{' '}
                          {f.origin.note}
                        </p>
                      )}
                      {act &&
                        f.confirmed &&
                        f.sourceType !== 'report_entry' && (
                          <button
                            className="text-button"
                            disabled={busy}
                            onClick={() =>
                              setSelected(selected === id ? '' : id)
                            }
                          >
                            <HistoryIcon size={15} />
                            Reconcile this finding
                          </button>
                        )}
                      {selected === id && act && (
                        <ReconcileForm
                          f={f}
                          source={e}
                          busy={busy}
                          act={act}
                          onDone={() => setSelected('')}
                        />
                      )}
                    </div>
                  )
                })}
                {(e.outcomes || []).map((o) => (
                  <p className="timeline-outcome" key={o.id}>
                    {o.date} · {human(o.source)} · {o.text}
                  </p>
                ))}
              </article>
            ))}
          </section>
        ))}
      </section>
    </>
  )
}
