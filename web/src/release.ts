// The app never contacts a server to look for updates. It knows when this
// version was released and, after a while, reminds the owner to check the
// release page themselves.
export const RELEASE = { version: '0.2.0', date: '2026-11-24', page: 'https://github.com/ctlplne/healthctl/releases' }
export const REMIND_AFTER_DAYS = 90

export function updateNotice(now: Date, release = RELEASE): string {
  const released = new Date(release.date + 'T00:00:00Z')
  const days = Math.floor((now.getTime() - released.getTime()) / 86_400_000)
  if (!(days >= REMIND_AFTER_DAYS)) return ''
  const months = Math.floor(days / 30)
  return 'This version (' + release.version + ') is about ' + months + ' months old. healthctl never checks for updates by itself; see the release page for newer versions and safety fixes.'
}
