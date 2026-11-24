import { useEffect, useRef, useState } from 'react'

export type SetAside = { text: string; question: string }

// useQuestionDraft ties a draft answer to the question it answers. If the
// question changes underneath it (another tab, a retried request, a reload
// after a conflict), the words are set aside instead of being saved against the
// new question, and onChange lets the caller reset related input.
export function useQuestionDraft(question: string, onChange?: () => void) {
  const [answer, setAnswer] = useState(''),
    [draftFor, setDraftFor] = useState(question),
    [setAside, setSetAside] = useState<SetAside | null>(null)
  const latest = useRef(answer)
  latest.current = answer
  useEffect(() => {
    if (question === draftFor) return
    if (latest.current.trim()) setSetAside({ text: latest.current, question: draftFor })
    setAnswer('')
    setDraftFor(question)
    onChange?.()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [question])
  return {
    answer,
    setAnswer,
    setAside,
    restore: () => {
      if (setAside) setAnswer(setAside.text)
      setSetAside(null)
    },
    discard: () => setSetAside(null),
  }
}
