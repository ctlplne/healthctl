import { expect, test } from '@playwright/test'

test.use({
  launchOptions: {
    executablePath:
      process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH ||
      process.env.CHROMIUM_PATH,
    // Keep the authorization host on loopback even if browser request interception fails.
    args: [
      '--host-resolver-rules=MAP auth.openai.com 127.0.0.1',
      '--no-proxy-server',
    ],
  },
})

test('A real local ChatGPT authorization can be declined while preserving the owner session and saved model', async ({
  page,
  context,
}) => {
  const ownerPassword = process.env.CLINICAL_E2E_PASSWORD
  if (!ownerPassword)
    throw new Error('Set a password for the disposable QA owner.')
  await page.goto('/')
  const origin = new URL(page.url()).origin
  expect(new URL(origin).hostname).toBe('127.0.0.1')
  await page
    .getByLabel('Username', { exact: true })
    .fill(process.env.CLINICAL_E2E_OWNER || 'synthetic-owner')
  await page.getByLabel('Password', { exact: true }).fill(ownerPassword)
  await page
    .getByRole('button', { name: /^(Create owner account|Sign in)$/ })
    .click()
  await expect(
    page.getByRole('heading', { name: 'Encounter journal' }),
  ).toBeVisible()
  const status = async () => {
    const r = await page.request.get('/api/chatgpt')
    expect(r.ok()).toBe(true)
    return r.json()
  }
  const provider = async () => {
    const r = await page.request.get('/api/settings')
    expect(r.ok()).toBe(true)
    return (await r.json()).provider
  }
  const [previous, accounts] = await Promise.all([provider(), status()])
  expect(accounts.availableHere).toBe(true)
  await page
    .getByRole('button', { name: 'Model settings', exact: true })
    .click()
  const providerPicker = page.getByRole('combobox', {
    name: 'Provider',
    exact: true,
  })
  const previousSelection = await providerPicker.inputValue()
  await providerPicker.selectOption('chatgpt')
  await page.getByRole('combobox', { name: 'ChatGPT account' }).selectOption('')
  await page
    .getByRole('checkbox', { name: /Allow eligible model requests/i })
    .check()

  // CDP pauses the actual redirected main-frame request, including redirects between origins.
  // Only the remote authorization page is simulated. All application routes hit the Go server.
  const cdp = await context.newCDPSession(page)
  await cdp.send('Fetch.enable', {
    patterns: [
      { urlPattern: 'https://auth.openai.com/*', requestStage: 'Request' },
    ],
  })
  const intercepted = new Promise<void>((resolve, reject) => {
    cdp.once(
      'Fetch.requestPaused',
      async ({ requestId, request, resourceType }) => {
        try {
          expect(resourceType).toBe('Document')
          const url = new URL(request.url),
            q = url.searchParams
          expect(url.origin + url.pathname).toBe(
            'https://auth.openai.com/api/accounts/authorize',
          )
          expect(q.get('client_id')).toBe('dynamic_agent_client')
          expect(q.get('response_type')).toBe('code')
          expect(q.get('agent_name_hint')).toBe('healthctl')
          expect(q.get('code_challenge_method')).toBe('S256')
          expect(q.get('resource')).toBe('https://api.openai.com/v1')
          expect(q.get('scope')?.split(' ').sort()).toEqual(
            [
              'openid',
              'profile',
              'email',
              'offline_access',
              'resource.invoke',
              'chatgpt.tokens.use.direct',
            ].sort(),
          )
          expect(q.get('redirect_uri')).toBe(origin + '/auth/callback')
          expect(/^urn:uuid:/i.test(q.get('ext_agent_host_id') || '')).toBe(
            true,
          )
          expect(/^[\w-]{43}$/.test(q.get('code_challenge') || '')).toBe(true)
          expect(Boolean(q.get('state') && q.get('nonce'))).toBe(true)
          expect(
            q.has('code_verifier') ||
              q.has('id_token_hint') ||
              q.has('login_hint'),
          ).toBe(false)
          const callback = new URL('/auth/callback', origin)
          callback.searchParams.set('state', q.get('state')!)
          callback.searchParams.set('error', 'access_denied')
          await cdp.send('Fetch.fulfillRequest', {
            requestId,
            responseCode: 302,
            responseHeaders: [
              { name: 'Location', value: callback.href },
              { name: 'Cache-Control', value: 'no-store' },
            ],
          })
          resolve()
        } catch (error) {
          await cdp
            .send('Fetch.failRequest', { requestId, errorReason: 'Aborted' })
            .catch(() => {})
          reject(error)
        }
      },
    )
  })
  const [, start, launch, finish] = await Promise.all([
    intercepted,
    page.waitForResponse(
      (r) => new URL(r.url()).pathname === '/api/chatgpt/start',
    ),
    page.waitForResponse((r) =>
      /^\/api\/chatgpt\/launch\/[\w-]+$/.test(new URL(r.url()).pathname),
    ),
    page.waitForResponse(
      (r) => new URL(r.url()).pathname === '/api/chatgpt/finish',
    ),
    page.getByRole('button', { name: 'Continue with ChatGPT' }).click(),
  ])
  expect(start.ok()).toBe(true)
  expect(launch.status()).toBe(303)
  expect(finish.status()).toBe(400)
  await expect(page.getByRole('alert')).toContainText(
    'ended without a new connection',
  )
  expect(page.url()).toBe(origin + '/auth/callback')
  await page.getByRole('button', { name: 'Return to Model settings' }).click()
  await expect(providerPicker).toHaveValue(previousSelection)
  expect(await provider()).toEqual(previous)
  expect(await status()).toEqual(accounts)
  expect(
    (await (await page.request.get('/api/session')).json()).authenticated,
  ).toBe(true)
  await cdp.detach()
})
