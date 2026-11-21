import { useState } from 'react'
import { VoiceInput } from './voice'
import type { Answer, Encounter, Fields } from './types'
import { rowText } from './records'

type Reply = {
  format: string
  purpose: string
  encounter: string
  revision: number
  speaker: string
  text: string
  category: string
}
export function detailLine(a: Answer) {
  const d = a.detail
  if (!d) return ''
  return [
    d.onset && 'Started ' + new Date(d.onset).toLocaleString(),
    d.end && 'Ended ' + new Date(d.end).toLocaleString(),
    d.approximate && 'Approximate times',
    d.firstRecorded && 'Recorded ' + new Date(d.firstRecorded).toLocaleString(),
    d.feedbackFrom &&
      'Earlier feedback from ' + d.feedbackFrom + ' · ' + d.feedbackText,
    d.printed && 'Printed label · ' + d.printed,
    d.prescribed && 'Prescribed instructions as reported · ' + d.prescribed,
    a.eventDate && 'Event date · ' + a.eventDate,
  ]
    .filter(Boolean)
    .join(' · ')
}
function downloadInvitation(e: Encounter, purpose: string) {
  // Only server-generated identifiers and fixed labels enter the offline form.
  if (!/^[a-f0-9]{32}$/.test(e.id) || !Number.isSafeInteger(e.historyRevision))
    throw Error('Open a saved encounter first.')
  const witness = purpose === 'witness',
    title = witness ? 'Witness observation' : 'Feedback on the visit brief'
  const html = `<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; form-action 'none'; base-uri 'none'"><title>${title}</title><style>body{font:18px system-ui;max-width:38rem;margin:5vh auto;padding:1rem}label,textarea,select,input,button{display:block;width:100%;margin:1rem 0;font:inherit}textarea{height:8rem}</style><h1>${title}</h1><p>This offline form belongs to a healthctl visit. Return the response file to the patient yourself. They decide whether to include it.</p><label>Your name or relationship<input id="speaker" maxlength="80"></label>${witness ? '' : '<label>Feedback category<select id="category"><option value="retake">Had to ask again</option><option value="incorrect">Something was incorrect</option><option value="changed_question">Changed my next question</option></select></label>'}<label>${witness ? 'In your own words, what did you personally observe about the patient?' : 'Which detail, and what would you change?'}<textarea id="words" maxlength="2500"></textarea></label><button id="save">Save response file</button><p id="message" role="status"></p><script>document.getElementById('save').onclick=()=>{const val=id=>document.getElementById(id).value.trim();if(!val('speaker')||!val('words')){document.getElementById('message').textContent='Add your name or relationship and your account.';return}const reply={format:'healthctl-contribution-1',purpose:'${purpose}',encounter:'${e.id}',revision:${e.historyRevision},speaker:val('speaker'),text:val('words'),category:${witness ? "''" : "val('category')"}};const u=URL.createObjectURL(new Blob([JSON.stringify(reply)],{type:'application/json'}));const a=document.createElement('a');a.href=u;a.download='healthctl-response.json';a.click();setTimeout(()=>URL.revokeObjectURL(u),5000);document.getElementById('message').textContent='Response prepared. Return it privately to the patient.'}</script></html>`
  const url = URL.createObjectURL(new Blob([html], { type: 'text/html' }))
  const link = document.createElement('a')
  link.href = url
  link.download = 'healthctl-' + purpose + '-form.html'
  link.click()
  setTimeout(() => URL.revokeObjectURL(url), 5000)
}
export function VisitPrep({
  e,
  act,
  busy,
  onSources,
}: {
  e: Encounter
  act: (c: Fields) => Promise<boolean>
  busy: boolean
  onSources: () => void
}) {
  const [mode, setMode] = useState('episode_log'),
    [text, setText] = useState(''),
    [speaker, setSpeaker] = useState(''),
    [ids, setIds] = useState<string[]>([]),
    [onset, setOnset] = useState(''),
    [end, setEnd] = useState(''),
    [approx, setApprox] = useState(false),
    [rx, setRx] = useState(''),
    [date, setDate] = useState(''),
    [category, setCategory] = useState('retake'),
    [consent, setConsent] = useState(false),
    [err, setErr] = useState(''),
    [notice, setNotice] = useState('')
  const rows = (e.reportReviews || []).filter(
    (r) =>
      r.active &&
      !r.withdrawn &&
      r.row.subject === 'self' &&
      r.approvedFor?.kind === e.provider.kind &&
      r.approvedFor.model === e.provider.model &&
      r.approvedFor.endpoint === e.provider.endpoint &&
      r.approvedFor.structured === e.provider.structured,
  )
  const external = mode === 'witness' || mode === 'clinician_feedback',
    sources = mode === 'medication_use' || mode === 'resolve_sources'
  function reset() {
    setText('')
    setSpeaker('')
    setIds([])
    setConsent(false)
    setRx('')
    setDate('')
    setOnset('')
    setEnd('')
    setApprox(false)
    setErr('')
  }
  async function load(file?: File) {
    if (!file) return
    try {
      if (file.size > 18000) throw Error()
      const r = JSON.parse(await file.text()) as Reply
      if (
        r.format !== 'healthctl-contribution-1' ||
        r.purpose !== mode ||
        r.encounter !== e.id ||
        r.revision !== e.historyRevision ||
        typeof r.text !== 'string' ||
        !r.text.trim() ||
        r.text.length > 3000 ||
        typeof r.speaker !== 'string' ||
        !r.speaker.trim() ||
        r.speaker.length > 80 ||
        (mode === 'clinician_feedback' &&
          !['retake', 'incorrect', 'changed_question'].includes(r.category))
      )
        throw Error()
      setText(r.text)
      setSpeaker(r.speaker)
      if (r.category) setCategory(r.category)
      setConsent(false)
      setErr('')
    } catch {
      setErr(
        'This response must match the selected form and current visit revision.',
      )
    }
  }
  return (
    <section className="panel form-panel">
      <h2>Visit preparation</h2>
      <label>
        What would you like to record?
        <select
          value={mode}
          onChange={(ev) => {
            reset()
            setNotice('')
            setMode(ev.target.value)
          }}
        >
          <option value="episode_log">It happened again</option>
          <option value="medication_use">How I take a medicine</option>
          <option value="resolve_sources">Reconcile two documents</option>
          {e.age >= 18 && <option value="witness">A witness’s account</option>}
          <option value="clinician_feedback">
            Feedback on the clinician brief
          </option>
        </select>
      </label>
      {external && (
        <>
          <p>
            Give the contributor a blank offline form or enter their account
            together here. The patient reviews the response before including it.
            The contributor’s identity is self-reported.
          </p>
          <button
            className="secondary"
            disabled={
              busy ||
              (mode === 'clinician_feedback' &&
                e.confirmedRevision !== e.historyRevision)
            }
            onClick={() => downloadInvitation(e, mode)}
          >
            Download blank {mode === 'witness' ? 'witness' : 'feedback'} form
          </button>
          <label>
            Open returned response
            <input
              type="file"
              accept=".json"
              onChange={(ev) => void load(ev.target.files?.[0])}
            />
          </label>
        </>
      )}
      {sources && (
        <>
          <p>
            Select reviewed, authorized passages. Their original wording stays
            linked to your account.
          </p>
          <button className="text-button" onClick={onSources}>
            Upload or review source documents
          </button>
          {rows
            .filter(
              (r) =>
                mode !== 'medication_use' ||
                e.documents?.find((d) => d.id === r.documentId)
                  ?.medicationLabel,
            )
            .map((r) => (
              <label key={r.id}>
                <input
                  type={mode === 'medication_use' ? 'radio' : 'checkbox'}
                  name="prep-source"
                  checked={ids.includes(r.id)}
                  onChange={() =>
                    setIds(
                      mode === 'medication_use'
                        ? [r.id]
                        : ids.includes(r.id)
                          ? ids.filter((x) => x !== r.id)
                          : [...ids, r.id],
                    )
                  }
                />
                {e.documents?.find((d) => d.id === r.documentId)?.title} ·{' '}
                {r.row.locator} · {r.row.excerpt || r.row.original} ·{' '}
                {rowText(r.row)}
              </label>
            ))}
        </>
      )}
      <form
        onSubmit={async (ev) => {
          ev.preventDefault()
          setErr('')
          const ok = await act({
            action: mode,
            text: text.trim(),
            speaker,
            reviewIds: ids,
            prescribed: rx,
            date,
            category,
            confirmedDestination: consent,
            forRevision: e.historyRevision,
            approximate: approx,
            onset: onset ? new Date(onset).toISOString() : '',
            end: end ? new Date(end).toISOString() : '',
          })
          if (ok) {
            reset()
            setNotice('Saved to this encounter.')
          }
        }}
      >
        {mode === 'episode_log' && (
          <>
            <label>
              Episode start
              <input
                type="datetime-local"
                required
                value={onset}
                onChange={(ev) => setOnset(ev.target.value)}
              />
            </label>
            <label>
              End, if known
              <input
                type="datetime-local"
                min={onset}
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
              Approximate times
            </label>
          </>
        )}
        {external && (
          <label>
            Contributor’s name or relationship
            <input
              required
              maxLength={80}
              value={speaker}
              onChange={(ev) => setSpeaker(ev.target.value)}
            />
          </label>
        )}
        {mode === 'medication_use' && (
          <>
            <label>
              Prescribed instructions, if known
              <input
                maxLength={500}
                value={rx}
                onChange={(ev) => setRx(ev.target.value)}
              />
            </label>
            <label>
              Date actual use changed, if known
              <input
                type="date"
                value={date}
                onChange={(ev) => setDate(ev.target.value)}
              />
            </label>
          </>
        )}
        {mode === 'resolve_sources' && (
          <label>
            When did the event actually happen, if known?
            <input
              type="date"
              value={date}
              onChange={(ev) => setDate(ev.target.value)}
            />
          </label>
        )}
        {mode === 'clinician_feedback' && (
          <label>
            Feedback category
            <select
              value={category}
              onChange={(ev) => setCategory(ev.target.value)}
            >
              <option value="retake">Had to ask again</option>
              <option value="incorrect">Incorrect detail</option>
              <option value="changed_question">Changed my next question</option>
            </select>
          </label>
        )}
        <label>
          {mode === 'medication_use'
            ? 'How do you actually take it?'
            : mode === 'resolve_sources'
              ? 'How do you remember the difference?'
              : mode === 'witness'
                ? 'Witness’s own words'
                : mode === 'clinician_feedback'
                  ? 'Feedback for patient review'
                  : 'What happened during this episode?'}
          <textarea
            required
            maxLength={2500}
            value={text}
            onChange={(ev) => setText(ev.target.value)}
          />
        </label>
        <VoiceInput limit={2500} onUse={setText} />
        {mode !== 'episode_log' && (
          <label>
            <input
              type="checkbox"
              required
              checked={consent}
              onChange={(ev) => setConsent(ev.target.checked)}
            />
            {mode === 'clinician_feedback'
              ? 'I reviewed this feedback and want to keep it with this brief.'
              : 'Include this attributed account in my history, model interview and visit brief.'}
          </label>
        )}
        <button
          className="primary"
          disabled={
            busy ||
            (mode === 'medication_use' && ids.length !== 1) ||
            (mode === 'resolve_sources' && ids.length < 2) ||
            (mode === 'clinician_feedback' &&
              e.confirmedRevision !== e.historyRevision)
          }
        >
          Save reviewed entry
        </button>
      </form>
      {e.feedback?.map((f, i) => (
        <p key={i}>
          {f.speaker} · {f.category} · {f.text} · History revision {f.revision}
        </p>
      ))}
      {err && <p role="alert">{err}</p>}
      {notice && <p role="status">{notice}</p>}
    </section>
  )
}
