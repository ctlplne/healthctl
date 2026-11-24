import { useEffect, useRef, useState } from 'react'
import { SafetyBoundary } from './boundary'
import { InstallationSafety } from './safety'
import { createRoot } from 'react-dom/client'
import {
  BookOpen,
  CalendarDays,
  Library,
  CircleHelp,
  FolderOpen,
  Info,
  LoaderCircle,
  LockKeyhole,
  LogOut,
  Settings2,
  X,
} from 'lucide-react'
import { RegionContext } from './region'
import { Timeline } from './timeline'
import {
  ChatGPTConnections,
  ChatGPTReturn,
  captureChatGPTReturn,
} from './chatgpt'
import {
  capturePortalReturn,
  encounterSourceLink,
  linkedSourceEncounter,
  PortalReturn,
} from './ehr'
import { EvidenceLookup } from './evidence'
import { Operator } from './operator'
import { setLocale } from './types'
import type { Region } from './records'
import { api, RequestError, SIGNED_OUT, updateToken } from './api'
import { parseRoute, routePath, type Page } from './route'
import { schedule } from './poll'
import { Auth, IntakeForm, Journal, ProviderEditor } from './forms'
import { EncounterPage } from './encounter'
import { Brand, Notice } from './shared'
import type {
  Encounter,
  EncounterRow,
  Fields,
  Provider,
  Session,
  Settings,
} from './types'
import '@fontsource/geist/latin-400.css'
import '@fontsource/geist/latin-500.css'
import '@fontsource/geist/latin-600.css'
import '@fontsource/instrument-serif/latin-400.css'
import '@fontsource/instrument-serif/latin-400-italic.css'
import './styles.css'

// The route the app was opened at. An encounter link opens once signed in.
const startRoute = parseRoute(location.pathname)
function App() {
  const [session, setSession] = useState<Session | null>(null),
    [settings, setSettings] = useState<Settings | null>(null),
    [records, setRecords] = useState<EncounterRow[]>([]),
    [e, setEncounter] = useState<Encounter | null>(null)
  const [page, setPage] = useState<Page>(() =>
      new URLSearchParams(location.search).get('desktop') === 'settings'
        ? 'settings'
        : startRoute.page === 'record'
          ? 'journal'
          : startRoute.page,
    ),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(''),
    [pollError, setPollError] = useState(''),
    [notice, setNotice] = useState(false),
    [message, setMessage] = useState('')
  const [diaryEntry, setDiaryEntry] = useState<string>()
  const [returnMode, setReturnMode] = useState(false)
  const [reportFirst, setReportFirst] = useState(false),
    [region, setRegion] = useState<Region | null>(null)
  const [planReturn, setPlanReturn] = useState(captureChatGPTReturn),
    [planSelection, setPlanSelection] = useState<string>()
  const [portalReturn, setPortalReturn] = useState(capturePortalReturn)
  const [sourceEncounter, setSourceEncounter] = useState(linkedSourceEncounter)
  const locked = useRef(false),
    lastSubmission = useRef<{ key: string; id: string } | null>(null)
  const syncSession = async () => {
    const s = await api<Session>('/session')
    updateToken(s.csrf)
    setSession(s)
    document.title = s.displayName || 'healthctl'
  }
  const reloadWorkspace = async () => {
    const [config, list] = await Promise.all([
      api<Settings>('/settings'),
      api<{ encounters: EncounterRow[] }>('/encounters'),
    ])
    setSettings(config)
    setRecords(list.encounters || [])
  }
  const describe = (err: unknown) =>
    err instanceof Error ? err.message : 'The request could not be completed.'
  const guard = async <T,>(job: () => Promise<T>): Promise<T | undefined> => {
    if (locked.current) return
    locked.current = true
    setBusy(true)
    setError('')
    setMessage('')
    try {
      return await job()
    } catch (err) {
      if (
        err instanceof RequestError &&
        session?.authenticated &&
        err.status === 401
      ) {
        setEncounter(null)
        setSettings(null)
        setRecords([])
        setPage('journal')
        lastSubmission.current = null
        await syncSession().catch(() => setSession(null))
      }
      if (err instanceof RequestError && err.status === 409) {
        try {
          if (page === 'record' && e)
            setEncounter(await api<Encounter>('/encounters/' + e.id))
          await reloadWorkspace()
        } catch {
          /* Original conflict remains actionable. */
        }
      }
      setError(describe(err))
    } finally {
      locked.current = false
      setBusy(false)
    }
  }
  const run = (job: () => Promise<void>) => {
    void guard(job)
  }
  useEffect(() => {
    void syncSession().catch((err) => setError(describe(err)))
    const resume = (ev: PageTransitionEvent) => {
      if (ev.persisted) window.location.reload()
    }
    window.addEventListener('pageshow', resume)
    return () => window.removeEventListener('pageshow', resume)
  }, [])
  useEffect(() => {
    if (session?.authenticated)
      void api<Region>('/region')
        .then((r) => {
          setRegion(r)
          setLocale(r.locale)
        })
        .catch(() => setRegion(null))
    if (session?.authenticated)
      void reloadWorkspace().catch((err) => setError(describe(err)))
  }, [session?.authenticated])
  useEffect(() => {
    if (
      !session?.authenticated ||
      !sourceEncounter ||
      portalReturn ||
      e?.id === sourceEncounter
    )
      return
    let active = true
    void api<Encounter>('/encounters/' + encodeURIComponent(sourceEncounter))
      .then((next) => {
        if (active) {
          setEncounter(next)
          setPage('record')
        }
      })
      .catch((err) => {
        if (active) {
          setError(describe(err))
          setSourceEncounter(undefined)
          window.history.replaceState(null, '', '/')
        }
      })
    return () => {
      active = false
    }
  }, [session?.authenticated, sourceEncounter, portalReturn, e?.id])
  useEffect(() => {
    window.scrollTo(0, 0)
  }, [page])
  // Keep the URL in step with the screen, so links, Back and reload work.
  useEffect(() => {
    if (page === 'record' && !e) return
    const path = routePath({ page, encounterId: e?.id })
    if (!location.hash && location.pathname !== path && !location.pathname.startsWith(path + '/')) {
      window.history.pushState(null, '', path + location.search)
    }
  }, [page, e?.id])
  useEffect(() => {
    const follow = () => {
      const r = parseRoute(location.pathname)
      if (r.page === 'record' && r.encounterId) {
        if (r.encounterId !== e?.id) open(r.encounterId)
      } else if (r.page !== page) move(r.page)
    }
    window.addEventListener('popstate', follow)
    return () => window.removeEventListener('popstate', follow)
  })
  const startOpened = useRef(false)
  useEffect(() => {
    if (session?.authenticated && startRoute.page === 'record' && startRoute.encounterId && !startOpened.current) {
      startOpened.current = true
      open(startRoute.encounterId)
    }
  }, [session?.authenticated])
  // An ended session asks the user to sign in again; saved history is safe.
  useEffect(() => {
    const signedOut = () => {
      setSession((s) => (s ? { ...s, authenticated: false } : s))
      setEncounter(null)
      setError('Your session ended. Sign in again; your saved history is safe.')
    }
    window.addEventListener(SIGNED_OUT, signedOut)
    return () => window.removeEventListener(SIGNED_OUT, signedOut)
  }, [])
  useEffect(() => {
    setPollError('')
    if (page !== 'record' || !session?.authenticated || !e?.pending) return
    const { id, revision } = e
    const controller = new AbortController()
    let stop = () => {}
    let attempt = 0
    const check = async () => {
      let failed = false
      try {
        const next = await api<Encounter>(
          '/encounters/' + id,
          'GET',
          undefined,
          {
            signal: AbortSignal.any([
              controller.signal,
              AbortSignal.timeout(10000),
            ]),
          },
        )
        if (controller.signal.aborted) return
        setPollError('')
        if (next.revision > revision || !next.pending) {
          setEncounter((current) =>
            current?.id === id && current.revision === revision
              ? next
              : current,
          )
          return
        }
      } catch (err) {
        if (controller.signal.aborted) return
        if (err instanceof RequestError && err.status === 401) {
          setEncounter(null)
          setPage('journal')
          setRecords([])
          setSettings(null)
          lastSubmission.current = null
          await syncSession().catch(() => setSession(null))
          return
        }
        if (err instanceof RequestError && err.status === 404) {
          setEncounter(null)
          setPage('journal')
          setError('This encounter is no longer available.')
          void reloadWorkspace().catch(() => {})
          return
        }
        setPollError(
          'Your history is saved. Checking the request status again automatically.',
        )
        failed = true
      }
      // Wait longer the longer the request runs, and pause while hidden.
      attempt += failed ? 2 : 1
      if (!controller.signal.aborted) stop = schedule(controller.signal, attempt, () => void check())
    }
    stop = schedule(controller.signal, -3, () => void check())
    return () => {
      controller.abort()
      stop()
    }
  }, [page, session?.authenticated, e?.id, e?.revision, e?.pending])
  const waitingInJournal = page === 'journal' && session?.authenticated && records.some(row => row.pending)
  useEffect(() => {
    if (!waitingInJournal) return
    const controller = new AbortController()
    let stop = () => {}
    let attempt = 0
    const refresh = async () => {
      try {
        const list = await api<{ encounters: EncounterRow[] }>('/encounters', 'GET', undefined, { signal: controller.signal })
        if (!controller.signal.aborted) setRecords(list.encounters)
      } catch {
        // A temporary connection failure leaves the saved journal in view.
      } finally {
        if (!controller.signal.aborted) stop = schedule(controller.signal, ++attempt, () => void refresh(), 2500)
      }
    }
    stop = schedule(controller.signal, 0, () => void refresh(), 2500)
    return () => {
      controller.abort()
      stop()
    }
  }, [waitingInJournal])
  const leaveSources = () => {
    if (sourceEncounter || portalReturn)
      window.history.replaceState(null, '', '/')
    setSourceEncounter(undefined)
    setPortalReturn(null)
  }
  const returnToSources = (id?: string, next?: Encounter) => {
    setPortalReturn(null)
    setError('')
    setSourceEncounter(id)
    setEncounter(next || null)
    setPage(next ? 'record' : 'journal')
    window.history.replaceState(null, '', id ? encounterSourceLink(id) : '/')
  }
  const requestId = (path: string, body: Fields) => {
    const key = JSON.stringify([path, body])
    if (key !== lastSubmission.current?.key)
      lastSubmission.current = { key, id: crypto.randomUUID() }
    return lastSubmission.current!.id
  }
  const save = async (path: string, body: Fields) => {
    const next = await api<Encounter>(
      path,
      'POST',
      {
        ...body,
        requestId: requestId(path, body),
      },
      { asyncModel: true, signal: AbortSignal.timeout(20000) },
    )
    lastSubmission.current = null
    setEncounter(next)
    setPage('record')
    return next
  }
  const action = (cmd: Fields) =>
    guard(() =>
      save('/encounters/' + e!.id + '/actions', {
        ...cmd,
        revision: e!.revision,
      }),
    )
  const move = (p: Page) => {
    if (locked.current) return
    leaveSources()
    setPlanSelection(undefined)
    setPage(p)
    setError('')
    setMessage('')
    setNotice(false)
    setEncounter(null)
    if (p === 'journal') run(reloadWorkspace)
  }
  const storeProvider = async (p: Provider) => {
    const s = await api<Settings>('/settings', 'PUT', { provider: p })
    setPlanSelection(undefined)
    setSettings(s)
    setPage('journal')
    setMessage('Provider saved for new encounters.')
  }
  const open = (id: string) =>
    run(async () => {
      leaveSources()
      setEncounter(await api<Encounter>('/encounters/' + id))
      setPage('record')
    })
  const remove = async () => {
    await api('/encounters/' + e!.id, 'DELETE', { revision: e!.revision })
    setEncounter(null)
    setPage('journal')
    leaveSources()
    await reloadWorkspace()
    setMessage('Encounter deleted.')
  }
  const logout = () =>
    run(async () => {
      const s = await api<Session>('/logout', 'POST', {})
      leaveSources()
      updateToken(s.csrf)
      setSession(s)
      setPage('journal')
      setSettings(null)
      setRecords([])
      setEncounter(null)
      setNotice(false)
      lastSubmission.current = null
    })
  if (!session)
    return (
      <main className="loading">
        <Brand name="healthctl" />
        {error ? (
          <>
            <p role="alert">{error}</p>
            <button className="secondary" onClick={() => run(syncSession)}>
              Reconnect
            </button>
          </>
        ) : (
          <p role="status">Opening your workspace…</p>
        )}
      </main>
    )
  if (!session.authenticated)
    return (
      <>
        <a href="#main-content" className="skip">
          Skip to content
        </a>
        <Auth
          session={session}
          busy={busy}
          run={run}
          error={error}
          onLogin={setSession}
        />
      </>
    )
  if (planReturn)
    return (
      <>
        <a href="#main-content" className="skip">
          Skip to content
        </a>
        <ChatGPTReturn
          callback={planReturn}
          name={session.displayName}
          done={(id) => {
            window.history.replaceState(null, '', '/')
            setPlanReturn(null)
            setPlanSelection(id)
            setPage('settings')
          }}
        />
      </>
    )
  if (portalReturn)
    return (
      <>
        <a href="#main-content" className="skip">
          Skip to content
        </a>
        <PortalReturn
          callback={portalReturn}
          name={session.displayName}
          onExit={(id) => returnToSources(id)}
          onAttach={(record) => returnToSources(record.id, record)}
        />
      </>
    )
  return (
    <ChatGPTConnections>
      <div className="app-shell">
        <a href="#main-content" className="skip">
          Skip to content
        </a>
        <aside className="sidebar">
          <Brand name={session.displayName} />
          <div className="workspace-meta">
            <span className="workspace-icon">
              <LockKeyhole size={16} />
            </span>
            <div>
              <strong>Personal workspace</strong>
              <small>Self-hosted installation</small>
            </div>
          </div>
          <nav className="main-nav" aria-label="Workspace">
            <button
              className={
                ['record', 'intake', 'journal'].includes(page) ? 'selected' : ''
              }
              aria-current={
                ['record', 'intake', 'journal'].includes(page)
                  ? 'page'
                  : undefined
              }
              disabled={busy}
              onClick={() => move('journal')}
            >
              <FolderOpen size={19} />
              Encounter journal
            </button>
            <button
              className={page === 'timeline' ? 'selected' : ''}
              aria-current={page === 'timeline' ? 'page' : undefined}
              disabled={busy}
              onClick={() => move('timeline')}
            >
              <CalendarDays size={19} />
              Personal timeline
            </button>
            <button
              className={page === 'evidence' ? 'selected' : ''}
              aria-current={page === 'evidence' ? 'page' : undefined}
              disabled={busy}
              onClick={() => move('evidence')}
            >
              <Library size={19} />
              Medical evidence
            </button>
            <button
              className={page === 'settings' ? 'selected' : ''}
              aria-current={page === 'settings' ? 'page' : undefined}
              disabled={busy}
              onClick={() => move('settings')}
            >
              <Settings2 size={19} />
              Model settings
            </button>
          </nav>
          <div className="rail-bottom">
            <button className="rail-link" onClick={() => setNotice((v) => !v)}>
              <CircleHelp size={17} />
              Experimental-use notice
            </button>
            <button className="rail-link" onClick={logout} disabled={busy}>
              <LogOut size={17} />
              Sign out
            </button>
          </div>
        </aside>
        <div className="workspace-body">
          <header className="topbar">
            <span>
              <BookOpen size={16} /> Personal workspace
            </span>
            <button onClick={() => setNotice((v) => !v)} aria-expanded={notice}>
              <Info size={15} />
              Experimental · Unreviewed
            </button>
          </header>
          <main id="main-content" tabIndex={-1} className="content">
            <SafetyBoundary warnings={e?.warnings || []} resetKey={page + ':' + (e?.id || '')}>
            {notice && (
              <div className="notice-disclosure">
                <Notice text={session.notice} />
                <button
                  onClick={() => setNotice(false)}
                  className="icon-btn"
                  aria-label="Close notice"
                >
                  <X size={16} />
                </button>
              </div>
            )}
            {busy && (
              <div className="progress-status" role="status">
                <LoaderCircle size={16} className="spin" />
                Working…
              </div>
            )}
            {error && (
              <div className="error-banner" role="alert">
                <Info size={18} />
                <span>{error}</span>
                <button
                  onClick={() => setError('')}
                  className="icon-btn"
                  aria-label="Dismiss error"
                >
                  <X size={16} />
                </button>
              </div>
            )}
            {message && (
              <p className="form-success" role="status">
                {message}
              </p>
            )}
            {!settings ? (
              <section>
                {error ? (
                  <button
                    className="secondary"
                    onClick={() => run(reloadWorkspace)}
                  >
                    Reload workspace
                  </button>
                ) : (
                  <p role="status">Loading your workspace…</p>
                )}
              </section>
            ) : page === 'timeline' ? (
              <Timeline busy={busy} onOpen={open} />
            ) : page === 'evidence' ? (
              <EvidenceLookup busy={busy} run={run} />
            ) : page === 'settings' ? (
              <>
                <ProviderEditor
                  chatgptSelection={planSelection}
                  settings={settings}
                  busy={busy}
                  run={run}
                  onSave={storeProvider}
                />
                <Operator busy={busy} run={run} region={region} />
              </>
            ) : page === 'intake' && settings.provider?.kind ? (
              <IntakeForm
                rows={records}
                diaryEntry={returnMode ? diaryEntry : undefined}
                returnMode={returnMode}
                reportFirst={reportFirst}
                provider={settings.provider}
                session={session}
                busy={busy}
                run={run}
                onCreate={async (b) => {
                  await save('/encounters', b)
                }}
                onBack={() => move('journal')}
              />
            ) : page === 'record' && e ? (
              <RegionContext.Provider value={region}>
                <EncounterPage
                  key={e.id}
                  e={e}
                  pollError={pollError}
                  startAtSources={sourceEncounter === e.id}
                  initialTab={startRoute.encounterId === e.id ? startRoute.tab : undefined}
                  onTab={(tab) => {
                    if (!location.hash) window.history.replaceState(null, '', routePath({ page: 'record', encounterId: e.id, tab }) + location.search)
                  }}
                  name={session.displayName}
                  emergency={session.emergency}
                  assessmentNotice={session.assessmentNotice}
                  provider={settings.provider}
                  busy={busy}
                  run={run}
                  onAction={action}
                  onUpdate={(next) =>
                    setEncounter((current) =>
                      current?.id === next.id &&
                      next.revision >= current.revision
                        ? next
                        : current,
                    )
                  }
                  careDestination={region?.careDestination || ''}
                  onLeave={() => move('journal')}
                  onDelete={remove}
                />
              </RegionContext.Provider>
            ) : (
              <>
              <InstallationSafety />
              <Journal
                rows={records}
                settings={settings}
                onOpen={open}
                onDemo={() => run(async () => {
                  await storeProvider({ kind: 'demo', model: 'synthetic', endpoint: '', structured: 'auto' })
                  setReturnMode(false)
                  setReportFirst(false)
                  setPage('intake')
                })}
                onReturn={(entryId) => {
                  setDiaryEntry(entryId)
                  setReturnMode(true)
                  setReportFirst(false)
                  move('intake')
                }}
                onNew={() => {
                  setReturnMode(false)
                  setReportFirst(false)
                  move('intake')
                }}
                onReport={() => {
                  setReturnMode(false)
                  setReportFirst(true)
                  move('intake')
                }}
                onSettings={() => move('settings')}
              />
              </>
            )}
            </SafetyBoundary>
          </main>
          <footer className="workspace-footer">
            <span>{session.displayName}</span>
            <a href="/privacy.html" target="_blank" rel="noreferrer">Where your data goes</a>
          </footer>
        </div>
      </div>
    </ChatGPTConnections>
  )
}
const banner = document.getElementById('installation-guidance')!
const measureBanner = () => {
  document.documentElement.style.setProperty(
    '--guidance-height',
    `${banner.offsetHeight}px`,
  )
}
measureBanner()
new ResizeObserver(measureBanner).observe(banner)
createRoot(document.getElementById('root')!).render(<App />)
