import type { Encounter, Fact, Provider } from './types'
export type ReportRow = {
  id: string
  original: string
  excerpt?: string
  locator: string
  name: string
  value: string
  units: string
  referenceRange: string
  flag: string
  collectionDate: string
  issueDate: string
  subject: string
  note: string
  uncertain: boolean
}
export type DocumentSource = {
  priorEncounter?: string
  priorRevision?: number
  medicationLabel?: boolean
  id: string
  title: string
  kind: string
  belongsTo: 'self' | 'other' | 'unknown'
  purpose: string
  priority: string
  issueDate: string
  rows: ReportRow[]
}
export type ReportReview = {
  id: string
  documentId: string
  rowId: string
  row: ReportRow
  active: boolean
  withdrawn: boolean
  historyRevision: number
  factId: string
  approvedFor?: Provider
}
export type Outcome = {
  id: string
  date: string
  source: 'documented' | 'patient_reported' | 'symptom_change'
  text: string
  documentId?: string
  assessmentVersion: number
  createdAt: string
}
export type QuestionsSource = {
  key?: string
  entryLabel?: string
  answerHint?: string
  documentId: string
  rowId: string
  reviewId: string
  reason: string
  pathway: string
}
export type Region = {
  locale: string
  units: string
  emergency: string
  careDestination: string
}
export type Origin = {
  category?: string
  reconciliationStatus?: string
  encounterId: string
  factId: string
  historyRevision: number
  date: string
  note: string
  sourceType: string
  documentId?: string
  rowId?: string
}
export type Summary = {
  historyRevision: number
  confirmedAt: string
  factIds: string[]
  answerIds: string[]
}
export function latestReview(e: Encounter, docId: string, rowId: string) {
  return (e.reportReviews || [])
    .filter((r) => r.documentId === docId && r.rowId === rowId)
    .at(-1)
}
export function factSource(f: Fact) {
  if (f.sourceType === 'witness_observation') return 'Witness observation'
  if (f.origin) return 'Reconciled from earlier history'
  if (f.sourceType === 'report_entry') return 'Reviewed report entry'
  if (f.sourceType === 'patient_correction') return 'Patient correction'
  return 'Patient account'
}
export function rowText(row: ReportRow) {
  return [
    row.name,
    row.value,
    row.units,
    row.referenceRange && 'Reference ' + row.referenceRange,
    row.flag && 'Flag ' + row.flag,
    row.collectionDate && 'Collected ' + row.collectionDate,
    row.issueDate && 'Issued ' + row.issueDate,
    row.note,
  ]
    .filter(Boolean)
    .join(' · ')
}
