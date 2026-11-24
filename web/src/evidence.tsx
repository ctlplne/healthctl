import { useEffect, useState } from 'react'
import { BookOpen, Search } from 'lucide-react'
import type { Fact } from './types'
import { api } from './api'
import { Callout, PageTitle } from './shared'
import type { Run } from './forms'

export type Passage = {
  id: string
  collection: string
  version: string
  title: string
  section: string
  text: string
  publisher: string
  url: string
  published: string
  population: string
  limitations: string
  rights: string
  rightsUrl: string
  verified: string | boolean
  reviewStatus: string
  status: string
  conflictsWith: string[]
}
export type SearchResult = {
  relation?: string
  claimLinks?: { passageId: string; quote: string; factIds: string[] }[]
  corpusVersion: string
  outcome: string
  reason: string
  passages: Passage[]
  conflicts: string[]
}
type Collection = {
  id: string
  version: string
  title: string
  origin: string
  status: string
  installedAt: string
  passages: number
}
type Catalog = { collections: Collection[]; corpusVersion: string }
const referencePath = (p: Passage) =>
  '/evidence/' +
  [p.collection, p.version, p.id].map(encodeURIComponent).join('/')
function link(url: string) {
  try {
    const u = new URL(url)
    return u.protocol === 'https:' || u.protocol === 'http:'
      ? u.href
      : undefined
  } catch {
    return undefined
  }
}
export function PassageView({ p }: { p: Passage }) {
  return (
    <div className="citation">
      <h3>{p.title}</h3>
      <p className="metadata">
        {[p.publisher, p.published, p.section].filter(Boolean).join(' · ')}
      </p>
      <blockquote>{p.text}</blockquote>
      <dl className="citation-meta">
        {[
          ['Population', p.population],
          ['Applicability', p.limitations],
          ['Review status', p.reviewStatus],
          ['Record checked', String(p.verified || 'Unspecified')],
          ['Collection', p.collection + ' · ' + p.version],
          ['Availability', p.status],
          ['Reuse rights', p.rights],
        ].map(([k, v]) => (
          <div key={k}>
            <dt>{k}</dt>
            <dd>{v || 'Unspecified'}</dd>
          </div>
        ))}
      </dl>
      {p.conflictsWith?.length > 0 && (
        <p>
          <strong>Related conflicting passages</strong> ·{' '}
          {p.conflictsWith.join(', ')}
        </p>
      )}
      {link(p.url) && (
        <a href={link(p.url)} target="_blank" rel="noreferrer">
          Publisher’s source
        </a>
      )}
      {link(p.rightsUrl) && (
        <>
          {' '}
          ·{' '}
          <a href={link(p.rightsUrl)} target="_blank" rel="noreferrer">
            Reuse terms
          </a>
        </>
      )}
    </div>
  )
}
export function EvidenceLookup({
  busy,
  run,
  initial = '',
  embedded = false,
}: {
  busy: boolean
  run: Run
  initial?: string
  embedded?: boolean
}) {
  const [mode, setMode] = useState('question')
  const [query, setQuery] = useState(initial),
    [result, setResult] = useState<SearchResult | null>(null),
    [opened, setOpened] = useState<Passage | null>(null)
  useEffect(() => {
    setQuery(initial)
    setResult(null)
    setOpened(null)
  }, [initial])
  const search = async () => {
    const r = await api<SearchResult>('/evidence/search', 'POST', {
      query,
      mode,
    })
    setResult(r)
    setOpened(null)
  }
  return (
    <>
      {!embedded && (
        <PageTitle
          title="Medical evidence"
          description="Search this installation’s reference collection."
        />
      )}
      <section className="panel">
        <div className="topic-mark">
          <BookOpen size={17} />
          Local reference collection
        </div>
        <h2>{embedded ? 'Medical source lookup' : 'Find a passage'}</h2>
        <p className="lede">
          Search by a medical question or condition. Results show the source
          passage, who it applies to, and its limitations.
        </p>
        <form
          onSubmit={(ev) => {
            ev.preventDefault()
            run(search)
          }}
        >
          <fieldset>
            <legend>Search mode</legend>
            <div className="radios">
              <label>
                <input
                  type="radio"
                  name="search-mode"
                  checked={mode === 'question'}
                  onChange={() => {
                    setMode('question')
                    setResult(null)
                    setOpened(null)
                  }}
                />
                Medical question
              </label>
              <label>
                <input
                  type="radio"
                  name="search-mode"
                  checked={mode === 'browse'}
                  onChange={() => {
                    setMode('browse')
                    setResult(null)
                    setOpened(null)
                  }}
                />
                Browse passages
              </label>
            </div>
          </fieldset>
          <label>
            {mode === 'question' ? 'Medical question' : 'Search terms'}
            <input
              required
              minLength={2}
              maxLength={500}
              value={query}
              onChange={(ev) => setQuery(ev.target.value)}
            />
          </label>
          <button className="primary" disabled={busy}>
            <Search size={16} />
            Search evidence
          </button>
        </form>
        {result && (
          <div aria-live="polite" className="evidence-results">
            <p className="metadata">
              Collection revision {result.corpusVersion}
            </p>
            {result.outcome === 'unavailable' || !result.passages?.length ? (
              <Callout title="No supporting passage found">
                {result.reason ||
                  'There is insufficient support in this collection for this question.'}
              </Callout>
            ) : (
              <>
                <h3>Matching passages</h3>
                <p>{result.reason}</p>
                {result.conflicts?.length > 0 && (
                  <Callout title="Sources may differ">
                    {result.conflicts.join(' · ')}
                  </Callout>
                )}
                {result.passages.map((p) => (
                  <div
                    className="history-row"
                    key={p.collection + '/' + p.version + '/' + p.id}
                  >
                    <h4>
                      {p.title} · {p.section}
                    </h4>
                    <p>{p.text}</p>
                    <p className="metadata">
                      {p.publisher} · {p.published} · {p.population}
                    </p>
                    <button
                      className="text-button"
                      disabled={busy}
                      onClick={() =>
                        run(async () =>
                          setOpened(await api<Passage>(referencePath(p))),
                        )
                      }
                    >
                      Inspect citation
                    </button>
                  </div>
                ))}
              </>
            )}
          </div>
        )}
        {opened && (
          <details open className="archive">
            <summary>Citation details</summary>
            <PassageView p={opened} />
          </details>
        )}
      </section>
    </>
  )
}
export function EvidenceManager({ busy, run }: { busy: boolean; run: Run }) {
  const [catalog, setCatalog] = useState<Catalog | null>(null),
    [pack, setPack] = useState(''),
    [rights, setRights] = useState(false),
    [reason, setReason] = useState(''),
    [error, setError] = useState('')
  const load = async () => setCatalog(await api<Catalog>('/evidence'))
  useEffect(() => {
    void load().catch((err) => setError(err.message))
  }, [])
  return (
    <details className="panel">
      <summary>Evidence collections and rights</summary>
      {error && <p role="alert">{error}</p>}
      {catalog && (
        <>
          <p className="metadata">Corpus revision {catalog.corpusVersion}</p>
          <label>
            Reason for a collection status change
            <input
              maxLength={500}
              value={reason}
              onChange={(ev) => setReason(ev.target.value)}
            />
          </label>
          {catalog.collections.map((c) => (
            <div className="history-row" key={c.id + c.version}>
              <h4>{c.title}</h4>
              <p>
                {c.id} · {c.version} · {c.status} · {c.passages} passages
              </p>
              <p className="metadata">{c.origin}</p>
              <button
                disabled={busy || !reason.trim()}
                className="secondary"
                onClick={() =>
                  run(async () => {
                    await api(
                      '/evidence/collections/' +
                        encodeURIComponent(c.id) +
                        '/' +
                        encodeURIComponent(c.version) +
                        '/status',
                      'POST',
                      {
                        status: c.status === 'active' ? 'withdrawn' : 'active',
                        reason,
                      },
                    )
                    await load()
                    setReason('')
                  })
                }
              >
                {c.status === 'active'
                  ? 'Withdraw collection version'
                  : 'Activate collection version'}
              </button>
            </div>
          ))}
        </>
      )}
      <form
        onSubmit={(ev) => {
          ev.preventDefault()
          run(async () => {
            let parsed
            try {
              parsed = JSON.parse(pack)
            } catch {
              throw new Error('The pack must be valid JSON.')
            }
            await api('/evidence/packs', 'POST', {
              pack: parsed,
              rightsConfirmed: rights,
            })
            setPack('')
            setRights(false)
            await load()
          })
        }}
      >
        <label>
          Licensed collection pack (JSON)
          <textarea
 autoComplete="off"
            required
            rows={4}
            value={pack}
            onChange={(ev) => setPack(ev.target.value)}
          />
        </label>
        <label className="check">
          <input
            type="checkbox"
            required
            checked={rights}
            onChange={(ev) => setRights(ev.target.checked)}
          />
          I have verified the rights for local use of each included passage.
        </label>
        <button disabled={busy} className="secondary">
          Install collection
        </button>
      </form>
    </details>
  )
}

export function AttachedLiterature({
  result,
  facts,
}: {
  result: SearchResult
  facts: Fact[]
}) {
  const [selected, setSelected] = useState<Passage | null>(null),
    [error, setError] = useState('')
  return (
    <div className="attached-evidence">
      <h4>Medical literature</h4>
      {result.relation && (
        <p className="metadata">
          {(
            {
              supported_context: 'Support for specific context',
              related_reading: 'Related reading',
              unavailable: 'No supporting passage',
            } as Record<string, string>
          )[result.relation] || result.relation}
        </p>
      )}
      {result.claimLinks?.map((c, i) => (
        <div className="claim-link" key={i}>
          <blockquote>{c.quote}</blockquote>
          <small>Passage {c.passageId}</small>
          <ul>
            {c.factIds.map((id) => {
              const f = facts.find((x) => x.id === id)
              return (
                <li key={id}>{f ? f.concept + ' · “' + f.quote + '”' : id}</li>
              )
            })}
          </ul>
        </div>
      ))}
      <p>{result.reason}</p>
      {!result.passages.length && (
        <p>Insufficient support in this local collection.</p>
      )}
      {result.passages.map((p) => (
        <div key={p.id} className="citation-link">
          <strong>{p.title}</strong> · {p.publisher} · {p.published} ·{' '}
          {p.status || 'Availability unknown'}{' '}
          <button
            className="text-button"
            onClick={async () => {
              setError('')
              try {
                setSelected(await api<Passage>(referencePath(p)))
              } catch {
                setSelected({ ...p, status: 'Current availability unknown' })
                setError(
                  'The current catalog entry could not be checked. The saved passage is shown.',
                )
              }
            }}
          >
            Open cited passage
          </button>
        </div>
      ))}
      {error && <p role="alert">{error}</p>}
      {selected && <PassageView p={selected} />}
    </div>
  )
}
