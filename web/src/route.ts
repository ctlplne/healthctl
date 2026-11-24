// URL routes, so links, Back and reload keep the user where they were.
export type Page = 'journal' | 'settings' | 'intake' | 'record' | 'timeline' | 'evidence'
export type Route = { page: Page; encounterId?: string; tab?: string }

const pagePaths: Record<Exclude<Page, 'record'>, string> = {
  journal: '/',
  settings: '/settings',
  intake: '/new',
  timeline: '/timeline',
  evidence: '/library',
}
const tabs = ['interview', 'history', 'estimate', 'report', 'sources', 'followup', 'prep']
const id = /^[A-Za-z0-9_-]{1,128}$/

export function parseRoute(pathname: string): Route {
  const parts = pathname.split('/').filter(Boolean)
  if (parts[0] === 'encounters' && parts[1] && id.test(parts[1])) {
    return { page: 'record', encounterId: parts[1], tab: tabs.includes(parts[2] ?? '') ? parts[2] : undefined }
  }
  for (const [page, path] of Object.entries(pagePaths)) {
    if (path === '/' + (parts[0] ?? '') || (path === '/' && parts.length === 0)) return { page: page as Page }
  }
  return { page: 'journal' }
}

export function routePath(r: Route): string {
  if (r.page === 'record') {
    if (!r.encounterId) return '/'
    return '/encounters/' + encodeURIComponent(r.encounterId) + (r.tab && r.tab !== 'interview' ? '/' + r.tab : '')
  }
  return pagePaths[r.page]
}
