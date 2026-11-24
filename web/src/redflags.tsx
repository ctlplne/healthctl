import type { Gap, RedFlagQuestion } from './records'

// RedFlags shows the application's own safety questions for the active
// symptom areas that still need an answer. The wording belongs to the app; a
// yes raises the emergency warning without asking the model.
export function RedFlags({
  due,
  busy,
  onAnswer,
}: {
  due: RedFlagQuestion[]
  busy: boolean
  onAnswer: (key: string, answer: 'yes' | 'no' | 'unsure') => void
}) {
  if (!due.length) return null
  return (
    <section className="panel safety-net red-flags" aria-labelledby="red-flags-heading">
      <h2 id="red-flags-heading">Safety questions for this concern</h2>
      <p>The app asks these for every concern like yours. Development content, not clinically reviewed.</p>
      {due.map((q) => (
        <div key={q.key} role="group" aria-label={q.question} className="red-flag">
          <p>{q.question}</p>
          <div className="actions">
            <button type="button" className="primary danger" disabled={busy} onClick={() => onAnswer(q.key, 'yes')}>
              Yes
            </button>
            <button type="button" className="secondary" disabled={busy} onClick={() => onAnswer(q.key, 'no')}>
              No
            </button>
            <button type="button" className="secondary" disabled={busy} onClick={() => onAnswer(q.key, 'unsure')}>
              Not sure
            </button>
          </div>
        </div>
      ))}
    </section>
  )
}

// Gaps lists what the app expected for this concern that no question has
// covered yet. The model can't remove these; answering a question does.
export function Gaps({ gaps }: { gaps: Gap[] }) {
  if (!gaps.length) return null
  return (
    <section className="panel" aria-labelledby="gaps-heading">
      <h2 id="gaps-heading">Not yet asked</h2>
      <p>The app expects these for this kind of concern. You can continue the interview to answer them, or confirm without them.</p>
      <ul>
        {gaps.map((g) => (
          <li key={g.pathway + g.element + g.note}>{g.note}</li>
        ))}
      </ul>
    </section>
  )
}
