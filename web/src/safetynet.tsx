import type { Encounter } from './types'

export const SAFETY_NET_QUESTION =
  'Before you go on: is anything new, or getting rapidly worse, since you started? For example chest pain, trouble breathing, weakness or numbness on one side, confusion, or thoughts of harming yourself.'

export function safetyCheckDue(e: Encounter, point: 'review' | 'return') {
  return !(e.safetyChecks || []).some((c) => c.point === point && c.historyRevision === e.historyRevision)
}

// SafetyNet asks the application's own safety question. The wording and what
// happens next belong to the app: an emergency answer shows the emergency
// warning, and a change goes back to the interview to describe it.
export function SafetyNet({
  point,
  busy,
  onAnswer,
}: {
  point: 'review' | 'return'
  busy: boolean
  onAnswer: (answer: 'none' | 'changed' | 'emergency') => void
}) {
  return (
    <section className="panel safety-net" aria-labelledby={'safety-net-' + point}>
      <h2 id={'safety-net-' + point}>A quick safety check</h2>
      <p>{SAFETY_NET_QUESTION}</p>
      <div className="actions">
        <button type="button" className="secondary" disabled={busy} onClick={() => onAnswer('none')}>
          No, nothing new or worse
        </button>
        <button type="button" className="secondary" disabled={busy} onClick={() => onAnswer('changed')}>
          Something is new or worse
        </button>
        <button type="button" className="primary danger" disabled={busy} onClick={() => onAnswer('emergency')}>
          I may be having an emergency now
        </button>
      </div>
    </section>
  )
}
