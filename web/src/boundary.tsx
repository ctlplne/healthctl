import { Component, type ErrorInfo, type ReactNode } from 'react'
import type { Warning } from './types'

type Props = { warnings: Warning[]; resetKey: string; children: ReactNode }
type State = { failed: boolean; key: string }

// SafetyBoundary keeps active urgent warnings on screen even if the page that
// would normally show them fails to render. It logs nothing, so no clinical
// content reaches the console.
export class SafetyBoundary extends Component<Props, State> {
  state: State = { failed: false, key: this.props.resetKey }

  static getDerivedStateFromError(): Partial<State> {
    return { failed: true }
  }

  static getDerivedStateFromProps(props: Props, state: State): Partial<State> | null {
    return props.resetKey !== state.key ? { failed: false, key: props.resetKey } : null
  }

  componentDidCatch(_error: Error, _info: ErrorInfo) {}

  render() {
    if (!this.state.failed) return this.props.children
    const active = this.props.warnings.filter((w) => w.active)
    return (
      <section className="panel safety-fallback">
        {active.map((w) => (
          <div key={w.id} className="callout alert" role="alert">
            <strong>{w.level === 'check' ? 'Get this checked promptly' : 'Active urgent warning'}</strong>
            <p>{w.message}</p>
          </div>
        ))}
        <h2>This page could not be shown</h2>
        <p>
          Your history is saved. Reload to try again. If you may be having an
          emergency, seek local emergency care immediately.
        </p>
        <button type="button" className="primary" onClick={() => window.location.reload()}>
          Reload
        </button>
      </section>
    )
  }
}
