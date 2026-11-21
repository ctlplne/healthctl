import { expect, test } from '@playwright/test'
import { randomUUID } from 'node:crypto'

test('Patient portal consent imports only selected labs into local report review', async ({ page, browser, baseURL }) => {
  const browserDestinations = new Set<string>()
  const portalPaths = new Set<string>()
  page.on('request', req => {
    const dest = new URL(req.url())
    if (dest.protocol === 'http:' || dest.protocol === 'https:') {
      browserDestinations.add(dest.origin)
      if (dest.origin === 'http://localhost:8091') portalPaths.add(dest.pathname)
    }
  })
  const local = page.request
  const credentials = { username: 'synthetic-owner', password: process.env.CLINICAL_E2E_PASSWORD }
  const initial = await (await local.get('/api/session')).json()
  const auth = await local.post(initial.setupRequired ? '/api/setup' : '/api/login', { data: credentials })
  expect(auth.ok()).toBeTruthy()
  const session = await auth.json()
  const headers = { 'X-CSRF-Token': session.csrf }
  const provider = { kind: 'demo', model: 'synthetic', structured: 'auto', endpoint: '' }
  expect((await local.put('/api/settings', { headers, data: { provider } })).ok()).toBeTruthy()
  const created = await local.post('/api/encounters', { headers, data: {
    requestId: randomUUID(), disclosedProvider: provider, age: 36, reportFirst: true,
    concern: 'I want to review a fictional portal result.', urgentNow: false,
    pregnancyRelated: false, noticeVersion: session.noticeVersion,
  } })
  expect(created.ok()).toBeTruthy()
  const visit = await created.json()
  const apiPath = '/api/encounters/' + visit.id
  const sourceLink = '/#encounter/' + visit.id + '/sources'
  try {
    await page.goto(sourceLink)
    await page.getByRole('button', { name: 'Connect to patient portal' }).click()
    await expect(page.getByRole('heading', { name: 'Fictional hospital portal' })).toBeVisible()
    const launch = new URL(page.url())
    expect(launch.hostname).toBe('localhost')
    expect(launch.searchParams.get('code_challenge_method')).toBe('S256')
    expect(launch.searchParams.get('scope')?.split(' ').sort()).toEqual(['launch/patient', 'patient/Observation.rs'])
    expect(page.url()).not.toContain(visit.id)
    const other = await browser.newContext({ baseURL })
    try {
      const otherAuth = await (await other.request.post('/api/login', { data: credentials })).json()
      const wrong = await other.request.post('/api/ehr/preview', {
        headers: { 'X-CSRF-Token': otherAuth.csrf }, data: { state: launch.searchParams.get('state'), code: 'wrong-session' },
      })
      expect(wrong.status()).toBe(400)
    } finally {
      await other.close()
    }
    const previewResponse = page.waitForResponse(res => res.url().endsWith('/api/ehr/preview'))
    await page.getByRole('button', { name: 'Allow laboratory access' }).click()
    const received = await previewResponse
    expect(received.ok()).toBeTruthy()
    const p = await received.json()
    expect(p.skipped).toBe(1)
    expect(p.rows).toHaveLength(2)
    await expect(page.getByRole('heading', { name: 'Choose lab results' })).toBeVisible()
    expect(new URL(page.url()).search).toBe('')
    await expect(page.getByRole('note', { name: 'Model guidance' })).toBeVisible()
    await expect(page.getByRole('alert').filter({ hasText: /critical/i })).toBeVisible()
    const check = page.getByRole('checkbox', { name: 'Fictional marker A' })
    await expect(check).not.toBeChecked()
    await expect(page.getByText('Wrong-patient marker')).toHaveCount(0)
    await check.check()
    await expect(page.getByRole('button', { name: 'Import selected results (1)' })).toBeDisabled()
    await page.getByRole('checkbox', { name: 'These results belong to me.' }).check()
    await page.getByRole('button', { name: 'Import selected results (1)' }).click()
    await expect(page).toHaveURL(new RegExp('#encounter/' + visit.id + '/sources$'))
    const report = await (await local.get(apiPath + '/export')).json()
    expect(report.documents).toHaveLength(1)
    const doc = report.documents[0]
    expect(doc.rows).toHaveLength(1)
    expect(doc.rows[0]).toMatchObject({ name: 'Fictional marker A', units: 'mg/L', value: '4.2' })
    expect(report.reportReviews ?? []).toHaveLength(0)
    expect(report.reportConsents ?? []).toHaveLength(0)
    const original = await (await local.get(apiPath + '/documents/' + doc.id + '/original')).text()
    expect(original).toContain('Fictional marker A')
    expect(original).not.toMatch(/Fictional marker B|Wrong-patient marker|another-patient/)
    const retry = await local.post('/api/ehr/import', { headers, data: {
      ticket: p.ticket, keys: [p.rows[0].key], confirmedPatient: true,
    } })
    expect(retry.ok()).toBeTruthy()
    expect((await retry.json()).documents).toHaveLength(1)
    await page.getByRole('button', { name: 'Connect to patient portal' }).click()
    await page.getByRole('button', { name: 'Allow laboratory access' }).click()
    await page.getByRole('checkbox', { name: 'Fictional marker B' }).check()
    await page.getByRole('checkbox', { name: 'These results belong to me.' }).check()
    await page.getByRole('button', { name: 'Import selected results (1)' }).click()
    await expect(page).toHaveURL(new RegExp('#encounter/' + visit.id + '/sources$'))
    const urgent = await (await local.get(apiPath + '/export')).json()
    expect(urgent.warnings.some((w: {active: boolean}) => w.active)).toBe(true)
    expect(urgent.state).toBe('interrupted_for_safety')
    await page.getByRole('button', { name: 'Connect to patient portal' }).click()
    await page.getByRole('button', { name: 'Decline access' }).click()
    await expect(page.getByText('Connection declined', { exact: true })).toBeVisible()
    expect((await (await local.get(apiPath)).json()).documents).toHaveLength(2)
    expect([...browserDestinations].sort()).toEqual([new URL(baseURL!).origin, 'http://localhost:8091'].sort())
    expect([...portalPaths].sort()).toEqual(['/authorize', '/decision'])
  } finally {
    await local.delete(apiPath, { headers })
  }
})
