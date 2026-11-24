import { ObservationPhotos } from './observationPhotos'
import { SafetyNet, safetyCheckDue } from './safetynet'
import { EncounterUsage } from './usage'
import { defaultTab, readyNotice, type Tab } from './turn'
import { useQuestionDraft } from './draft'
import { VisitPrep } from './visitPrep'
import { ReadBack, VoiceInput } from './voice'
import { useEffect, useRef, useState } from 'react'
import {
  BookOpenCheck,
  ChevronDown,
  CirclePause,
  ClipboardList,
  Download,
  FileText,
  ListChecks,
  LoaderCircle,
  MessageSquareText,
  Network,
  Printer,
  Save,
  Trash2,
  WifiOff,
} from 'lucide-react'
import type { AssessmentNotice, Encounter, Fact, Fields, Provider, Report } from './types'
import { date, kinds, stateName } from './types'
import { Callout, ContextBox, DemoTag, Notice, PageTitle } from './shared'
import { AssessmentRequest, History } from './history'
import { Estimate, ReportBody, UrgencyReminder } from './report'
import type { Run } from './forms'
import { api } from './api'
import { SourceReview } from './documents'
import { Destination, PlanUsage, ManageUsage } from './chatgpt'
import { Timeline } from './timeline'
import { EvidenceLookup } from './evidence'
import { AfterVisit, Revisions, Interop, BriefBody, Appendix } from './handoff'
import type { Brief } from './handoff'

const responseOptions = [
  { label: 'Not sure', text: 'Not sure' },
  { label: 'Not applicable', text: 'Not applicable' },
  { label: 'Skip this question', text: 'I prefer not to answer.' },
]
const isStandardOption = (s: string) =>
  /^(?:i(?:’m|'m| am)? )?(?:not sure|unsure|do(?:n['’]t| not) know|not applicable(?: to me)?|n\/?a|prefer not to answer|skip(?: this question)?)[.!]?$/i.test(
    s.trim(),
  )


function ElapsedWait({ since }: { since: string }) {
  const [now, setNow] = useState(Date.now)
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [])
  const seconds = Math.max(0, Math.floor((now - Date.parse(since)) / 1000))
  if (!Number.isFinite(seconds)) return null
  return (
    <span className="model-elapsed" role="timer" aria-label="Elapsed wait" aria-live="off">
      Waiting {Math.floor(seconds / 60)}:{String(seconds % 60).padStart(2, '0')}
    </span>
  )
}

type Props = {
  e: Encounter
  name: string
  emergency: string
  assessmentNotice: AssessmentNotice
  provider: Provider | null
  busy: boolean
  run: Run
  onAction: (cmd: Fields) => Promise<Encounter | undefined>
  onLeave: () => void
  onDelete: () => Promise<void>
  onUpdate: (e: Encounter) => void
  careDestination: string
  pollError: string
  startAtSources?: boolean
  // initialTab opens a tab named in the URL; onTab reports tab changes to it.
  initialTab?: string
  onTab?: (tab: Tab) => void
}
export function EncounterPage({
  e,
  name,
  emergency,
  assessmentNotice,
  provider,
  busy,
  run,
  onAction,
  onLeave,
  onDelete,
  onUpdate,
  careDestination,
  pollError,
  startAtSources = false,
  initialTab,
  onTab,
}: Props) {
  const [tab, setTab] = useState<Tab>(() =>
      initialTab && ['interview', 'history', 'estimate', 'report', 'sources', 'followup', 'prep'].includes(initialTab)
        ? (initialTab as Tab)
        : startAtSources ||
      (e.reportFirst &&
        !e.documents?.length &&
        !e.warnings.some((w) => w.active))
        ? 'sources'
        : defaultTab(e),
    ),
    [eventDate, setEventDate] = useState(''),
    [answerMode, setAnswerMode] = useState<'choice' | 'text'>(),
    [urgentNow, setUrgentNow] = useState(false),
    [deleting, setDeleting] = useState(false),
    [moving, setMoving] = useState(false),
    [consent, setConsent] = useState(false),
    [report, setReport] = useState<Report | null>(null)
  const [source, setSource] = useState<{
      documentId: string
      rowId: string
      reviewId?: string
    }>(),
    [focusFact, setFocusFact] = useState(''),
    [format, setFormat] = useState<'user' | 'brief' | 'appendix'>('user'),
    [brief, setBrief] = useState<Brief | null>(null),
    [timeline, setTimeline] = useState(false)
  const viewFact = (f: Fact) => {
    if (f.documentId && f.rowId) {
      setSource({
        documentId: f.documentId,
        rowId: f.rowId,
        reviewId: f.reviewId,
      })
      setTab('sources')
    } else {
      setFocusFact(f.id)
      setTab('history')
    }
  }
  const textarea = useRef<HTMLTextAreaElement>(null)
  const choices = [...new Set(e.choices || [])].filter(
    (c) => !isStandardOption(c),
  )
  const background = e.questionSource?.key === 'background' && !!e.question
  const [revealedQuestion, setRevealedQuestion] = useState('')
  const questionKey = e.historyRevision + '|' + e.question
  const storyFirst =
    e.narrativeFirst && !background && revealedQuestion !== questionKey
  const ownWords = answerMode === 'text'
  const writeAnswer =
    (!e.demo && !background) || ownWords ||
    (!!storyFirst && answerMode !== 'choice') ||
    (!answerMode && !choices.length && !background)
  useEffect(() => {
    if (ownWords) textarea.current?.focus()
  }, [ownWords])
  const activeWarnings = e.warnings.filter((w) => w.active)
  const latest = e.assessments.at(-1),
    current = latest?.historyRevision === e.historyRevision ? latest : undefined
  const stale = latest && !current,
    failure = e.state === 'failed_recoverably'
  useEffect(() => {
    setReport(null)
    setBrief(null)
    setFormat('user')
  }, [e.revision])
  const wasPending = useRef(e.pending)
  const turnCount = e.turns?.length ?? 0
  const previousTurns = useRef(turnCount)
  const [readyRevision, setReadyRevision] = useState<number>()
  const statusPanel = useRef<HTMLDivElement>(null)
  useEffect(() => {
    // A finished turn never moves the user: the notice offers the way to the
    // result, and corrections in progress on this tab stay as they were.
    if (wasPending.current && !e.pending) {
      if (turnCount > previousTurns.current) setReadyRevision(e.revision)
    }
    if (e.pending && !wasPending.current && !activeWarnings.length) {
      statusPanel.current?.scrollIntoView({ block: 'nearest' })
    }
    wasPending.current = e.pending
    previousTurns.current = turnCount
  }, [e.pending, e.state, e.revision, turnCount, activeWarnings.length])
  const ready = readyRevision === e.revision ? readyNotice(e, tab) : { message: '' }
  const readyMessage = ready.message
  useEffect(() => onTab?.(tab), [tab])
  // Move focus to a new question so it is announced and ready to answer.
  const questionHeading = useRef<HTMLHeadingElement>(null)
  const shownQuestion = useRef(e.question)
  useEffect(() => {
    if (e.question && e.question !== shownQuestion.current && tab === 'interview') questionHeading.current?.focus()
    shownQuestion.current = e.question
  }, [e.question, tab])
  // A draft belongs to the question it answers. See useQuestionDraft.
  const draft = useQuestionDraft(e.question, () => {
      setEventDate('')
      setAnswerMode(undefined)
      setUrgentNow(false)
    }),
    { answer, setAnswer, setAside } = draft
  const act = async (cmd: Fields): Promise<boolean> => {
    const next = await onAction(cmd)
    if (!next) return false
    if (next.question !== e.question) {
      setAnswer('')
      setEventDate('')
      setAnswerMode(undefined)
      setUrgentNow(false)
    }
    if (cmd.action === 'change_provider') {
      setMoving(false)
      setConsent(false)
    }
    setTab(cmd.action === 'confirm' ? 'history' : defaultTab(next))
    return true
  }
  const addAnswer = async (text: string) => {
    const value = text.trim()
    if (!value && !urgentNow) return
    if (
      await act({
        action: value ? 'answer' : 'report_emergency',
        text: value,
        ...(value && e.questionSource?.key === 'medication_start' && eventDate
          ? { date: eventDate }
          : {}),
        ...(urgentNow ? { urgentNow: true } : {}),
      })
    ) {
      setAnswer('')
      setEventDate('')
      setAnswerMode(undefined)
      setUrgentNow(false)
    }
  }
  const readHistory = () => {
    if (
      e.pending ||
      e.scopeReason ||
      e.state === 'history_review' ||
      e.state === 'assessment_ready' ||
      e.state === 'needs_clarification'
    ) {
      setTab('history')
      return
    }
    void act({ action: 'review' })
  }
  const openReport = () =>
    run(async () => {
      const doc = await api<Report>('/encounters/' + e.id + '/export')
      if (doc.revision > e.revision) onUpdate(doc)
      setReport(doc)
      setFormat('user')
      setTab('report')
    })
  const showInterview = () => {
    setTab('interview')
  }
  const assessmentRequest = assessmentNotice ? (
    <AssessmentRequest
      key={e.id + '/' + e.revision + '/' + assessmentNotice.version}
      notice={assessmentNotice}
      retry={e.failedTask === 'assessment'}
      busy={busy || !!e.pending}
      onAction={act}
      onOpenReport={openReport}
    />
  ) : (
    <Callout title="Assessment notice unavailable">
      Your history is saved. Reload to get the current notice, or open your report.
    </Callout>
  )
  const canMove =
    provider &&
    !e.demo &&
    provider.kind !== 'demo' &&
    ['kind', 'model', 'endpoint', 'structured', 'reasoning'].some(
      (k) => provider[k as keyof Provider] !== e.provider[k as keyof Provider],
    )
  return (
    <>
      <div className="screen-encounter">
        <PageTitle title="Encounter record" onBack={onLeave} />
        <div className="encounter-top">
          <div className="record-metadata">
            <span className="tag">{stateName(e.state)}</span>
            {e.demo && <DemoTag />}
            <span>{date(e.createdAt)}</span>
          </div>
          <h2>{e.concern}</h2>
          <p>
            <Save size={14} /> Saved to this installation · History revision{' '}
            {e.historyRevision}
          </p>
        </div>
        {activeWarnings.map((w) => (
          <Callout key={w.id} title={w.level === 'check' ? 'Get this checked promptly' : 'Urgent warning'} alert>
            <p>{w.message}</p>
            <p>
              Share these findings with the healthcare professional who
              evaluates you.
            </p>
            <small>Reported wording: “{w.quote}”</small>
          </Callout>
        ))}
        {activeWarnings.length > 0 && careDestination && (
          <p className="callout alert">
            Local care information · {careDestination}
          </p>
        )}
        <div ref={statusPanel} className={e.pending ? 'callout model-wait' : undefined}>
          {e.pending && (pollError
            ? <WifiOff size={20} aria-hidden="true" />
            : <LoaderCircle size={20} className="spin" aria-hidden="true" />)}
          <div role="status" aria-live="polite" aria-atomic="true">
            {e.pending ? (
              <>
                <strong>{pollError ? 'Reconnecting to your installation' :
                  e.pending === 'assessment' ? 'Preparing your provisional estimate' :
                  e.state === 'history_review' ? 'Preparing your history for review' :
                  'Preparing your next interview question'}</strong>
                <p>{pollError
                  ? pollError
                  : <>Waiting for <b>{e.provider.model || kinds[e.provider.kind]}</b> to respond.
                    {' '}The model can take a few minutes to reason through your history.
                    Your history is saved, so you can return to the journal while you wait.</>}</p>
              </>
            ) : readyMessage && (
              <p className="model-ready" role="status">
                {readyMessage}{' '}
                {ready.goTo && (
                  <button type="button" className="text-button" onClick={() => setTab(ready.goTo!)}>
                    {ready.goTo === 'interview' ? 'Go to the question' : ready.goTo === 'history' ? 'Review my history' : 'Open the estimate'}
                  </button>
                )}
              </p>
            )}
          </div>
          {e.pending && !pollError && <ElapsedWait key={e.updatedAt} since={e.updatedAt} />}
        </div>
        {failure && (
          <Callout title="The provider could not finish this request" alert>
            {e.provider.kind === 'chatgpt' &&
              /quota|limit|usage unavailable|plan usage/i.test(e.failure) && (
                <ManageUsage primary />
              )}
            <p>
              {e.failure ||
                'Your input is saved and the assessment is incomplete.'}
            </p>
            {e.failedTask !== 'assessment' && (
              <button
                className="secondary"
                disabled={busy}
                onClick={() => void act({ action: 'retry' })}
              >
                Retry provider request
              </button>
            )}
          </Callout>
        )}
        {failure && e.failedTask === 'assessment' && ['interview', 'estimate', 'history'].includes(tab) && (
          <section className="panel">{assessmentRequest}</section>
        )}
        {e.scopeReason && (
          <Callout title="History-only encounter">{e.scopeReason}</Callout>
        )}
        {stale && (
          <Callout title="The earlier estimate is outdated">
            You changed the history after assessment {latest.version}. Review
            the current revision to generate a new estimate.
          </Callout>
        )}
        <nav className="tabs" aria-label="Encounter sections">
          {(
            [
              {
                key: 'interview',
                name: 'Interview',
                icon: MessageSquareText,
                open: showInterview,
              },
              {
                key: 'sources',
                name: 'Source reports',
                icon: ClipboardList,
                open: () => setTab('sources'),
              },
              {
                key: 'history',
                name: 'History review',
                icon: ListChecks,
                open: readHistory,
              },
              {
                key: 'estimate',
                name: 'Estimate',
                icon: BookOpenCheck,
                open: () => setTab('estimate'),
              },
              {
                key: 'report',
                name: 'Report',
                icon: FileText,
                open: openReport,
              },
              ...(!e.demo
                ? [
                    {
                      name: 'Visit preparation',
                      icon: ClipboardList,
                      key: 'prep',
                      open: () => setTab('prep'),
                    },
                  ]
                : []),
              {
                key: 'followup',
                name: 'Follow-up',
                icon: ListChecks,
                open: () => setTab('followup'),
              },
            ] as const
          ).map((t) => (
            <button
              key={t.key}
              disabled={busy}
              aria-current={tab === t.key ? 'page' : undefined}
              className={tab === t.key ? 'active' : ''}
              onClick={t.open}
            >
              <t.icon size={17} />
              {t.name}
            </button>
          ))}
        </nav>
        <div className="encounter-layout">
          <div className="encounter-content">
            {tab === 'sources' && (
              <SourceReview
                e={e}
                busy={busy}
                run={run}
                refresh={onUpdate}
                highlight={source}
                act={async (c) => !!(await onAction(c))}
                onInterview={() => void act({ action: 'continue' })}
              />
            )}
            {tab === 'sources' && (
              <ObservationPhotos
                key={e.id}
                e={e}
                busy={busy}
                run={run}
                refresh={onUpdate}
                act={act}
              />
            )}
            {tab === 'prep' && (
              <VisitPrep
                e={e}
                busy={busy}
                act={async (c) => !!(await onAction(c))}
                onSources={() => setTab('sources')}
              />
            )}
            {tab === 'followup' && (
              <AfterVisit
                e={e}
                busy={busy}
                act={async (c) => !!(await onAction(c))}
              />
            )}
            {tab === 'interview' && (
              <section className="panel interview-panel">
                {e.provider.kind === 'chatgpt' && <PlanUsage />}
                <div className="topic-mark">
                  <MessageSquareText size={19} />{' '}
                  {background
                    ? 'Health background'
                    : e.question
                      ? 'Interview question'
                      : 'Interview'}
                </div>
                {e.pending ? (
                  <div className="actions">
                    <button
                      className="secondary"
                      disabled={busy}
                      onClick={onLeave}
                    >
                      Pause and return to journal
                    </button>
                    {e.state !== 'history_review' && (
                      <button
                        className="secondary"
                        disabled={busy}
                        onClick={readHistory}
                      >
                        Review saved history now
                      </button>
                    )}
                    {!activeWarnings.length && (
                      <button
                        className="text-button"
                        disabled={busy}
                        onClick={() => void act({ action: 'report_emergency' })}
                      >
                        I may be having a medical emergency right now
                      </button>
                    )}
                  </div>
                ) : e.state === 'interrupted_for_safety' ? (
                  <>
                    <h2>Routine questions are paused</h2>
                    <p>
                      You can review your answers, correct a statement, or open
                      your report now. You can choose to request an estimate
                      after confirming the history.
                    </p>
                    <button
                      className="primary"
                      disabled={busy}
                      onClick={readHistory}
                    >
                      Review my history
                    </button>
                  </>
                ) : (
                  <>
                    {e.returnSource && safetyCheckDue(e, 'return') && (
                      <SafetyNet
                        point="return"
                        busy={busy}
                        onAnswer={(answer) => void act({ action: 'safety_check', category: 'return', status: answer })}
                      />
                    )}
                    <h2 aria-live="polite" aria-atomic="true" id="current-question" tabIndex={-1} ref={questionHeading}>
                      {e.question || 'Add to your history'}
                    </h2>
                    {e.question && (
                      <details className="why">
                        <summary>Why this question?</summary>
                        <p>
                          {e.questionSource?.reason ||
                            [...(e.turns || [])]
                              .reverse()
                              .find((t) => t.question === e.question)?.reason ||
                            'This question addresses missing information in your history.'}
                        </p>
                        {e.questionSource?.modelNote && (
                          <p className="model-note">
                            <strong>Model’s note (a suggestion, not checked by the app):</strong> {e.questionSource.modelNote}
                          </p>
                        )}
                        {e.questionSource?.documentId && (
                          <button
                            className="text-button"
                            onClick={() => {
                              setSource({
                                documentId: e.questionSource!.documentId,
                                rowId: e.questionSource!.rowId,
                              })
                              setTab('sources')
                            }}
                          >
                            Open the report passage
                          </button>
                        )}
                      </details>
                    )}
                    {e.scopeReason && (
                      <p>
                        You can record additional information for your visit
                        report.
                      </p>
                    )}
                    <form
                      onSubmit={(ev) => {
                        ev.preventDefault()
                        void addAnswer(answer)
                      }}
                    >
                      {setAside && (
                        <div className="callout set-aside" role="status">
                          <strong>The question changed while you were answering.</strong>
                          <p>
                            Your earlier words, written for “{setAside.question || 'the previous step'}”, are kept here and were not saved. Check them before you use them for this question.
                          </p>
                          <blockquote>{setAside.text}</blockquote>
                          <div className="actions">
                            <button type="button" className="secondary" onClick={() => { draft.restore(); setAnswerMode('text') }}>
                              Use these words for this question
                            </button>
                            <button type="button" className="secondary" onClick={draft.discard}>
                              Discard them
                            </button>
                          </div>
                        </div>
                      )}
                      {writeAnswer && (
                        <label>
                          Your answer
                          <textarea
                            ref={textarea}
                            maxLength={4000}
                            rows={2}
                            value={answer}
                            onChange={(ev) => {
                              setAnswerMode('text')
                              setAnswer(ev.target.value)
                            }}
                            placeholder={
                              e.questionSource?.answerHint ||
                              'A short answer in your own words is fine.'
                            }
                          />
                        </label>
                      )}
                      <div
                        className="choices"
                        role="group"
                        aria-labelledby="current-question"
                      >
                        {storyFirst && choices.length > 0 && (
                          <button
                            type="button"
                            onClick={() => setRevealedQuestion(questionKey)}
                          >
                            Show answer choices
                          </button>
                        )}
                        {!storyFirst &&
                          choices.map((choice) => (
                            <button
                              type="button"
                              key={choice}
                              className={
                                !ownWords && answer === choice ? 'chosen' : ''
                              }
                              aria-pressed={!ownWords && answer === choice}
                              onClick={() => {
                                setAnswerMode('choice')
                                setAnswer(choice)
                              }}
                            >
                              {choice}
                            </button>
                          ))}
                        {!storyFirst &&
                          (background || (e.demo && choices.length > 0) || !writeAnswer) && (
                            <button
                              type="button"
                              aria-pressed={ownWords}
                              className={ownWords ? 'chosen' : ''}
                              onClick={() => {
                                if (!ownWords) setAnswer('')
                                setAnswerMode('text')
                                textarea.current?.focus()
                              }}
                            >
                              {e.questionSource?.entryLabel ||
                                'Use my own words'}
                            </button>
                          )}
                      </div>
                      {!e.demo && (
                        <VoiceInput
                          onUse={(value) => {
                            setAnswerMode('text')
                            setAnswer(value)
                          }}
                        />
                      )}
                      {e.questionSource?.key === 'medication_start' && (
                        <label>
                          Actual first dose date, if known
                          <input
                            type="date"
                            value={eventDate}
                            onChange={(ev) => setEventDate(ev.target.value)}
                          />
                        </label>
                      )}
                      {writeAnswer && <ReadBack words={answer} />}
                      <div
                        className="choices response-options"
                        role="group"
                        aria-label="Other ways to answer"
                      >
                        {responseOptions.map(({ label, text }) => (
                          <button
                            type="button"
                            key={text}
                            aria-pressed={!ownWords && answer === text}
                            className={
                              !ownWords && answer === text ? 'chosen' : ''
                            }
                            onClick={() => {
                              setAnswer(text)
                              setAnswerMode('choice')
                            }}
                          >
                            {label}
                          </button>
                        ))}
                      </div>
                      <label className="check">
                        <input
                          type="checkbox"
                          checked={urgentNow}
                          onChange={(ev) => setUrgentNow(ev.target.checked)}
                        />
                        <span>
                          I may be having a medical emergency right now.
                        </span>
                      </label>
                      {urgentNow && (
                        <Callout
                          alert
                          title="Seek local emergency care immediately."
                        >
                          {emergency ||
                            'Do not wait for this interview to seek help.'}
                        </Callout>
                      )}
                      <div className="actions">
                        <button
                          className="primary"
                          disabled={busy || (!answer.trim() && !urgentNow)}
                        >
                          {urgentNow && !answer.trim()
                            ? 'Save emergency warning'
                            : 'Save answer'}
                        </button>
                        {!e.demo && !e.focused && (
                          <button
                            type="button"
                            className="secondary"
                            disabled={busy}
                            onClick={() =>
                              void act({ action: 'focus_interview' })
                            }
                          >
                            Switch to a short interview
                          </button>
                        )}
                        {!e.scopeReason && (
                          <button
                            type="button"
                            className="secondary"
                            disabled={busy}
                            onClick={readHistory}
                          >
                            Review history now
                          </button>
                        )}
                      </div>
                    </form>
                    {!activeWarnings.length && !e.scopeReason && !background && (
                        <ObservationPhotos
                          key={e.id}
                          e={e}
                          busy={busy}
                          run={run}
                          refresh={onUpdate}
                          act={act}
                        />
                      )}
                  </>
                )}
              </section>
            )}
            {tab === 'history' && (
              <>
                <div className="section-tools">
                  <button
                    className="secondary"
                    onClick={() => setTimeline(!timeline)}
                  >
                    {timeline
                      ? 'Hide earlier history'
                      : 'Reconcile earlier history'}
                  </button>
                </div>
                {timeline && (
                  <Timeline
                    current={e}
                    busy={busy}
                    act={async (c) => !!(await onAction(c))}
                  />
                )}
                <History
                  assessmentRequest={assessmentRequest}
                  e={e}
                  busy={busy || !!e.pending}
                  onAction={act}
                  focusFact={focusFact}
                  onSource={viewFact}
                />
              </>
            )}
            {tab === 'estimate' && (
              <section className="panel">
                <div className="topic-mark">
                  <BookOpenCheck size={19} /> Possible explanations
                </div>
                <h2>Provisional explanations</h2>
                <Notice text={e.notice} />
                <UrgencyReminder warnings={e.warnings} />
                {e.pending === 'assessment' ? null : current ? (
                  <>
                    <Estimate a={current} facts={e.facts} onFact={viewFact} />
                    <div className="actions">
                      <button
                        className="primary"
                        disabled={busy}
                        onClick={openReport}
                      >
                        Open visit report
                      </button>
                      {!e.scopeReason && (
                        <button
                          className="secondary"
                          disabled={busy}
                          onClick={showInterview}
                        >
                          Add information
                        </button>
                      )}
                    </div>
                  </>
                ) : (
                  <>
                    <p>
                      {stale
                        ? 'No estimate is available for the current history revision.'
                        : e.scopeReason
                          ? 'This encounter is outside the scope for an estimate.'
                          : 'Review and confirm your history before generating an estimate.'}
                    </p>
                    <button
                      className="secondary"
                      disabled={busy}
                      onClick={e.scopeReason ? openReport : readHistory}
                    >
                      {e.scopeReason
                        ? 'Open visit report'
                        : 'Open history review'}
                    </button>
                  </>
                )}
                <Revisions e={e} />
                <details className="archive">
                  <summary>Explore medical evidence</summary>
                  <EvidenceLookup
                    embedded
                    busy={busy}
                    run={run}
                    initial={current?.items?.[0]?.name || ''}
                  />
                </details>
                {e.assessments.length > 0 && (
                  <details>
                    <summary>Assessment versions</summary>
                    {e.assessments
                      .slice()
                      .reverse()
                      .map((a) => (
                        <details className="archive" key={a.version}>
                          <summary>
                            Version {a.version} ·{' '}
                            {a.historyRevision === e.historyRevision
                              ? 'For this history revision'
                              : 'Outdated'}{' '}
                            · {date(a.createdAt)}
                          </summary>
                          <Estimate a={a} facts={e.facts} onFact={viewFact} />
                        </details>
                      ))}
                  </details>
                )}
              </section>
            )}
            {tab === 'report' && (
              <>
                <div className="format-selection" aria-label="Report format">
                  <button
                    aria-pressed={format === 'user'}
                    onClick={() => setFormat('user')}
                  >
                    Your report
                  </button>
                  <button
                    aria-pressed={format === 'brief'}
                    disabled={busy}
                    onClick={() =>
                      run(async () => {
                        setBrief(
                          await api<Brief>('/encounters/' + e.id + '/brief'),
                        )
                        setFormat('brief')
                      })
                    }
                  >
                    Clinician brief
                  </button>
                  <button
                    aria-pressed={format === 'appendix'}
                    onClick={() => setFormat('appendix')}
                  >
                    Provenance appendix
                  </button>
                </div>
                <div className="report-actions">
                  <button className="primary" onClick={() => window.print()}>
                    <Printer size={17} />
                    Print / save PDF
                  </button>
                  <a
                    className="secondary"
                    href={'/api/encounters/' + e.id + '/export'}
                    download="healthctl.json"
                  >
                    <Download size={17} />
                    Download JSON
                  </a>
                </div>
                {format === 'brief' && brief ? (
                  <BriefBody b={brief} e={e} name={name} />
                ) : format === 'appendix' ? (
                  <Appendix e={e} name={name} />
                ) : (
                  <ReportBody e={report || e} name={name} />
                )}
                <Interop e={e} busy={busy} run={run} />
              </>
            )}
          </div>
          <aside className="record-aside">
            <div className="side-panel">
              <ClipboardList size={23} />
              <h3>Encounter details</h3>
              <dl>
                <dt>Age at intake</dt>
                <dd>{e.age} years</dd>
                <dt>Provider</dt>
                <dd>
                  {kinds[e.provider.kind]}
                  {e.provider.kind === 'chatgpt' && (
                    <>
                      <br />
                      <Destination p={e.provider} />
                    </>
                  )}
                </dd>
                <dt>Model</dt>
                <dd>{e.provider.model || 'Built-in synthetic case'}</dd>
                {!e.demo && <EncounterUsage id={e.id} revision={e.revision} />}
                <dt>History confirmation</dt>
                <dd>
                  {e.confirmedRevision === e.historyRevision
                    ? 'Confirmed by you'
                    : 'Awaiting confirmation'}
                </dd>
              </dl>
              <button className="text-button" onClick={onLeave}>
                <CirclePause size={16} />
                Pause and return
              </button>
            </div>
            <details className="manage">
              <summary>
                Manage encounter <ChevronDown size={15} />
              </summary>
              <h4>Current destination</h4>
              <p className="endpoint">
                <Destination p={e.provider} />
              </p>
              {canMove && !moving && (
                <button className="secondary" onClick={() => setMoving(true)}>
                  <Network size={16} />
                  Change provider
                </button>
              )}
              {moving && provider && (
                <div className="move-box">
                  <ContextBox p={provider} />
                  <label className="check">
                    <input
                      type="checkbox"
                      checked={consent}
                      onChange={(ev) => setConsent(ev.target.checked)}
                    />
                    <span>
                      I agree to send this encounter’s history to this
                      destination on the next request.
                    </span>
                  </label>
                  <button
                    className="primary"
                    disabled={busy || !consent}
                    onClick={() =>
                      void act({
                        action: 'change_provider',
                        provider,
                        confirmedDestination: true,
                      })
                    }
                  >
                    Confirm provider change
                  </button>
                  <button
                    className="text-button"
                    onClick={() => {
                      setMoving(false)
                      setConsent(false)
                    }}
                  >
                    Cancel
                  </button>
                </div>
              )}
              {e.providerChanges?.length > 0 && (
                <>
                  <h4>Provider changes</h4>
                  <ul>
                    {e.providerChanges.map((c, i) => (
                      <li key={i}>
                        <strong>{date(c.at, true)}</strong>
                        <div>
                          From {kinds[c.previous.kind]} · {c.previous.model}
                        </div>
                        <div className="endpoint">
                          <Destination p={c.previous} />
                        </div>
                        <div>
                          To {kinds[c.current.kind]} · {c.current.model}
                        </div>
                        <div className="endpoint">
                          <Destination p={c.current} />
                        </div>
                      </li>
                    ))}
                  </ul>
                </>
              )}
              {!deleting ? (
                <button
                  className="text-button danger"
                  onClick={() => setDeleting(true)}
                >
                  <Trash2 size={15} />
                  Delete encounter
                </button>
              ) : (
                <div className="delete-box">
                  <strong>Delete this encounter?</strong>
                  <p>
                    This removes its history and reports from primary storage.
                    Existing downloads and backups may retain copies.
                  </p>
                  <button
                    className="danger-button"
                    disabled={busy}
                    onClick={() => run(onDelete)}
                  >
                    Delete permanently
                  </button>
                  <button
                    className="text-button"
                    onClick={() => setDeleting(false)}
                  >
                    Cancel
                  </button>
                </div>
              )}
            </details>
          </aside>
        </div>
      </div>
      <div className="print-only">
        {format === 'brief' && brief ? (
          <BriefBody b={brief} e={e} name={name} />
        ) : format === 'appendix' ? (
          <Appendix e={e} name={name} />
        ) : (
          <ReportBody e={report || e} name={name} />
        )}
      </div>
    </>
  )
}
