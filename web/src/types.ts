import type {
  DocumentSource,
  ReportReview,
  Outcome,
  QuestionsSource,
  Origin,
  Summary,
} from './records'
export type ProviderKind =
  'compatible' | 'openai' | 'chatgpt' | 'anthropic' | 'demo'
export type Provider = {
  kind: ProviderKind
  model: string
  endpoint: string
  structured: 'auto' | 'native' | 'prompt'
  reasoning?: '' | 'effort'
}
export type Settings = {
  provider: Provider | null
  allowlist: string[]
  credentialStatus: Record<string, boolean | string>
}
export type AssessmentNotice = {
  version: string
  risks: string
  liability: string
  choice: string
}
export type Session = {
  assessmentNotice: AssessmentNotice
  setupRequired: boolean
  authenticated: boolean
  csrf: string
  displayName: string
  notice: string
  noticeVersion: string
  emergency: string
}
export type VisitDetail = {
  feedbackFrom?: string
  feedbackText?: string
  speaker?: string
  onset?: string
  end?: string
  approximate?: boolean
  firstRecorded: string
  priorEncounter?: string
  printed?: string
  prescribed?: string
}
export type Feedback = {
  speaker: string
  category: string
  text: string
  revision: number
  at: string
}
export type Answer = {
  detail?: VisitDetail
  eventDate?: string
  reviewIds?: string[]
  disposition?: string
  sourceOutdated?: boolean
  id: string
  kind: string
  question: string
  text: string
  active: boolean
  supersedes?: string
  createdAt: string
}
export type Fact = {
  sourceOutdated?: boolean
  sourceType?: string
  documentId?: string
  rowId?: string
  reviewId?: string
  origin?: Origin
  historyRevision?: number
  id: string
  concept: string
  // category is set by the app from where the finding was recorded.
  category?: 'medication' | 'allergy'
  term?: string
  code?: string
  quote: string
  answerId: string
  status: string
  subject: string
  timeContext: string
  value: string
  units: string
  active: boolean
  confirmed: boolean
  corrected: boolean
}
export type Warning = {
  id: string
  message: string
  quote: string
  answerId: string
  active: boolean
  createdAt: string
  level?: 'check'
}
export type Item = {
  name: string
  clinicalTerm: string
  explanation: string
  supporting: string[]
  conflicting: string[]
  limitations: string[]
  vetted?: boolean
  description?: string
  descriptionStatus?: string
  unstable?: boolean
}
export type Assessment = {
  dontMiss?: string[]
  stabilityChecked?: boolean
  literature?: (import('./evidence').SearchResult & { itemIndex: number })[]
  corpusVersion?: string
  version: number
  historyRevision: number
  createdAt: string
  outcome: string
  provider: { provider: string; requestedModel: string; returnedModel: string }
  destination: Provider
  items: Item[]
  outstanding: string[]
}
export type EncounterRow = {
  pending?: string
  id: string
  concern: string
  state: string
  revision: number
  historyRevision: number
  createdAt: string
  updatedAt: string
  provider: Provider
  demo?: boolean
}
export type SafetyCheck = {
  historyRevision: number
  point: 'review' | 'return'
  answer: 'none' | 'changed' | 'emergency'
  at: string
}
export type Encounter = EncounterRow & {
  safetyChecks?: SafetyCheck[]
  redFlagsDue?: import('./records').RedFlagQuestion[]
  gaps?: import('./records').Gap[]
  returnSource?: string
  agenda?: string
  narrativeFirst?: boolean
  focused?: boolean
  feedback?: Feedback[]
  episodeId?: string
  episodeTitle?: string
  reportFirst?: boolean
  photos?: { id: string; area: string; createdAt: string }[]
  pathways?: { id: string }[]
  documents?: DocumentSource[]
  reportReviews?: ReportReview[]
  outcomes?: Outcome[]
  questionSource?: QuestionsSource
  summaries?: Summary[]
  turns?: {
    question?: string
    reason: string
    task: string
    historyRevision: number
  }[]
  confirmedRevision: number
  age: number
  background: string
  pregnancyRelated: boolean
  demo: boolean
  notice: string
  noticeVersion: string
  answers: Answer[]
  facts: Fact[]
  warnings: Warning[]
  contradictions: { concept: string; factIds: string[] }[]
  outstanding: string[]
  question: string
  choices: string[]
  assessments: Assessment[]
  failedTask?: string
  failure: string
  scopeReason: string
  pending: string
  providerChanges: { previous: Provider; current: Provider; at: string }[]
}
export type Report = Encounter & { exportedAt: string; exportVersion: string }
export type Fields = Record<string, unknown>
export const kinds: Record<ProviderKind, string> = {
  compatible: 'OpenAI-compatible endpoint',
  chatgpt: 'ChatGPT plan',
  openai: 'OpenAI',
  anthropic: 'Anthropic Claude',
  demo: 'Synthetic demo',
}
export const destination = (p: Provider) =>
  ({
    compatible: p.endpoint,
    chatgpt: 'https://api.openai.com/v1/responses',
    openai: 'https://api.openai.com/v1',
    anthropic: 'https://api.anthropic.com/v1',
    demo: 'Built-in synthetic case',
  })[p.kind]
let locale = 'en'
export function canonicalLocale(value: string) {
  try {
    return Intl.getCanonicalLocales(value || 'en')[0] || 'en'
  } catch {
    return 'en'
  }
}
export function setLocale(value: string) {
  locale = canonicalLocale(value)
}
export const date = (iso: string, withTime = false) =>
  new Intl.DateTimeFormat(locale, {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
    ...(withTime
      ? { hour: 'numeric', minute: '2-digit', timeZoneName: 'short' }
      : {}),
  }).format(new Date(iso))
const states: Record<string, string> = {
  interviewing: 'Interview in progress',
  history_review: 'History review',
  assessing: 'Generating assessment',
  assessment_ready: 'Assessment ready',
  needs_clarification: 'More information needed',
  interrupted_for_safety: 'Urgent warning',
  failed_recoverably: 'Provider unavailable',
  unsupported_scope: 'History only',
  report_review: 'Review report entries',
}
export const stateName = (v: string) => states[v] || human(v)
const sourceNames: Record<string, string> = {
  patient_reported: 'self-reported',
  patient_answer: 'your answer',
  patient_correction: 'your correction',
}
export const human = (v: string) => sourceNames[v] || v.replaceAll('_', ' ')
export const sampleConcern = 'My nose is runny and I keep sneezing.'
export const providerDefaults = (): Provider => ({
  kind: 'compatible',
  model: '',
  endpoint: '',
  structured: 'auto',
})
