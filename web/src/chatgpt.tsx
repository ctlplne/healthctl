import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
} from 'react'
import type { ReactNode } from 'react'
import { api } from './api'
import type { Provider } from './types'
import { destination } from './types'
import { Brand, Callout } from './shared'
import type { Work } from './forms'

type Account = {
  id: string
  label: string
  connected: boolean
  planEnabled: boolean
}
type Connections = { availableHere: boolean; accounts: Account[] }
type Model = { slug: string; displayName: string }
const responsesURL = 'https://api.openai.com/v1/responses'
const planUsage = 'https://chatgpt.com/settings/usage'
const Accounts = createContext<{
  data: Connections | null
  error: string
  refresh: () => Promise<void>
}>({ data: null, error: '', refresh: async () => {} })

export function ChatGPTConnections({ children }: { children: ReactNode }) {
  const [data, setData] = useState<Connections | null>(null),
    [error, setError] = useState('')
  const latest = useRef(0)
  const refresh = useCallback(async () => {
    const key = ++latest.current
    try {
      const next = await api<Connections>('/chatgpt')
      if (key === latest.current) {
        setData(next)
        setError('')
      }
    } catch {
      if (key === latest.current) {
        setData(null)
        setError('ChatGPT account information could not be loaded.')
      }
    }
  }, [])
  useEffect(() => {
    void refresh()
    return () => {
      latest.current++
    }
  }, [refresh])
  return (
    <Accounts.Provider value={{ data, error, refresh }}>
      {children}
    </Accounts.Provider>
  )
}
function accountLabel(id: string, accounts: Account[]) {
  const label =
    accounts.find((a) => a.id === id)?.label || 'Saved ChatGPT account'
  return `${label} · ${id}`
}
export function Destination({ p }: { p: Provider }) {
  const { data } = useContext(Accounts)
  if (p.kind !== 'chatgpt') return <>{destination(p)}</>
  return (
    <>
      {responsesURL}
      {p.endpoint && (
        <>
          <span className="plan-screen-account">
            {' '}
            · {accountLabel(p.endpoint, data?.accounts || [])}
          </span>
          <span className="plan-print-account">
            {' '}
            · Registration {p.endpoint}
          </span>
        </>
      )}
    </>
  )
}
export function ManageUsage({ primary = false }: { primary?: boolean }) {
  return (
    <a
      className={primary ? 'primary' : 'text-button'}
      href={planUsage}
      target="_blank"
      rel="noreferrer"
    >
      Manage usage
    </a>
  )
}
export function PlanUsage() {
  return (
    <p className="plan-usage">
      <strong>Using ChatGPT plan</strong> · <ManageUsage />
    </p>
  )
}
export type PlanCallback = {
  state: string
  code: string
  clientId: string
  denied: boolean
  accessDenied: boolean
  malformed: boolean
}
export function captureChatGPTReturn(): PlanCallback | null {
  if (window.location.pathname !== '/auth/callback') return null
  const query = new URLSearchParams(window.location.search)
  window.history.replaceState(null, '', '/auth/callback')
  const one = (key: string) => query.get(key) || ''
  const malformed =
    ['state', 'code', 'client_id', 'error'].some(
      (key) => query.has(key) && (query.getAll(key).length !== 1 || !one(key)),
    ) ||
    (query.has('code') && query.has('error'))
  return {
    state: one('state'),
    code: one('code'),
    clientId: one('client_id'),
    denied: query.has('error'),
    accessDenied: one('error') === 'access_denied',
    malformed,
  }
}
function openLaunch(url: string) {
  const launch = new URL(url, window.location.origin)
  if (
    launch.origin !== window.location.origin ||
    !/^\/api\/chatgpt\/launch\/[a-zA-Z0-9_-]+$/.test(launch.pathname) ||
    launch.search ||
    launch.hash ||
    launch.username ||
    launch.password
  )
    throw new Error(
      'This installation could not open its ChatGPT sign-in route. Retry from Model settings.',
    )
  window.location.assign(launch.href)
}
function PlanWelcome({ done }: { done: () => void }) {
  const ref = useRef<HTMLDialogElement>(null)
  useEffect(() => {
    ref.current?.showModal()
  }, [])
  return (
    <dialog
      className="plan-welcome"
      ref={ref}
      aria-labelledby="welcome-title"
      onCancel={(ev) => {
        ev.preventDefault()
        done()
      }}
    >
      <p>{document.getElementById('preview-notice')?.textContent}</p>
      <aside role="note" aria-label="Model guidance" className="modal-guidance">
        {document.getElementById('model-guidance')?.textContent}
      </aside>
      <h2 id="welcome-title">You’re using your ChatGPT plan</h2>
      <p>
        Eligible requests you send with this connection use your plan or
        available credits. You can review usage and limits in ChatGPT.
      </p>
      <ManageUsage />
      <div className="actions">
        <button className="primary" onClick={done} autoFocus>
          Got it
        </button>
      </div>
    </dialog>
  )
}
export function ChatGPTReturn({
  callback,
  name,
  done,
}: {
  callback: PlanCallback
  name: string
  done: (id?: string) => void
}) {
  const [result, setResult] = useState<{
      account: Account
      firstPlanUse?: boolean
    } | null>(null),
    [error, setError] = useState('')
  useEffect(() => {
    let active = true
    if (
      callback.malformed ||
      !callback.state ||
      (!callback.denied && !callback.code)
    ) {
      setError(
        'This sign-in response is incomplete or has already been used. Open Model settings to reconnect.',
      )
      return
    }
    void api<
      | { account: Account; firstPlanUse?: boolean }
      | { retryAuthorizationUrl: string }
    >('/chatgpt/finish', 'POST', {
      state: callback.state,
      code: callback.code,
      clientId: callback.clientId,
      denied: callback.denied,
    })
      .then((r) => {
        if (!active) return
        if ('retryAuthorizationUrl' in r) openLaunch(r.retryAuthorizationUrl)
        else setResult(r)
      })
      .catch((err) => {
        if (active)
          setError(
            callback.accessDenied
              ? 'ChatGPT authorization ended without a new connection. You can choose a saved account or connect again in Model settings.'
              : callback.denied
                ? 'ChatGPT sign-in could not be completed. Return to Model settings to retry.'
                : err instanceof Error
                  ? err.message
                  : 'The ChatGPT connection could not be completed.',
          )
      })
    return () => {
      active = false
    }
  }, [callback])
  return (
    <div className="auth">
      <header className="auth-header">
        <Brand name={name} />
      </header>
      <main id="main-content" className="portal-content">
        <h1>ChatGPT connection</h1>
        {error ? (
          <>
            <Callout title="Connection incomplete" alert>
              {error}
            </Callout>
            <button className="secondary" onClick={() => done()}>
              Return to Model settings
            </button>
          </>
        ) : !result ? (
          <p role="status">Verifying the ChatGPT connection…</p>
        ) : (
          <>
            <p>
              <strong>{result.account.label}</strong> is connected to this
              installation.
            </p>
            <p>
              {result.account.planEnabled
                ? 'Plan permission is enabled. Select an available model and save the provider to use it for new encounters.'
                : 'The account is connected for identity only. Authorize plan use in Model settings before selecting a model.'}
            </p>
            {result.firstPlanUse && result.account.planEnabled ? (
              <PlanWelcome done={() => done(result.account.id)} />
            ) : (
              <button
                className="primary"
                onClick={() => done(result.account.id)}
              >
                Continue to Model settings
              </button>
            )}
          </>
        )}
      </main>
    </div>
  )
}

function Authorize({ account, busy, run }: { account?: Account } & Work) {
  const [enablePlan, setEnablePlan] = useState(
    !!(account?.connected && account.planEnabled),
  )
  const begin = async () => {
    const response = await api<{ authorizationUrl: string }>(
      '/chatgpt/start',
      'POST',
      { ...(account ? { accountId: account.id } : {}), enablePlan },
    )
    openLaunch(response.authorizationUrl)
  }
  return (
    <div className="plan-authorization">
      <label className="check">
        <input
          type="checkbox"
          checked={enablePlan}
          onChange={(ev) => setEnablePlan(ev.target.checked)}
        />
        <span>
          Allow eligible model requests from this installation to use my ChatGPT
          plan or credits.
        </span>
      </label>
      <button
        data-oai-siwc-button=""
        className="chatgpt-signin"
        type="button"
        disabled={busy}
        onClick={() => run(begin)}
      >
        Continue with ChatGPT
      </button>
      <p className="metadata">
        You’ll sign in at OpenAI. Your local owner account stays signed in here.
      </p>
    </div>
  )
}
export function PlanSettings({
  config,
  update,
  onReady,
  run,
  busy,
}: {
  config: Provider
  update: (fields: Partial<Provider>) => void
  onReady: (value: boolean) => void
} & Work) {
  const { data, error, refresh } = useContext(Accounts),
    account = data?.accounts.find((a) => a.id === config.endpoint)
  const [catalog, setCatalog] = useState<{
      account: string
      models: Model[]
    } | null>(null),
    [catalogError, setCatalogError] = useState(''),
    [attempt, setAttempt] = useState(0),
    [disconnect, setDisconnect] = useState(false),
    [revoked, setRevoked] = useState<boolean | null>(null)
  const usable = !!(
      data?.availableHere &&
      account?.connected &&
      account.planEnabled
    ),
    accountId = account?.id || ''
  useEffect(() => {
    let active = true
    setCatalog(null)
    setCatalogError('')
    setDisconnect(false)
    if (usable && accountId)
      void api<{ models: Model[] }>(
        '/chatgpt/accounts/' + encodeURIComponent(accountId) + '/models',
      )
        .then((r) => {
          if (active) setCatalog({ account: accountId, models: r.models || [] })
        })
        .catch((err) => {
          if (active)
            setCatalogError(
              err instanceof Error
                ? err.message
                : 'The models for this account could not be loaded.',
            )
        })
    return () => {
      active = false
    }
  }, [usable, accountId, attempt])
  const models = catalog?.account === accountId ? catalog.models : []
  const ready =
    usable && !!config.model && models.some((m) => m.slug === config.model)
  useEffect(() => {
    onReady(ready)
    return () => onReady(false)
  }, [ready, onReady])
  const end = async () => {
    const response = await api<{ revocationConfirmed: boolean }>(
      '/chatgpt/accounts/' + encodeURIComponent(accountId),
      'DELETE',
      {},
    )
    setRevoked(response.revocationConfirmed)
    setDisconnect(false)
    await refresh()
  }
  return (
    <section className="plan-settings">
      <h3>Use your ChatGPT plan</h3>
      <p>
        You’re signed in to this installation as its owner. Select a ChatGPT
        account to run eligible requests through OpenAI using that account’s
        plan limits and credits.
      </p>
      {error ? (
        <p role="alert">
          {error}{' '}
          <button
            type="button"
            className="text-button"
            onClick={() => void refresh()}
          >
            Retry account list
          </button>
        </p>
      ) : !data ? (
        <p role="status">Loading saved ChatGPT accounts…</p>
      ) : (
        <>
          <label>
            ChatGPT account
            <select
              value={config.endpoint}
              onChange={(ev) => {
                setRevoked(null)
                update({ endpoint: ev.target.value, model: '' })
              }}
            >
              <option value="">New ChatGPT account</option>
              {config.endpoint && !account && (
                <option value={config.endpoint}>
                  Unavailable saved account · {config.endpoint}
                </option>
              )}
              {data.accounts.map((a) => (
                <option value={a.id} key={a.id}>
                  {accountLabel(a.id, data.accounts)} ·{' '}
                  {a.connected
                    ? a.planEnabled
                      ? 'Plan enabled'
                      : 'Identity only'
                    : 'Disconnected'}
                </option>
              ))}
            </select>
          </label>
          {revoked !== null && (
            <p role="status">
              {revoked ? (
                'This ChatGPT account was disconnected.'
              ) : (
                <>
                  Disconnected locally. Remote revocation could not be
                  confirmed. Review this app’s access in ChatGPT settings.{' '}
                  <ManageUsage />
                </>
              )}
            </p>
          )}
          {usable && (
            <>
              <label>
                Available ChatGPT model
                <select
                  required
                  value={config.model}
                  onChange={(ev) => update({ model: ev.target.value })}
                  disabled={busy || !catalog || catalog.account !== accountId}
                >
                  <option value="">Choose a model</option>
                  {config.model &&
                    !models.some((m) => m.slug === config.model) && (
                      <option disabled value={config.model}>
                        Previously selected · {config.model}
                      </option>
                    )}
                  {models.map((m) => (
                    <option key={m.slug} value={m.slug}>
                      {m.displayName || m.slug}
                    </option>
                  ))}
                </select>
              </label>
              {catalogError ? (
                <p role="alert">
                  {catalogError}{' '}
                  <button
                    type="button"
                    className="text-button"
                    onClick={() => setAttempt((n) => n + 1)}
                  >
                    Retry model list
                  </button>
                </p>
              ) : !catalog ? (
                <p role="status">Loading models for this account…</p>
              ) : !models.length ? (
                <p>This account returned no selectable models.</p>
              ) : config.model && !ready ? (
                <p role="alert">
                  The saved model is unavailable for this account. Choose a
                  model to change the selection.
                </p>
              ) : null}
              <p className="metadata">
                This connection requests the highest recognized reasoning effort
                advertised by the selected model. When the model lists no
                supported effort, its default applies.
              </p>
              <PlanUsage />
            </>
          )}
          {!data.availableHere ? (
            <p className="hint">
              To connect or reauthorize, open this installation at{' '}
              <code>http://127.0.0.1</code> with its published port. A private
              SSH tunnel can forward the remote installation to a local port.
            </p>
          ) : config.endpoint && !account ? (
            <p>
              This saved account is unavailable. Choose a saved connection or
              select New ChatGPT account.
            </p>
          ) : account?.connected ? (
            <details className="archive">
              <summary>Reauthorize this ChatGPT account</summary>
              <Authorize
                key={account.id}
                account={account}
                busy={busy}
                run={run}
              />
            </details>
          ) : (
            <Authorize
              key={accountId || 'new'}
              account={account}
              busy={busy}
              run={run}
            />
          )}
          {account?.connected && (
            <>
              {disconnect ? (
                <div className="hint">
                  <div>
                    <strong>Disconnect {account.label}?</strong>
                    <p>
                      Encounters using this account will need a new
                      authorization before their next request.
                    </p>
                    <div className="actions">
                      <button
                        type="button"
                        className="secondary"
                        disabled={busy}
                        onClick={() => run(end)}
                      >
                        Confirm disconnect
                      </button>
                      <button
                        type="button"
                        className="text-button"
                        onClick={() => setDisconnect(false)}
                      >
                        Cancel
                      </button>
                    </div>
                  </div>
                </div>
              ) : (
                <button
                  type="button"
                  className="text-button danger"
                  disabled={busy}
                  onClick={() => setDisconnect(true)}
                >
                  Disconnect ChatGPT account
                </button>
              )}
            </>
          )}
        </>
      )}
    </section>
  )
}
