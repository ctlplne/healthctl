import { Diary, ReturnPicker, emptySelection } from './diary'
import { ReadBack, VoiceInput } from './voice'
import { useState } from 'react'
import type { FormEvent } from 'react'
import {
  BookOpenCheck,
  ClipboardList,
  FileText,
  FlaskConical,
  KeyRound,
  LoaderCircle,
  LockKeyhole,
  Network,
  Plus,
  Settings2,
  ShieldCheck,
} from 'lucide-react'
import { api, updateToken } from './api'
import { Destination, PlanSettings, ManageUsage } from './chatgpt'
import {
  Brand,
  Callout,
  ContextBox,
  DemoTag,
  Empty,
  Notice,
  PageTitle,
  PaperArt,
} from './shared'
import {
  date,
  destination,
  kinds,
  providerDefaults,
  sampleConcern,
  stateName,
} from './types'
import type {
  EncounterRow,
  Fields,
  Provider,
  ProviderKind,
  Session,
  Settings,
} from './types'

export type Run = (job: () => Promise<void>) => void
export type Work = { run: Run; busy: boolean }
export function Auth({
  session,
  run,
  busy,
  onLogin,
  error,
}: Work & { session: Session; onLogin: (s: Session) => void; error: string }) {
  const [username, setUsername] = useState(''),
    [password, setPassword] = useState('')
  const setup = session.setupRequired
  const submit = (e: FormEvent) => {
    e.preventDefault()
    run(async () => {
      const s = await api<Session>(setup ? '/setup' : '/login', 'POST', {
        username: username.trim(),
        password,
      })
      updateToken(s.csrf)
      setPassword('')
      onLogin(s)
    })
  }
  return (
    <div className="auth">
      <header className="auth-header">
        <Brand name={session.displayName} />
        <span className="auth-meta">
          <LockKeyhole size={14} />
          Self-hosted workspace
        </span>
      </header>
      <main id="main-content" className="auth-layout">
        <div className="auth-intro">
          <div className="auth-illustration">
            <PaperArt />
          </div>
          <p className="section-kicker">
            <span className="kicker-line" />
            Personal health record
          </p>
          <h1>
            Your clinical
            <br />
            <em>history.</em>
          </h1>
          <p className="intro-copy">
            Prepare for a healthcare visit with a guided symptom interview, a
            history you can correct, and a report you can bring with you.
          </p>
          <div className="intro-features">
            <span>
              <ClipboardList />
              Guided interview
            </span>
            <span>
              <FileText />
              Visit report
            </span>
          </div>
        </div>
        <section className="auth-form">
          <span className="icon-tile">
            <KeyRound />
          </span>
          <h2>{setup ? 'Create your owner account' : 'Welcome back'}</h2>
          <p>
            {setup
              ? 'This account manages the model connection and your saved encounters.'
              : 'Sign in to open your encounters and model settings.'}
          </p>
          <form onSubmit={submit}>
            <label>
              Username
              <input
                required
                autoComplete="username"
                minLength={setup ? 2 : 1}
                maxLength={80}
                value={username}
                onChange={(e) => setUsername(e.target.value)}
              />
            </label>
            <label>
              <span id="auth-password-label">Password</span>
              <input
                aria-labelledby="auth-password-label"
                aria-describedby={setup ? 'auth-password-hint' : undefined}
                type="password"
                required
                autoComplete={setup ? 'new-password' : 'current-password'}
                minLength={setup ? 12 : 1}
                maxLength={72}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
              {setup && (
                <small id="auth-password-hint">
                  Use at least 12 characters.
                </small>
              )}
            </label>
            {error && (
              <p className="form-error" role="alert">
                {error}
              </p>
            )}
            <button className="primary full" disabled={busy}>
              {setup ? 'Create owner account' : 'Sign in'}
            </button>
          </form>
          <div className="auth-notice">
            <Notice text={session.notice} />
            <a href="/privacy.html" target="_blank" rel="noreferrer">
              Where your data goes
            </a>
          </div>
        </section>
      </main>
    </div>
  )
}

export function Journal({
  rows,
  onOpen,
  onNew,
  onReport,
  onReturn,
  onDemo,
  onSettings,
  settings,
}: {
  rows: EncounterRow[]
  onOpen: (id: string) => void
  onNew: () => void
  onReturn: (entryId?: string) => void
  onDemo: () => void
  onReport: () => void
  onSettings: () => void
  settings: Settings
}) {
  const [diary, setDiary] = useState(false)
  if (diary) return <Diary onClose={() => setDiary(false)} onVisit={settings.provider?.kind && settings.provider.kind !== 'demo' ? onReturn : undefined} />
  const p = settings.provider
  return (
    <>
      <PageTitle
        title="Encounter journal"
        description="Your symptom interviews and visit reports."
      />
      <section className="journal-hero">
        <div>
          <span className="section-kicker">
            <span className="kicker-line" />
            Guided history
          </span>
          <h2>Start a new encounter</h2>
          <p>
            Describe your symptoms, review your history, and prepare a report
            for your next visit.
          </p>
          <button className="primary" onClick={p?.kind ? onNew : onSettings}>
            {p?.kind ? <Plus size={17} /> : <Settings2 size={17} />}{' '}
            {p?.kind ? 'New encounter' : 'Configure a provider'}
          </button>
          {!p?.kind && (
            <button className="secondary report-first-action" onClick={onDemo}>
              <FlaskConical size={17} /> Try the offline demo
            </button>
          )}
          {p?.kind && (
            <button
              className="secondary report-first-action"
              onClick={onReport}
            >
              <FileText size={17} />
              Start with a report
            </button>
          )}
          <button
            className="secondary report-first-action"
            onClick={() => setDiary(true)}
          >
            It happened again
          </button>
          {p?.kind && p.kind !== 'demo' && (
            <button
              className="secondary report-first-action"
              onClick={() => onReturn()}
            >
              Start a return visit
            </button>
          )}
        </div>
        <PaperArt />
      </section>
      <section className="section">
        <div className="section-heading">
          <h2>Saved encounters</h2>
          <span className="count">{rows.length}</span>
        </div>
        <div className="list-panel">
          {rows.length === 0 ? (
            <Empty>
              No encounters yet. Your saved interviews will appear here.
            </Empty>
          ) : (
            rows.map((row) => (
              <button
                key={row.id}
                className="journal-row"
                onClick={() => onOpen(row.id)}
              >
                <span className="row-icon">
                  <FileText size={20} />
                </span>
                <span className="row-copy">
                  <strong>{row.concern}</strong>
                  <small>
                    {date(row.updatedAt, true)}
                    {row.provider?.model ? ' · ' + row.provider.model : ''}
                  </small>
                </span>
                <span className="row-tags">
                  {(row.demo || row.provider?.kind === 'demo') && <DemoTag />}
                  <span className="tag">
                    {row.pending && <LoaderCircle size={13} className="spin" aria-hidden="true" />}
                    {row.pending ? 'Waiting for model' : stateName(row.state)}
                  </span>
                </span>
              </button>
            ))
          )}
        </div>
      </section>
    </>
  )
}

export function ProviderEditor({
  settings,
  busy,
  run,
  onSave,
  chatgptSelection,
}: Work & {
  settings: Settings
  onSave: (p: Provider) => Promise<void>
  chatgptSelection?: string
}) {
  const [config, setConfig] = useState<Provider>(
    chatgptSelection !== undefined
      ? settings.provider?.kind === 'chatgpt' &&
        settings.provider.endpoint === chatgptSelection
        ? settings.provider
        : {
            ...providerDefaults(),
            kind: 'chatgpt',
            endpoint: chatgptSelection,
            model: '',
          }
      : settings.provider?.kind
        ? settings.provider
        : providerDefaults(),
  )
  const [status, setStatus] = useState(''),
    [testError, setTestError] = useState(false),
    [planReady, setPlanReady] = useState(false)
  const update = (fields: Partial<Provider>) => {
    setConfig((p) => ({ ...p, ...fields }))
    setStatus('')
  }
  const changeKind = (kind: ProviderKind) => {
    setConfig({
      ...providerDefaults(),
      kind,
      model: kind === 'demo' ? 'synthetic' : '',
    })
    setStatus('')
  }
  const normalized = {
    ...config,
    model: config.model.trim(),
    endpoint:
      config.kind === 'compatible' || config.kind === 'chatgpt'
        ? config.endpoint.trim()
        : '',
  }
  const test = async () => {
    try {
      await api('/settings/test', 'POST', { provider: normalized })
      setTestError(false)
      setStatus(
        normalized.kind === 'demo'
          ? 'The built-in synthetic case is available.'
          : 'Connection verified with a nonclinical request.',
      )
    } catch (e) {
      setTestError(true)
      setStatus(e instanceof Error ? e.message : 'Connection test failed.')
    }
  }
  const submit = (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    if (config.kind === 'chatgpt' && !planReady) return
    const testOnly =
      (e.nativeEvent as SubmitEvent).submitter?.getAttribute('value') === 'test'
    run(testOnly ? test : () => onSave(normalized))
  }
  const creds = settings.credentialStatus?.[config.kind]
  return (
    <>
      <PageTitle
        title="Model settings"
        description="Choose where AI processing happens for new encounters, or try the scripted demo."
      />
      <p>
        <a href="/privacy.html" target="_blank" rel="noreferrer">
          Where your data goes when you connect a model
        </a>
      </p>
      <div className="settings-layout">
        <section className="panel form-panel">
          <h2>
            <Network size={21} /> Model connection
          </h2>
          <form onSubmit={submit}>
            <label>
              Provider
              <select
                value={config.kind}
                onChange={(e) => changeKind(e.target.value as ProviderKind)}
              >
                {Object.entries(kinds).map(([v, n]) => (
                  <option key={v} value={v}>
                    {n}
                  </option>
                ))}
              </select>
            </label>
            {config.kind === 'chatgpt' && (
              <PlanSettings
                config={config}
                update={update}
                onReady={setPlanReady}
                busy={busy}
                run={run}
              />
            )}
            {config.kind !== 'demo' && config.kind !== 'chatgpt' && (
              <>
                <label>
                  <span id="model-label">Model identifier</span>
                  <input
                    aria-labelledby="model-label"
                    aria-describedby="model-hint"
                    required
                    maxLength={150}
                    autoCapitalize="off"
                    spellCheck={false}
                    value={config.model}
                    onChange={(e) => update({ model: e.target.value })}
                  />
                  <small id="model-hint">
                    Use the model identifier supported by this provider.
                  </small>
                </label>
                {config.kind === 'compatible' && (
                  <label>
                    <span id="endpoint-label">Endpoint base URL</span>
                    <input
                      aria-labelledby="endpoint-label"
                      aria-describedby="endpoint-hint"
                      required
                      type="url"
                      maxLength={500}
                      autoCapitalize="off"
                      spellCheck={false}
                      placeholder="http://host.docker.internal:11434/v1"
                      value={config.endpoint}
                      onChange={(e) => update({ endpoint: e.target.value })}
                    />
                    <small id="endpoint-hint">
                      Use an exact base URL from the server’s endpoint
                      allowlist.
                    </small>
                  </label>
                )}
                <label>
                  Structured output
                  <select
                    value={config.structured || 'auto'}
                    onChange={(e) =>
                      update({
                        structured: e.target.value as Provider['structured'],
                      })
                    }
                  >
                    <option value="auto">Automatic</option>
                    <option value="native">Native JSON schema</option>
                    <option value="prompt">Constrained prompt</option>
                  </select>
                </label>
                {config.kind === 'anthropic' && (
                  <p className="metadata">
                    Claude consumer subscriptions are unsupported here. This
                    connection uses an Anthropic API credential.
                  </p>
                )}
                <div className="hint">
                  <KeyRound size={17} />
                  <div>
                    <strong>Server-side credential</strong>
                    <p>
                      {creds === true
                        ? 'Configured'
                        : creds === false
                          ? 'No credential configured'
                          : typeof creds === 'string'
                            ? creds
                            : 'Configured on the application server.'}{' '}
                      {config.kind === 'compatible'
                        ? 'Endpoints that require a key use BYOM_API_KEY and BYOM_KEY_ENDPOINT.'
                        : `Configure ${config.kind === 'openai' ? 'OPENAI_API_KEY' : 'ANTHROPIC_API_KEY'} on the server.`}
                    </p>
                  </div>
                </div>
              </>
            )}
            {config.kind === 'demo' && <ContextBox p={normalized} />}
            {config.kind === 'chatgpt' &&
              testError &&
              /limit|quota|usage unavailable/i.test(status) && (
                <ManageUsage primary />
              )}
            {status && (
              <p
                role="status"
                className={testError ? 'form-error' : 'form-success'}
              >
                {status}
              </p>
            )}
            <div className="actions">
              <button
                className="primary"
                disabled={busy || (config.kind === 'chatgpt' && !planReady)}
              >
                Save provider
              </button>
              <button
                className="secondary"
                value="test"
                disabled={busy || (config.kind === 'chatgpt' && !planReady)}
              >
                Test connection
              </button>
            </div>
          </form>
        </section>
        <aside className="side-panel">
          <ShieldCheck size={24} />
          <h3>Provider privacy</h3>
          <p>
            Existing encounters keep their original provider. Moving an
            encounter to a new destination requires your confirmation.
          </p>
          {config.kind !== 'demo' && (
            <>
              <h4>Selected destination</h4>
              <p>
                Your installation sends relevant history directly to the model
                here. The model suggests follow-up questions and can generate
                possible explanations at your request.
              </p>
              <code>
                {!destination(normalized) ? (
                  'Enter a base URL'
                ) : (
                  <Destination p={normalized} />
                )}
              </code>
            </>
          )}
          {config.kind === 'compatible' && (
            <>
              <h4>Allowed endpoints</h4>
              {(settings.allowlist || []).length ? (
                <ul>
                  {settings.allowlist.map((a) => (
                    <li key={a}>
                      <code>{a}</code>
                    </li>
                  ))}
                </ul>
              ) : (
                <p>
                  Set CLINICAL_ALLOWED_ENDPOINTS on the server to allow a
                  compatible endpoint.
                </p>
              )}
            </>
          )}
        </aside>
      </div>
    </>
  )
}

export function IntakeForm({
  diaryEntry,
  reportFirst,
  returnMode,
  rows,
  provider,
  session,
  busy,
  run,
  onCreate,
  onBack,
}: Work & {
  provider: Provider
  session: Session
  diaryEntry?: string
  returnMode: boolean
  rows: EncounterRow[]
  reportFirst: boolean
  onCreate: (body: Fields) => Promise<void>
  onBack: () => void
}) {
  const demo = provider.kind === 'demo'
  const [concern, setConcern] = useState(
      reportFirst ? '' : demo ? sampleConcern : '',
    ),
    [age, setAge] = useState(demo ? '35' : ''),
    [urgent, setUrgent] = useState(''),
    [pregnancy, setPregnancy] = useState(''),
    [ack, setAck] = useState(false)
  const [demoCase, setDemoCase] = useState(sampleConcern)
  const practice = demo && !reportFirst && !returnMode && demoCase === ''
  const [selection, setSelection] = useState(() => ({ ...emptySelection(), diaryIds: diaryEntry ? [diaryEntry] : [] })),
    [narrativeFirst, setNarrativeFirst] = useState(!demo),
    [focused, setFocused] = useState(false),
    [agenda, setAgenda] = useState('')
  const submit = (e: FormEvent) => {
    e.preventDefault()
    run(() =>
      onCreate({
        concern: concern.trim(),
        age: Number(age),
        guidedHistory: true,
        focused,
        narrativeFirst: practice || narrativeFirst,
        demoPractice: practice,
        agenda,
        ...(returnMode && (selection.source || selection.diaryIds.length)
          ? { returnVisit: selection }
          : {}),
        pregnancyRelated: pregnancy === 'yes',
        urgentNow: urgent === 'yes',
        noticeVersion: session.noticeVersion,
        disclosedProvider: provider,
        reportFirst,
      }),
    )
  }
  return (
    <>
      <PageTitle
        title={
          reportFirst
            ? 'Start with a report'
            : returnMode
              ? 'Return visit'
              : 'New encounter'
        }
        parent="Encounter journal"
        onBack={onBack}
        description="Describe your concern in English. The symptom interview covers adults aged 18 and older. You can still save a history for a child or a pregnancy-related concern."
      />
      <div className="intake-layout">
        <form className="panel form-panel" onSubmit={submit}>
          <h2>
            <ClipboardList size={22} /> About this encounter
          </h2>
          <ContextBox p={provider} />
          {demo && !reportFirst && !returnMode && (
            <label>
              Fictional interview example
              <select
                value={demoCase}
                onChange={(ev) => {
                  setDemoCase(ev.target.value)
                  setConcern(ev.target.value)
                }}
              >
                <option value="">Practice with your own fictional concern</option>
                <option value={sampleConcern}>Runny nose and sneezing</option>
                <option value="My fingernails have changed shape.">
                  Changing fingernails and daily activity
                </option>
              </select>
            </label>
          )}
          <label>
            {reportFirst
              ? 'What would you like to understand?'
              : 'Chief concern'}
            <textarea
              required
              maxLength={4000}
              rows={3}
              value={concern}
              onChange={(e) => {
                setConcern(e.target.value)
                if (demo && e.target.value !== demoCase) setDemoCase('')
              }}
              placeholder={
                reportFirst
                  ? 'For example, help me understand my routine screening results.'
                  : 'What changed or started bothering you? A short answer is enough to begin.'
              }
            />
          </label>
          {!demo && (
            <>
              <ReadBack words={concern} />
              <VoiceInput onUse={setConcern} />
              <label>
                What question do you want this visit to answer?
                <input
                  maxLength={600}
                  value={agenda}
                  onChange={(ev) => setAgenda(ev.target.value)}
                />
              </label>
              <label>
                Interview length
                <select
                  value={focused ? 'short' : 'full'}
                  onChange={(ev) => setFocused(ev.target.value === 'short')}
                >
                  <option value="full">Full interview</option>
                  <option value="short">
                    Short interview · up to two follow-up answers
                  </option>
                </select>
              </label>
              <label>
                Answer style
                <select
                  value={narrativeFirst ? 'story' : 'choices'}
                  onChange={(ev) =>
                    setNarrativeFirst(ev.target.value === 'story')
                  }
                >
                  <option value="story">My own words first</option>
                  <option value="choices">
                    Show answer choices immediately
                  </option>
                </select>
              </label>
            </>
          )}
          {returnMode && (
            <ReturnPicker rows={rows} value={selection} change={setSelection} />
          )}
          <label>
            Age in years
            <input
              className="short-input"
              type="number"
              required
              min={1}
              max={125}
              step={1}
              value={age}
              onChange={(e) => setAge(e.target.value)}
            />
          </label>
          <fieldset>
            <legend>Is the concern related to pregnancy?</legend>
            <Radios name="pregnancy" value={pregnancy} change={setPregnancy} />
          </fieldset>
          <fieldset>
            <legend>Are you reporting a possible emergency right now?</legend>
            <Radios name="urgent" value={urgent} change={setUrgent} />
          </fieldset>
          {urgent === 'yes' && (
            <Callout alert title="Seek local emergency care immediately.">
              {session.emergency ||
                'Do not wait for this interview to seek help.'}
            </Callout>
          )}
          <Notice text={session.notice} />
          <label className="check">
            <input
              required
              type="checkbox"
              checked={ack}
              onChange={(e) => setAck(e.target.checked)}
            />{' '}
            <span>
              I understand the experimental-use notice and where this
              encounter’s content will go.
            </span>
          </label>
          <div className="actions">
            <button className="primary" disabled={busy}>
              {reportFirst
                ? 'Create encounter and add report'
                : 'Start interview'}
            </button>
            <button className="secondary" type="button" onClick={onBack}>
              Cancel
            </button>
          </div>
        </form>
        <aside className="side-panel">
          <BookOpenCheck size={24} />
          <h3>Interview scope</h3>
          <p>
            Start with what brought you here. We’ll ask one question at a time
            about your concern and health background. You can choose an answer,
            use your own words, or skip.
          </p>
          {demo && (
            <>
              <h4>
                <FlaskConical size={16} /> Synthetic case
              </h4>
              <p>
                Follow a scripted example or practice telling a fictional story
                in your own words. Practice uses general questions about timing
                and daily life. Connect a model for personalized follow-ups.
              </p>
            </>
          )}
        </aside>
      </div>
    </>
  )
}
export function Radios({
  name,
  value,
  change,
}: {
  name: string
  value: string
  change: (v: string) => void
}) {
  return (
    <div className="radios">
      {['yes', 'no'].map((v) => (
        <label key={v}>
          <input
            type="radio"
            name={name}
            required
            checked={value === v}
            onChange={() => change(v)}
          />
          {v === 'yes' ? 'Yes' : 'No'}
        </label>
      ))}
    </div>
  )
}
