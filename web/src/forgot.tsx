// ForgotPassword explains how to reset the owner password. healthctl runs on
// the owner's own computer and has no email, so a reset proves control of
// that computer: the desktop launcher or the operator command does it.
export function ForgotPassword() {
  return (
    <details className="forgot">
      <summary>Forgot your password?</summary>
      <p>
        There's no email reset, because healthctl runs on your own computer.
        Reset the password on the computer that runs it:
      </p>
      <ul>
        <li>
          <strong>Mac or Windows app:</strong> open the healthctl launcher and
          choose <strong>Reset owner password…</strong>
        </li>
        <li>
          <strong>Docker installation:</strong> in the healthctl folder, run{' '}
          <code>python3 scripts/maintenance.py recover-owner</code>
        </li>
      </ul>
      <p>
        A reset signs out every device, ends shared brief links and disconnects
        ChatGPT accounts. Your encounters, diary and uploads are kept.
      </p>
    </details>
  )
}
