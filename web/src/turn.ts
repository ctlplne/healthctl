import type { Encounter } from './types'

export type Tab = 'interview' | 'history' | 'estimate' | 'report' | 'sources' | 'followup' | 'prep'

export const defaultTab = (e: Pick<Encounter, 'state'>): Tab =>
  e.state === 'history_review'
    ? 'history'
    : e.state === 'assessing' || e.state === 'assessment_ready' || e.state === 'needs_clarification'
      ? 'estimate'
      : 'interview'

// readyNotice describes a finished model turn without moving the user. When
// the result is on another tab, it offers a button to go there; the tab the
// user is on, and anything they were typing, stay as they were.
export function readyNotice(e: Pick<Encounter, 'state' | 'question'>, tab: Tab): { message: string; goTo?: Tab } {
  const message =
    e.state === 'interviewing' && e.question ? 'Your next question is ready.' :
    e.state === 'history_review' ? 'Your history is ready to review.' :
    e.state === 'needs_clarification' ? 'The model needs more information.' :
    e.state === 'assessment_ready' ? 'Your estimate is ready to review.' : ''
  const target = defaultTab(e)
  return message && target !== tab ? { message, goTo: target } : { message }
}
