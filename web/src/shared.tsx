import type { ReactNode } from 'react'
import {
  BookOpen,
  ChevronLeft,
  ClipboardList,
  FileText,
  FlaskConical,
  Info,
  LockKeyhole,
  Plus,
  Stethoscope,
  TriangleAlert,
} from 'lucide-react'
import type { Provider } from './types'
import { destination, kinds } from './types'
export function Brand({ name }: { name: string }) {
  return (
    <div className="brand">
      <span className="brandmark" aria-hidden="true">
        <BookOpen size={24} />
        <Plus size={11} strokeWidth={3} />
      </span>
      <span>{name || 'healthctl'}</span>
    </div>
  )
}
export function PageTitle({
  title,
  description,
  parent,
  onBack,
}: {
  title: string
  description?: string
  parent?: string
  onBack?: () => void
}) {
  return (
    <header className="page-title">
      {onBack && (
        <button className="back" onClick={onBack}>
          <ChevronLeft size={16} />
          {parent || 'Encounter journal'}
        </button>
      )}
      <h1>{title}</h1>
      {description && <p>{description}</p>}
    </header>
  )
}
export function Notice({ text }: { text: string }) {
  return (
    <aside className="notice">
      <Info size={19} />
      <div>
        <strong>Experimental AI assessment</strong>
        <p>
          {text
            .replace(/^Experimental AI assessment\s*[—–-]\s*/i, '')
            .replace(/^not reviewed/, 'Not reviewed')}
        </p>
      </div>
    </aside>
  )
}
export function DemoTag() {
  return (
    <span className="tag demo">
      <FlaskConical size={13} />
      Synthetic demo
    </span>
  )
}
export function ContextBox({ p }: { p: Provider }) {
  return (
    <div className="context-box">
      {p.kind === 'demo' ? <FlaskConical /> : <LockKeyhole />}
      <div>
        <strong>
          {p.kind === 'demo' ? 'Synthetic case only' : 'Interview destination'}
        </strong>
        <p>
          {p.kind === 'demo' ? (
            'Use the fictional case to try the workflow. This mode uses scripted examples and sends no model requests.'
          ) : (
            <>
              Your history will be sent to <b>{kinds[p.kind]}</b> at{' '}
              <span className="endpoint">{destination(p)}</span> using{' '}
              <b>{p.model}</b>.
            </>
          )}
        </p>
      </div>
    </div>
  )
}
export function Callout({
  title,
  children,
  alert = false,
}: {
  title: string
  children?: ReactNode
  alert?: boolean
}) {
  return (
    <aside
      className={'callout ' + (alert ? 'alert' : '')}
      role={alert ? 'alert' : undefined}
    >
      {alert ? <TriangleAlert size={20} /> : <Info size={20} />}
      <div>
        <strong>{title}</strong>
        {children && <div>{children}</div>}
      </div>
    </aside>
  )
}
export function PaperArt() {
  return (
    <div className="paper-art" aria-hidden="true">
      <div className="art-grid" />
      <div className="art-ring one" />
      <div className="art-ring two" />
      <div className="folio backsheet" />
      <div className="folio frontsheet">
        <div className="folio-head">
          <span className="mini-mark">+</span>
          <span />
        </div>
        <div className="paper-rule" />
        <div className="paper-rule short" />
        <div className="folio-notes">
          <ClipboardList size={23} />
          <i />
          <i />
        </div>
        <div className="paper-rule small" />
      </div>
      <div className="art-seal">
        <Stethoscope size={27} strokeWidth={1.5} />
      </div>
    </div>
  )
}
export function Empty({ children }: { children: ReactNode }) {
  return (
    <div className="empty">
      <FileText size={24} />
      <p>{children}</p>
    </div>
  )
}
