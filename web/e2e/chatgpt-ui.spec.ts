import { expect, test, type Page, type Route } from '@playwright/test'
import type { Provider } from '../src/types'

const first = '11'.repeat(16),
  second = '22'.repeat(16)
type Account = {
  id: string
  label: string
  connected: boolean
  planEnabled: boolean
}
async function workspace(
  page: Page,
  options: { planEnabled?: boolean; local?: boolean; firstUse?: boolean } = {},
) {
  let provider: Provider = {
    kind: 'demo',
    model: 'synthetic',
    endpoint: '',
    structured: 'auto',
  }
  const accounts: Account[] = [first, second].map((id) => ({
    id,
    label: 'Example account',
    connected: true,
    planEnabled: options.planEnabled !== false,
  }))
  const requests: { path: string; data?: Record<string, unknown> }[] = []
  let catalogFails = false
  await page.route('**/api/**', async (route: Route) => {
    const request = route.request(),
      url = new URL(request.url()),
      path = url.pathname,
      method = request.method()
    const data = request.postData() ? request.postDataJSON() : undefined
    requests.push({ path, data })
    if (method !== 'GET')
      expect(request.headers()['x-csrf-token']).toBe('fixture-csrf')
    const json = (value: unknown) => route.fulfill({ json: value })
    if (path === '/api/session')
      return json({
        setupRequired: false,
        authenticated: true,
        csrf: 'fixture-csrf',
        displayName: 'healthctl',
        notice: 'Synthetic notice.',
        noticeVersion: 'test',
        emergency: '',
      })
    if (path === '/api/region')
      return json({ locale: 'en', units: 'as-reported' })
    if (path === '/api/encounters') return json({ encounters: [] })
    if (path === '/api/settings') {
      if (method === 'PUT') provider = data.provider
      return json({ provider, credentialStatus: {}, allowlist: [] })
    }
    if (path === '/api/settings/test') return json({ ok: true })
    if (path === '/api/qualification')
      return json({ configurations: [], usage: {}, budget: {}, rates: {} })
    if (path === '/api/evidence') return json({ collections: [] })
    if (path === '/api/chatgpt')
      return json({ availableHere: options.local !== false, accounts })
    if (path.endsWith('/models')) {
      if (catalogFails)
        return route.fulfill({
          status: 503,
          json: { error: 'Catalog unavailable in the fictional test.' },
        })
      return json({
        models: path.includes(first)
          ? [
              {
                slug: 'model-b',
                displayName: 'Second by name, first in catalog',
              },
              {
                slug: 'model-a',
                displayName: 'First by name, second in catalog',
              },
            ]
          : [{ slug: 'model-c', displayName: 'Other account model' }],
      })
    }
    if (path === '/api/chatgpt/start')
      return json({ authorizationUrl: '/api/chatgpt/launch/synthetic-state' })
    if (path === '/api/chatgpt/launch/synthetic-state')
      return route.fulfill({
        status: 302,
        headers: {
          location:
            '/auth/callback?code=fake-code&state=fake-state&client_id=fake-issued',
        },
      })
    if (path === '/api/chatgpt/finish') {
      if (data.denied)
        return route.fulfill({
          status: 400,
          json: { error: 'Authorization denied' },
        })
      accounts[0].planEnabled = options.planEnabled !== false
      return json({
        account: accounts[0],
        firstPlanUse: options.firstUse || false,
      })
    }
    if (method === 'DELETE' && path.startsWith('/api/chatgpt/accounts/')) {
      const a = accounts.find((a) => path.endsWith(a.id))!
      a.connected = false
      a.planEnabled = false
      return json({ revocationConfirmed: false })
    }
    return route.fulfill({ status: 404, json: { error: 'Unknown test route' } })
  })
  return {
    requests,
    failCatalog: () => {
      catalogFails = true
    },
    saved: () => provider,
  }
}
async function selectPlan(page: Page) {
  await page.goto('/')
  await page
    .getByRole('button', { name: 'Model settings', exact: true })
    .click()
  await page
    .getByRole('combobox', { name: 'Provider', exact: true })
    .selectOption('chatgpt')
}

test('ChatGPT accounts have distinct destinations and ordered model catalogs with no fallback', async ({
  page,
}) => {
  const f = await workspace(page)
  await selectPlan(page)
  const account = page.getByRole('combobox', { name: 'ChatGPT account' }),
    model = page.getByRole('combobox', { name: 'Available ChatGPT model' }),
    save = page.getByRole('button', { name: 'Save provider', exact: true })
  await expect(save).toBeDisabled()
  await account.selectOption(first)
  await expect(model.locator('option')).toHaveText([
    'Choose a model',
    'Second by name, first in catalog',
    'First by name, second in catalog',
  ])
  await expect(model).toHaveValue('')
  await model.selectOption('model-b')
  await expect(save).toBeEnabled()
  await expect(page.locator('.side-panel').first()).toContainText(first)
  await expect(page.locator('.side-panel').first()).toContainText(
    'https://api.openai.com/v1/responses',
  )
  await account.selectOption(second)
  await expect(save).toBeDisabled()
  await expect(model.locator('option')).toHaveText([
    'Choose a model',
    'Other account model',
  ])
  await expect(model).toHaveValue('')
  await model.selectOption('model-c')
  await page.getByRole('button', { name: 'Test connection' }).click()
  await expect(
    page.getByRole('status').filter({ hasText: 'verified' }),
  ).toBeVisible()
  await save.click()
  await expect(
    page.getByRole('heading', { name: 'Encounter journal' }),
  ).toBeVisible()
  expect(f.saved()).toEqual({
    kind: 'chatgpt',
    endpoint: second,
    model: 'model-c',
    structured: 'auto',
  })
  await page.getByRole('button', { name: 'New encounter', exact: true }).click()
  await expect(page.locator('.context-box')).toContainText(second)
  await expect(page.locator('.context-box')).toContainText('Example account')
  await page
    .getByRole('navigation', { name: 'Workspace' })
    .getByRole('button', { name: 'Model settings' })
    .click()
  f.failCatalog()
  await account.selectOption(first)
  await expect(page.getByRole('alert')).toContainText('Catalog unavailable')
  await expect(save).toBeDisabled()
  expect(f.saved().endpoint).toBe(second)
})

test('Plan consent, callback scrubbing, first-use confirmation and account-only disconnect', async ({
  page,
}) => {
  const f = await workspace(page, { firstUse: true })
  await selectPlan(page)
  const consent = page.getByRole('checkbox', {
    name: /allow eligible model requests/i,
  })
  await expect(consent).not.toBeChecked()
  await consent.check()
  await page.getByRole('button', { name: 'Continue with ChatGPT' }).click()
  const welcome = page.getByRole('dialog')
  await expect(welcome).toContainText('You’re using your ChatGPT plan')
  await expect(
    welcome.getByRole('note', { name: 'Model guidance' }),
  ).toBeVisible()
  await expect(
    welcome.getByRole('link', { name: 'Manage usage' }),
  ).toHaveAttribute('href', 'https://chatgpt.com/settings/usage')
  expect(new URL(page.url()).search).toBe('')
  expect(f.requests.find((r) => r.path === '/api/chatgpt/start')?.data).toEqual(
    { enablePlan: true },
  )
  expect(
    f.requests.find((r) => r.path === '/api/chatgpt/finish')?.data,
  ).toEqual({
    state: 'fake-state',
    code: 'fake-code',
    clientId: 'fake-issued',
    denied: false,
  })
  await welcome.getByRole('button', { name: 'Got it' }).click()
  await expect(
    page.getByRole('combobox', { name: 'ChatGPT account' }),
  ).toHaveValue(first)
  await expect(
    page.getByRole('combobox', { name: 'Available ChatGPT model' }),
  ).toHaveValue('')
  expect(f.saved().kind).toBe('demo')
  await page.getByRole('button', { name: 'Disconnect ChatGPT account' }).click()
  await page.getByRole('button', { name: 'Confirm disconnect' }).click()
  await expect(
    page.getByRole('status').filter({ hasText: 'Disconnected locally' }),
  ).toContainText('Remote revocation could not be confirmed')
  await expect(
    page.getByRole('button', { name: 'Save provider' }),
  ).toBeDisabled()
  await expect(
    page.getByRole('navigation', { name: 'Workspace' }),
  ).toBeVisible()
  expect(f.requests.filter((r) => r.path === '/api/logout')).toHaveLength(0)
})

test('Identity-only or declined authorization keeps model requests unavailable; non-loopback origin explains the local option', async ({
  page,
}) => {
  const f = await workspace(page, { planEnabled: false, local: false })
  await page.goto('/auth/callback?code=fake-code&state=fake-state')
  await expect(page.getByText(/connected for identity only/)).toBeVisible()
  expect(new URL(page.url()).search).toBe('')
  await page.getByRole('button', { name: 'Continue to Model settings' }).click()
  await expect(
    page.getByRole('button', { name: 'Save provider' }),
  ).toBeDisabled()
  await expect(
    page.getByRole('combobox', { name: 'Available ChatGPT model' }),
  ).toHaveCount(0)
  await expect(page.getByText(/private SSH tunnel/)).toBeVisible()
  await expect(
    page.getByRole('button', { name: 'Continue with ChatGPT' }),
  ).toHaveCount(0)
  await page.goto('/auth/callback?error=access_denied&state=rejected')
  await expect(page.getByRole('alert')).toContainText(
    'ended without a new connection',
  )
  expect(new URL(page.url()).search).toBe('')
  await page.getByRole('button', { name: 'Return to Model settings' }).click()
  await page.goto(
    '/auth/callback?error=temporarily_unavailable&state=unavailable',
  )
  await expect(page.getByRole('alert')).toContainText(
    'sign-in could not be completed',
  )
  const finishCount = () =>
    f.requests.filter((r) => r.path === '/api/chatgpt/finish').length
  const earlier = finishCount()
  await page.goto(
    '/auth/callback?code=synthetic&state=malformed&client_id=one&client_id=two',
  )
  await expect(page.getByRole('alert')).toContainText(
    'incomplete or has already been used',
  )
  expect(new URL(page.url()).search).toBe('')
  expect(finishCount()).toBe(earlier)
  expect(f.saved().kind).toBe('demo')
  expect(f.requests.filter((r) => r.path.endsWith('/models'))).toHaveLength(0)
})

test('Existing ChatGPT encounters keep their account until the owner confirms a destination change', async ({
  page,
}) => {
  const f = await workspace(page)
  await selectPlan(page)
  const account = page.getByRole('combobox', { name: 'ChatGPT account' })
  await account.selectOption(second)
  await page
    .getByRole('combobox', { name: 'Available ChatGPT model' })
    .selectOption('model-c')
  await page.getByRole('button', { name: 'Save provider' }).click()
  const id = 'synthetic-pinned',
    createdAt = '2026-01-03T00:00:00Z',
    original: Provider = {
      kind: 'chatgpt',
      model: 'same-model',
      endpoint: first,
      structured: 'auto',
    },
    destination: Provider = { ...original, endpoint: second }
  let record: import('../src/types').Encounter = {
    id,
    createdAt,
    updatedAt: createdAt,
    provider: original,
    state: 'failed_recoverably',
    demo: false,
    concern: 'Fictional account switch',
    age: 35,
    pregnancyRelated: false,
    background: '',
    notice: 'Fictional notice.',
    noticeVersion: 'test',
    revision: 1,
    historyRevision: 1,
    confirmedRevision: 0,
    answers: [],
    facts: [],
    assessments: [],
    warnings: [],
    contradictions: [],
    outstanding: [],
    question: '',
    choices: [],
    failure: 'This ChatGPT plan usage limit has been reached.',
    scopeReason: '',
    pending: '',
    providerChanges: [],
  }
  const moves: unknown[] = []
  await page.route('**/api/settings', (r) =>
    r.fulfill({
      json: { provider: destination, credentialStatus: {}, allowlist: [] },
    }),
  )
  await page.route('**/api/encounters', (r) =>
    r.fulfill({ json: { encounters: [record] } }),
  )
  await page.route('**/api/encounters/' + id, (r) =>
    r.fulfill({ json: record }),
  )
  await page.route('**/api/encounters/' + id + '/actions', (r) => {
    const body = r.request().postDataJSON()
    moves.push(body)
    expect(body.confirmedDestination).toBe(true)
    expect(body.action).toBe('change_provider')
    expect(body.provider).toEqual(destination)
    record = {
      ...record,
      revision: 2,
      provider: destination,
      providerChanges: [
        { previous: original, current: destination, at: createdAt },
      ],
    }
    return r.fulfill({ json: record })
  })
  await page.reload()
  await page.getByRole('button', { name: /Fictional account switch/ }).click()
  await expect(page.locator('.record-aside')).toContainText(first)
  await expect(
    page.getByRole('alert').getByRole('link', { name: 'Manage usage' }),
  ).toHaveAttribute('href', 'https://chatgpt.com/settings/usage')
  await page.getByText('Manage encounter', { exact: true }).click()
  await page
    .getByRole('button', { name: 'Change provider', exact: true })
    .click()
  await expect(page.locator('.move-box .context-box')).toContainText(second)
  const confirm = page.getByRole('button', { name: 'Confirm provider change' })
  await expect(confirm).toBeDisabled()
  expect(moves).toHaveLength(0)
  await page.getByLabel('I agree to send this encounter').check()
  await confirm.click()
  await expect(page.locator('.manage li')).toContainText(first)
  await expect(page.locator('.manage li')).toContainText(second)
  expect(moves).toHaveLength(1)
  expect(f.saved().endpoint).toBe(second)
})
