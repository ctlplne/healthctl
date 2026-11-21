import { test, expect, type Page, type Route } from '@playwright/test'
import type { Provider, Encounter } from '../src/types'

// The server/portal integration suite exercises the protocol. These deterministic
// browser fixtures cover decline, recovery and selection without a remote portal.
const id = 'opaque-synthetic-ehr'
const provider: Provider = {
  kind: 'demo',
  model: 'synthetic',
  structured: 'auto',
  endpoint: '',
}
const encounter: Encounter = {
  id,
  revision: 1,
  historyRevision: 1,
  confirmedRevision: 0,
  state: 'interviewing',
  provider,
  demo: true,
  reportFirst: true,
  concern: 'Synthetic lab review',
  background: '',
  pregnancyRelated: false,
  notice: 'Fictional notice.',
  noticeVersion: 'test',
  question: '',
  choices: [],
  failure: '',
  scopeReason: '',
  pending: '',
  providerChanges: [],
  age: 35,
  answers: [],
  facts: [],
  warnings: [],
  assessments: [],
  contradictions: [],
  outstanding: [],
  documents: [],
  reportReviews: [],
  outcomes: [],
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
}
const lab = {
  key: 'selected',
  label: 'Example assay',
  value: '<5',
  unit: 'mg/L',
  referenceRange: '1–6',
  flag: 'HH',
  critical: true,
  date: '2026-10-02',
  issued: '2026-10-04',
  source: 'Observation/test-example',
  status: 'final',
  version: '2',
  panel: 'Fictional panel',
  context: ['Source qualifier retained'],
}
type Options = {
  preview: (r: Route) => Promise<void>
  result?: (r: Route) => Promise<void>
  cancel?: (r: Route) => Promise<void>
}
async function localFixture(page: Page, { preview, result, cancel }: Options) {
  const calls: string[] = []
  await page.route('**/api/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    calls.push(path)
    if (path === '/api/ehr/preview') return preview(route)
    if (path === '/api/ehr/import' && result) return result(route)
    if (path === '/api/ehr/cancel' && cancel) return cancel(route)
    const json = (
      {
        '/api/session': {
          authenticated: true,
          setupRequired: false,
          csrf: 'synthetic-csrf',
          displayName: 'healthctl',
          notice: 'Fictional notice.',
          noticeVersion: 'test',
          emergency: '',
        },
        '/api/settings': { provider, allowlist: [], credentialStatus: {} },
        '/api/encounters': { encounters: [] },
        '/api/region': { locale: 'en-US', units: 'as-reported' },
        '/api/ehr': { enabled: false },
        ['/api/encounters/' + id]: encounter,
      } as Record<string, unknown>
    )[path]
    if (json) return route.fulfill({ json })
    return route.fulfill({
      status: 404,
      json: { error: 'Unknown synthetic route' },
    })
  })
  return calls
}
const saved = {
  ticket: 'temporary-test-ticket',
  encounterId: id,
  name: 'Fictional portal',
  rows: [
    lab,
    {
      ...lab,
      key: 'excluded',
      label: 'Unselected assay',
      flag: 'H',
      critical: false,
    },
  ],
  more: true,
  skipped: 1,
  skippedByReason: { unsupported_value: 1 },
}

test('Portal review scrubs the callback, retains qualifiers, and attaches only confirmed selected rows', async ({
  page,
}) => {
  const calls = await localFixture(page, {
    preview: async (r) => {
      expect(r.request().headers()['x-csrf-token']).toBe('synthetic-csrf')
      expect(r.request().postDataJSON()).toEqual({
        code: 'fake-code',
        state: 'fake-state',
        issuer: 'https://test.invalid/fhir',
        denied: false,
      })
      await r.fulfill({ json: saved })
    },
    result: async (r) => {
      expect(r.request().postDataJSON()).toEqual({
        ticket: saved.ticket,
        keys: ['selected'],
        confirmedPatient: true,
      })
      await r.fulfill({ json: encounter })
    },
  })
  await page.goto(
    '/ehr/callback?code=fake-code&state=fake-state&iss=https%3A%2F%2Ftest.invalid%2Ffhir',
  )
  await expect(
    page.getByRole('heading', { name: 'Choose lab results' }),
  ).toBeVisible()
  expect(new URL(page.url()).search).toBe('')
  await expect(page.getByRole('note', { name: 'Model guidance' })).toBeVisible()
  const first = page.getByRole('checkbox', { name: 'Example assay' }),
    second = page.getByRole('checkbox', { name: 'Unselected assay' }),
    mine = page.getByRole('checkbox', { name: 'These results belong to me.' })
  await expect(first).not.toBeChecked()
  await expect(second).not.toBeChecked()
  await expect(mine).not.toBeChecked()
  const flag = page
    .getByRole('alert')
    .filter({ hasText: 'Critical flag in portal results' })
  await expect(flag).toBeVisible()
  await expect(flag).toContainText('Fictional panel · Example assay')
  await expect(flag).toContainText('HH')
  await expect(flag).not.toContainText('Unselected assay')
  expect(
    await flag.evaluate(
      (el) =>
        !!(
          el.compareDocumentPosition(document.querySelector('.portal-owner')!) &
          Node.DOCUMENT_POSITION_FOLLOWING
        ),
    ),
  ).toBe(true)
  expect(calls).not.toContain('/api/ehr/import')
  const row = page.locator('.portal-row').first()
  await expect(row).toContainText('<5 mg/L')
  await expect(row).toContainText('Effective date or period')
  await expect(row).toContainText('2026-10-04')
  await expect(row).toContainText('Source qualifier retained')
  await first.check()
  const submit = page.getByRole('button', {
    name: 'Import selected results (1)',
  })
  await expect(submit).toBeDisabled()
  await mine.check()
  await submit.click()
  await expect(page).toHaveURL(new RegExp('/#encounter/' + id + '/sources$'))
  await expect(
    page.getByRole('heading', { name: 'Medical reports' }),
  ).toBeVisible()
  await page.reload()
  await expect(
    page.getByRole('heading', { name: 'Medical reports' }),
  ).toBeVisible()
  expect(calls.filter((p) => p === '/api/ehr/import')).toHaveLength(1)
})

test('Portal denial, offline preview retry, empty results and cancellation leave local report entry available', async ({
  page,
}) => {
  let deny = true,
    offline = true,
    cancelled = false
  await localFixture(page, {
    preview: async (r) => {
      if (deny) {
        expect(r.request().postDataJSON().denied).toBe(true)
        await r.fulfill({ status: 400, json: { error: 'Connection ended' } })
      } else if (offline) {
        offline = false
        await r.abort('connectionreset')
      } else await r.fulfill({ json: { ...saved, rows: [] } })
    },
    cancel: async (r) => {
      expect(r.request().postDataJSON()).toEqual({ ticket: saved.ticket })
      cancelled = true
      await r.fulfill({ json: { cancelled: true } })
    },
  })
  await page.goto(
    '/ehr/callback?error=access_denied&error_description=private_synthetic_reason&state=denied',
  )
  await expect(page.getByRole('alert')).toContainText('Connection declined')
  expect(page.url()).not.toContain('private_synthetic_reason')
  await page.getByRole('button', { name: 'Return to journal' }).click()
  await expect(
    page.getByRole('heading', { name: 'Encounter journal' }),
  ).toBeVisible()
  deny = false
  await page.goto('/ehr/callback?code=fake&state=retry')
  await page.getByRole('button', { name: 'Retry preview' }).click()
  await expect(
    page.getByText('This preview contains no eligible lab results.'),
  ).toBeVisible()
  await expect(
    page.getByRole('button', { name: 'Import selected results (0)' }),
  ).toBeDisabled()
  await page.getByRole('button', { name: 'Cancel import' }).click()
  await expect(
    page.getByRole('heading', { name: 'Medical reports' }),
  ).toBeVisible()
  expect(cancelled).toBe(true)
  await expect(page.getByLabel('Report file')).toBeVisible()
  await expect(
    page.getByRole('button', { name: 'Connect to patient portal' }),
  ).toHaveCount(0)
})
