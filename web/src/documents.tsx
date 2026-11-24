import { regionalMeasurement, RegionContext } from './region'
import { useContext, useEffect, useRef, useState } from 'react'
import { FilePlus2, FileSearch, FileText, Send } from 'lucide-react'
import { api, upload as sendUpload } from './api'
import { EhrConnect } from './ehr'
import { Destination } from './chatgpt'
import { Callout } from './shared'
import { kinds } from './types'
import type { Encounter, Fields } from './types'
import { latestReview, rowText } from './records'
import type { DocumentSource, ReportRow } from './records'
import type { Run } from './forms'

type SelectedExtract = Pick<
  ReportRow,
  | 'name'
  | 'value'
  | 'units'
  | 'referenceRange'
  | 'flag'
  | 'collectionDate'
  | 'issueDate'
  | 'subject'
  | 'note'
  | 'uncertain'
  | 'locator'
> & { quote: string; rowId: string }
type Mutate = (cmd: Fields) => Promise<boolean>
type Props = {
  e: Encounter
  busy: boolean
  run: Run
  act: Mutate
  refresh: (e: Encounter) => void
  highlight?: { documentId: string; rowId: string; reviewId?: string }
  onInterview: () => void
}
export function SourceReview({
  e,
  busy,
  run,
  act,
  refresh,
  highlight,
  onInterview,
}: Props) {
  const docs = e.documents || []
  const [showUpload, setShowUpload] = useState(docs.length === 0),
    [inputMode, setInputMode] = useState('file'),
    [labelPhoto, setLabelPhoto] = useState(false),
    [file, setFile] = useState<File | null>(null),
    [paste, setPaste] = useState(''),
    [belongs, setBelongs] = useState('unknown'),
    [purpose, setPurpose] = useState(''),
    [priority, setPriority] = useState(''),
    [issue, setIssue] = useState('')
  const [selectedDoc, setSelectedDoc] = useState(
    highlight?.documentId || docs.at(-1)?.id || '',
  )
  const [submission, setSubmission] = useState(() => crypto.randomUUID())
  const [progress, setProgress] = useState<number>()
  const uploading = useRef<AbortController | null>(null)

  const doc = docs.find((d) => d.id === selectedDoc) || docs.at(-1)
  const upload = async () => {
    const item =
      inputMode === 'paste'
        ? new File([paste], 'pasted-report.txt', { type: 'text/plain' })
        : file
    if (!item) return
    const data = new FormData()
    data.set('medicationLabel', String(labelPhoto))
    data.append('file', item)
    data.append('revision', String(e.revision))
    data.append('requestId', submission)
    data.append('belongsTo', belongs)
    data.append('purpose', purpose)
    data.append('priority', priority)
    data.append('issueDate', issue)
    uploading.current = new AbortController()
    setProgress(0)
    let updated: Encounter
    try {
      updated = await sendUpload<Encounter>('/encounters/' + e.id + '/documents', data, {
        onProgress: setProgress,
        signal: uploading.current.signal,
      })
    } finally {
      uploading.current = null
      setProgress(undefined)
    }
    refresh(updated)
    setSelectedDoc(updated.documents?.at(-1)?.id || '')
    setShowUpload(false)
    setFile(null)
    setPaste('')
    setPurpose('')
    setPriority('')
    setIssue('')
    setBelongs('unknown')
    setSubmission(crypto.randomUUID())
  }
  return (
    <div className="source-workspace">
      <section className="panel">
        <div className="row-heading">
          <div>
            <div className="topic-mark">
              <FileText size={18} />
              Source documents
            </div>
            <h2>Medical reports</h2>
          </div>
          {!showUpload && (
            <button
              className="secondary"
              disabled={busy}
              onClick={() => setShowUpload(true)}
            >
              <FilePlus2 size={17} />
              Add report
            </button>
          )}
        </div>
        {progress !== undefined && (
          <div className="upload-progress" role="status">
            <progress value={progress} max={1} aria-label="Upload progress" /> {Math.round(progress * 100)}%{' '}
            <button type="button" className="secondary" onClick={() => uploading.current?.abort()}>
              Cancel upload
            </button>
          </div>
        )}
        {showUpload && (
          <form
            onChange={() => setSubmission(crypto.randomUUID())}
            onSubmit={(ev) => {
              ev.preventDefault()
              run(upload)
            }}
          >
            <p className="lede">
              Upload a written report or paste a passage. The file and text
              recognition stay on this installation. You decide which reviewed
              entries the model receives.
            </p>
            <fieldset>
              <legend>Report input</legend>
              <div className="radios">
                <label>
                  <input
                    type="radio"
                    name="report-input"
                    checked={inputMode === 'file'}
                    onChange={() => setInputMode('file')}
                  />
                  File or photo
                </label>
                <label>
                  <input
                    type="radio"
                    name="report-input"
                    checked={inputMode === 'paste'}
                    onChange={() => setInputMode('paste')}
                  />
                  Paste text
                </label>
              </div>
            </fieldset>
            {inputMode === 'file' && labelPhoto && (
              <label>
                Take a new photo of the medicine label
                <input
                  type="file"
                  accept="image/jpeg,image/png"
                  capture="environment"
                  onChange={(ev) => setFile(ev.target.files?.[0] || null)}
                />
              </label>
            )}
            {inputMode === 'file' ? (
              <label>
                <span id="source-file-label">Report file</span>
                <input
                  aria-labelledby="source-file-label"
                  aria-describedby="source-file-hint"
                  type="file"
                  required={!file}
                  accept=".pdf,.docx,.txt,.png,.jpg,.jpeg"
                  onChange={(ev) => setFile(ev.target.files?.[0] || null)}
                />
                <small id="source-file-hint">
                  PDF, DOCX, plain text, or a PNG/JPEG photo of a printed page.
                </small>
              </label>
            ) : (
              <label>
                Report text
                <textarea
                  required
                  maxLength={250000}
                  rows={6}
                  value={paste}
                  onChange={(ev) => setPaste(ev.target.value)}
                  placeholder="Paste the original lines, including units and reference ranges."
                />
              </label>
            )}
            <label>
              <input
                type="checkbox"
                checked={labelPhoto}
                onChange={(ev) => setLabelPhoto(ev.target.checked)}
              />{' '}
              This is a medicine box, bottle or blister pack
            </label>
            {labelPhoto && (
              <p>
                Review the printed name, strength and units beside the original.
                Record actual use in Visit preparation after authorizing a row.
              </p>
            )}
            <label>
              Whose report is this?
              <select
                value={belongs}
                onChange={(ev) => setBelongs(ev.target.value)}
              >
                <option value="unknown">I’m not sure</option>
                <option value="self">Mine</option>
                <option value="other">Someone else’s</option>
              </select>
            </label>
            <label>
              What prompted this test or visit?{' '}
              <span className="label-note">If known</span>
              <input
                maxLength={500}
                value={purpose}
                onChange={(ev) => setPurpose(ev.target.value)}
                placeholder="For example, routine screening"
              />
            </label>
            <label>
              What would you like to understand?{' '}
              <span className="label-note">Optional</span>
              <input
                maxLength={500}
                value={priority}
                onChange={(ev) => setPriority(ev.target.value)}
              />
            </label>
            <label>
              Printed issue date <span className="label-note">Optional</span>
              <input
                type="date"
                value={issue}
                onChange={(ev) => setIssue(ev.target.value)}
              />
            </label>
            <div className="actions">
              <button className="primary" disabled={busy}>
                <FileSearch size={17} />
                Read report locally
              </button>

              {docs.length > 0 && (
                <button
                  type="button"
                  className="secondary"
                  onClick={() => setShowUpload(false)}
                >
                  Cancel
                </button>
              )}
            </div>
          </form>
        )}
        {docs.length > 0 && (
          <div className="source-select">
            <label>
              Saved report
              <select
                value={doc?.id || ''}
                onChange={(ev) => setSelectedDoc(ev.target.value)}
              >
                {docs.map((d) => (
                  <option value={d.id} key={d.id}>
                    {d.title || 'Report'} · {d.rows?.length || 0} source
                    passages
                  </option>
                ))}
              </select>
            </label>
          </div>
        )}
      </section>
      <EhrConnect e={e} busy={busy} run={run} />
      {doc?.priorEncounter && (
        <p>
          Snapshot from earlier visit · history revision {doc.priorRevision}
        </p>
      )}
      {doc && (
        <DocumentReview
          key={doc.id}
          e={e}
          doc={doc}
          act={act}
          busy={busy}
          run={run}
          highlight={highlight?.documentId === doc.id ? highlight.rowId : ''}
          requestedReview={
            highlight?.documentId === doc.id ? highlight.reviewId : undefined
          }
          onInterview={onInterview}
        />
      )}
    </div>
  )
}
const rowFields = [
  ['name', 'Printed name or finding'],
  ['value', 'Result as printed'],
  ['units', 'Printed units'],
  ['referenceRange', 'Printed reference range'],
  ['flag', 'Printed flag'],
  ['collectionDate', 'Specimen collection date'],
  ['issueDate', 'Report issue date'],
  ['note', 'Correction or disagreement'],
] as const
function RowEditor({
  row,
  busy,
  save,
  cancel,
}: {
  row: ReportRow
  busy: boolean
  save: (data: Fields) => void
  cancel: () => void
}) {
  const [draft, setDraft] = useState({ ...row })
  return (
    <form
      className="inline-editor"
      onSubmit={(ev) => {
        ev.preventDefault()
        const editable: Fields = {
          subject: draft.subject || 'self',
          uncertain: Boolean(draft.uncertain),
          excerpt: draft.excerpt ?? row.original,
        }
        for (const [key] of rowFields) editable[key] = draft[key] || ''
        save(editable)
      }}
    >
      <label>
        Exact passage for this entry
        <textarea
          required
          rows={2}
          maxLength={4000}
          value={draft.excerpt ?? row.original}
          onChange={(ev) => setDraft({ ...draft, excerpt: ev.target.value })}
        />
        <small>
          Use a continuous excerpt of the original line. Exclude identifying
          headers.
        </small>
      </label>
      <div className="paired">
        {rowFields.map(([key, label]) => (
          <label key={key}>
            {label}
            <input
              required={key === 'name'}
              value={draft[key] || ''}
              maxLength={key === 'note' ? 750 : 240}
              onChange={(ev) => setDraft({ ...draft, [key]: ev.target.value })}
            />
          </label>
        ))}
      </div>
      <label>
        Who does the finding describe?
        <select
          value={draft.subject || 'self'}
          onChange={(ev) => setDraft({ ...draft, subject: ev.target.value })}
        >
          <option value="self">Me</option>
          <option value="family">A family member</option>
          <option value="other">Another person</option>
        </select>
      </label>
      <label className="check">
        <input
          type="checkbox"
          checked={Boolean(draft.uncertain)}
          onChange={(ev) =>
            setDraft({ ...draft, uncertain: ev.target.checked })
          }
        />
        I’m unsure of this transcription or its alignment.
      </label>
      <div className="actions">
        <button className="primary" disabled={busy}>
          Confirm transcription
        </button>
        <button type="button" className="secondary" onClick={cancel}>
          Cancel
        </button>
      </div>
    </form>
  )
}
function OriginalPage({
  url,
  kind,
  title,
  locator,
}: {
  url: string
  kind: string
  title: string
  locator?: string
}) {
  const first = Number(locator?.match(/\bpage\s+(\d{1,2})\b/i)?.[1]) || 1
  const [page, setPage] = useState(first),
    [input, setInput] = useState(String(first)),
    [enlarged, setEnlarged] = useState(false),
    [failed, setFailed] = useState(false)
  const src = '/api' + url + '/page/' + page
  return (
    <div className="original-page">
      <h4>Original page {page}</h4>
      {kind === 'pdf' && (
        <form
          onSubmit={(ev) => {
            ev.preventDefault()
            setPage(Number(input))
            setFailed(false)
          }}
        >
          <label>
            Page number
            <input
              type="number"
              min={1}
              max={24}
              step={1}
              required
              value={input}
              onChange={(ev) => setInput(ev.target.value)}
            />
          </label>
          <button className="secondary">Show page</button>
        </form>
      )}
      <div className="actions">
        <button
          type="button"
          className="text-button"
          aria-pressed={enlarged}
          onClick={() => setEnlarged(!enlarged)}
        >
          {enlarged ? 'Fit page to width' : 'Enlarge page'}
        </button>
        <a className="text-button" href={src} target="_blank" rel="noreferrer">
          Open full-size page
        </a>
      </div>
      {failed ? (
        <p role="alert">
          This page could not be displayed. Check the page number or download
          the original.
        </p>
      ) : (
        <div
          className="page-raster"
          role="region"
          aria-label="Original page image"
          tabIndex={0}
        >
          <img
            key={page}
            className={enlarged ? 'enlarged' : ''}
            src={src}
            alt={title + ' · original page ' + page}
            onError={() => setFailed(true)}
          />
        </div>
      )}
    </div>
  )
}
function DocumentReview({
  e,
  doc,
  act,
  busy,
  run,
  highlight,
  requestedReview,
  onInterview,
}: {
  e: Encounter
  doc: DocumentSource
  act: Mutate
  busy: boolean
  run: Run
  highlight?: string
  requestedReview?: string
  onInterview: () => void
}) {
  const region = useContext(RegionContext)
  const hasPage = ['pdf', 'jpeg', 'jpg', 'png'].includes(doc.kind)
  const [viewing, setViewing] = useState('')
  const [edit, setEdit] = useState(''),
    [ids, setIds] = useState<string[]>([]),
    [preview, setPreview] = useState<SelectedExtract[] | null>(null),
    [consent, setConsent] = useState(false),
    [manual, setManual] = useState(false),
    [passage, setPassage] = useState(''),
    [locator, setLocator] = useState(''),
    [filter, setFilter] = useState(''),
    [page, setPage] = useState(0)
  const rowRefs = useRef(new Map<string, HTMLElement>())
  const docRows = doc.rows || []
  const rows = docRows.filter((r) =>
    (r.original + ' ' + r.locator + ' ' + r.name)
      .toLowerCase()
      .includes(filter.toLowerCase()),
  )
  const shown = rows.slice(page * 12, (page + 1) * 12)
  useEffect(() => {
    setIds([])
    setPreview(null)
    setConsent(false)
    setEdit('')
  }, [e.revision, doc.id])
  useEffect(() => {
    if (!highlight) return
    setFilter('')
    const pos = docRows.findIndex((r) => r.id === highlight)
    if (pos >= 0) setPage(Math.floor(pos / 12))
  }, [highlight, doc.id])
  useEffect(() => {
    if (highlight) {
      const row = rowRefs.current.get(highlight)
      if (row) {
        row.focus()
        row.scrollIntoView({ block: 'center' })
      }
    }
  }, [page, highlight, doc.id])
  const changeIds = (next: string[]) => {
    setIds(next)
    setPreview(null)
    setConsent(false)
  }
  const url = '/encounters/' + e.id + '/documents/' + doc.id
  const readPreview = async () => {
    const result = await api<{ extracts: SelectedExtract[] }>(
      url + '/preview',
      'POST',
      { rowIds: ids },
    )
    setPreview(result.extracts || [])
    setConsent(false)
  }
  return (
    <>
      <section className="panel">
        <div className="row-heading">
          <h2>{doc.title || 'Source report'}</h2>
          <a className="text-button" href={'/api' + url + '/original'} download>
            Download original
          </a>
        </div>
        <p className="metadata">
          Original retained locally · {doc.kind}{' '}
          {doc.issueDate && ' · Issued ' + doc.issueDate}
        </p>
        {hasPage && (
          <>
            <button
              type="button"
              className="secondary"
              aria-expanded={viewing === 'document'}
              onClick={() =>
                setViewing(viewing === 'document' ? '' : 'document')
              }
            >
              {viewing === 'document'
                ? 'Close page viewer'
                : 'Browse original pages'}
            </button>
            {viewing === 'document' && (
              <OriginalPage url={url} kind={doc.kind} title={doc.title} />
            )}
          </>
        )}
        {doc.purpose && (
          <p>
            <strong>Reason for the test</strong> · {doc.purpose}
          </p>
        )}
        {doc.priority && (
          <p>
            <strong>Your priority</strong> · {doc.priority}
          </p>
        )}
        <label className="source-select">
          Report belongs to
          <select
            value={doc.belongsTo}
            disabled={busy}
            onChange={(ev) =>
              void act({
                action: 'report_identity',
                documentId: doc.id,
                belongsTo: ev.target.value,
              })
            }
          >
            <option value="self">Me</option>
            <option value="other">Someone else</option>
            <option value="unknown">Identity uncertain</option>
          </select>
        </label>
        {doc.belongsTo !== 'self' && (
          <Callout title="Check the report identity">
            Entries can be reviewed for use only after you confirm that the
            original report belongs to you.
          </Callout>
        )}
        <h3 className="subheading">Original passages and transcriptions</h3>
        <p className="lede">
          Check the printed digits, dates, units, and row alignment. A confirmed
          transcription records the report’s wording. Sharing is a separate step
          below.
        </p>
        {rows.length > 12 || filter ? (
          <label>
            Filter source passages
            <input
              value={filter}
              onChange={(ev) => {
                setPage(0)
                setFilter(ev.target.value)
              }}
            />
          </label>
        ) : null}
        {shown.length === 0 && (
          <p className="muted">
            No matching passage. Add a transcription with its page or line
            reference.
          </p>
        )}
        {shown.map((row) => {
          const review = latestReview(e, doc.id, row.id)
          const current = review?.active && !review.withdrawn ? review : null
          const prior = (e.reportReviews || []).filter(
            (r) => r.documentId === doc.id && r.rowId === row.id && !r.active,
          )
          const secondary = current
            ? regionalMeasurement(current.row, region)
            : ''
          return (
            <article
              key={row.id}
              className="passage-card"
              tabIndex={-1}
              ref={(el) => {
                if (el) rowRefs.current.set(row.id, el)
                else rowRefs.current.delete(row.id)
              }}
            >
              <div className="row-heading">
                <strong>{row.locator || 'Source passage'}</strong>
                <span className="tag">
                  {current ? (
                    current.approvedFor ? (
                      <>
                        Approved for {kinds[current.approvedFor.kind]}
                        {current.approvedFor.kind === 'chatgpt' && (
                          <>
                            {' '}
                            · <Destination p={current.approvedFor} />
                          </>
                        )}
                      </>
                    ) : (
                      'Reviewed locally'
                    )
                  ) : review?.withdrawn ? (
                    'Withdrawn'
                  ) : row.uncertain ? (
                    'Check transcription'
                  ) : (
                    'Unreviewed'
                  )}
                </span>
              </div>
              <blockquote>{row.original}</blockquote>
              {hasPage && (
                <>
                  <button
                    className="text-button"
                    type="button"
                    aria-expanded={viewing === row.id}
                    onClick={() => setViewing(viewing === row.id ? '' : row.id)}
                  >
                    {viewing === row.id
                      ? 'Close original page'
                      : 'View original page'}
                  </button>
                  {viewing === row.id && (
                    <OriginalPage
                      url={url}
                      kind={doc.kind}
                      title={doc.title}
                      locator={row.locator}
                    />
                  )}
                </>
              )}

              {current && (
                <div>
                  <h4>Reviewed entry</h4>
                  <p>{rowText(current.row)}</p>
                  {secondary && <p>{secondary}</p>}
                  <small>
                    {current.row.subject === 'family'
                      ? 'Family history'
                      : current.row.subject === 'other'
                        ? 'Another person'
                        : 'Personal record'}
                    {current.row.uncertain ? ' · Transcription uncertain' : ''}
                  </small>
                </div>
              )}
              {!!prior.length && (
                <details
                  className="archive"
                  open={
                    prior.some((r) => r.id === requestedReview) || undefined
                  }
                >
                  <summary>Earlier transcriptions ({prior.length})</summary>
                  {prior.map((r) => (
                    <section key={r.id}>
                      <h4>
                        {r.id === requestedReview
                          ? 'Transcription cited by this assessment'
                          : 'Earlier transcription'}{' '}
                        · History revision {r.historyRevision}
                      </h4>
                      <p>
                        {r.withdrawn ? 'Withdrawn' : 'Superseded'} ·{' '}
                        {r.row.subject === 'family'
                          ? 'Family history'
                          : r.row.subject === 'self'
                            ? 'Personal record'
                            : 'Another person'}
                      </p>
                      <blockquote>{r.row.excerpt || r.row.original}</blockquote>
                      <p>{rowText(r.row)}</p>
                    </section>
                  ))}
                </details>
              )}
              {edit === row.id ? (
                <RowEditor
                  row={current?.row || row}
                  busy={busy}
                  cancel={() => setEdit('')}
                  save={(data) =>
                    void act({
                      action: 'review_report_row',
                      documentId: doc.id,
                      rowId: row.id,
                      row: data,
                    })
                  }
                />
              ) : (
                <div className="actions">
                  <button
                    className="secondary"
                    disabled={busy || doc.belongsTo !== 'self'}
                    aria-label={`${current ? 'Correct entry' : 'Review entry'} · ${row.locator}`}
                    onClick={() => setEdit(row.id)}
                  >
                    {current ? 'Correct entry' : 'Review entry'}
                  </button>
                  {current && (
                    <>
                      <label className="check">
                        <input
                          type="checkbox"
                          checked={ids.includes(row.id)}
                          onChange={(ev) =>
                            changeIds(
                              ev.target.checked
                                ? [...ids, row.id]
                                : ids.filter((id) => id !== row.id),
                            )
                          }
                        />
                        Select for model
                      </label>
                      <button
                        className="text-button danger"
                        disabled={busy}
                        onClick={() =>
                          void act({
                            action: 'withdraw_report_row',
                            documentId: doc.id,
                            rowId: row.id,
                          })
                        }
                      >
                        Withdraw entry
                      </button>
                    </>
                  )}
                </div>
              )}
            </article>
          )
        })}
        {rows.length > 12 && (
          <div className="actions">
            <button
              className="secondary"
              disabled={page === 0}
              onClick={() => setPage((p) => p - 1)}
            >
              Previous passages
            </button>
            <span>
              {page + 1} / {Math.ceil(rows.length / 12)}
            </span>
            <button
              className="secondary"
              disabled={(page + 1) * 12 >= rows.length}
              onClick={() => setPage((p) => p + 1)}
            >
              Next passages
            </button>
          </div>
        )}
        <button className="text-button" onClick={() => setManual(!manual)}>
          {manual
            ? 'Close manual transcription'
            : 'Add a manually transcribed passage'}
        </button>
        {manual && (
          <form
            onSubmit={(ev) => {
              ev.preventDefault()
              void act({
                action: 'add_report_passage',
                documentId: doc.id,
                text: passage,
                locator,
              }).then((ok) => {
                if (ok) {
                  setManual(false)
                  setPassage('')
                  setLocator('')
                }
              })
            }}
          >
            <label>
              Original wording
              <textarea
                required
                value={passage}
                onChange={(ev) => setPassage(ev.target.value)}
                maxLength={4000}
              />
            </label>
            <label>
              Page or line reference
              <input
                required
                value={locator}
                onChange={(ev) => setLocator(ev.target.value)}
                maxLength={160}
              />
            </label>
            <button className="secondary" disabled={busy}>
              Add source passage
            </button>
          </form>
        )}
      </section>
      <section className="panel">
        <h2>Disclosure preview</h2>
        <p>
          Only selected, reviewed entries from this report can be approved for
          the encounter’s provider. Permanent originals and unselected pages
          stay on this installation.
        </p>
        <p className="metadata">
          {kinds[e.provider.kind]} · <Destination p={e.provider} /> ·{' '}
          {e.provider.model}
        </p>
        <button
          className="secondary"
          disabled={busy || !ids.length}
          onClick={() => run(readPreview)}
        >
          Preview selected extracts ({ids.length})
        </button>
        {(e.reportReviews || []).some(
          (r) => r.active && r.documentId === doc.id && r.approvedFor,
        ) && (
          <button
            className="text-button"
            disabled={busy}
            onClick={() =>
              void act({
                action: 'authorize_report',
                documentId: doc.id,
                rowIds: [],
              })
            }
          >
            Remove this report’s model permissions
          </button>
        )}
        {preview && (
          <div className="preview-box">
            <h3>Selected model input</h3>
            {preview.map((r) => (
              <article key={r.rowId} className="history-row">
                <h4>{r.name || 'Reviewed passage'}</h4>
                <p>
                  {[
                    r.value,
                    r.units,
                    r.referenceRange && 'Reference ' + r.referenceRange,
                    r.flag,
                    r.locator,
                  ]
                    .filter(Boolean)
                    .join(' · ')}
                </p>
                <blockquote>{r.quote}</blockquote>
                <small>
                  {[
                    r.subject === 'family'
                      ? 'Family history'
                      : r.subject === 'self'
                        ? 'Personal history'
                        : 'Another person',
                    r.note,
                    r.uncertain && 'Uncertain transcription',
                  ]
                    .filter(Boolean)
                    .join(' · ')}
                </small>
              </article>
            ))}
            <details>
              <summary>Inspect exact data fields</summary>
              <pre>{JSON.stringify(preview, null, 2)}</pre>
            </details>
            <label className="check">
              <input
                type="checkbox"
                checked={consent}
                onChange={(ev) => setConsent(ev.target.checked)}
              />
              I approve these extracts for this provider and endpoint.
            </label>
            <button
              className="primary"
              disabled={busy || !consent || !preview.length}
              onClick={() =>
                void act({
                  action: 'authorize_report',
                  documentId: doc.id,
                  rowIds: ids,
                })
              }
            >
              <Send size={16} />
              Approve selected extracts
            </button>
          </div>
        )}
        <div className="actions">
          <button
            className="secondary"
            disabled={busy || !!e.scopeReason}
            onClick={onInterview}
          >
            Continue to interview
          </button>
        </div>
      </section>
    </>
  )
}
