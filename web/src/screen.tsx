export type ScreenResult = {
  level: 'emergency' | 'check'
  quote: string
  message: string
  policyVersion: string
}

// ScreenAlert shows the application's warning for words that were just saved
// or dictated. The wording comes from the server's safety policy.
export function ScreenAlert({ warning }: { warning: ScreenResult }) {
  return (
    <div className={'callout screen-alert ' + warning.level} role="alert">
      <strong>{warning.level === 'check' ? 'Get this checked promptly' : 'Seek emergency care now'}</strong>
      <p>{warning.message}</p>
      <small>Matched words: “{warning.quote}” · policy {warning.policyVersion}</small>
    </div>
  )
}
