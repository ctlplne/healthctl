import { chromium } from 'playwright'
import { readFile } from 'node:fs/promises'
import { spawn } from 'node:child_process'
import { randomUUID } from 'node:crypto'
import assert from 'node:assert/strict'

// Run inside the disposable QA app's network namespace. Never accept a remote URL.
const base = 'http://127.0.0.1:8080'
const password = (await readFile('/dev/stdin', 'utf8')).trim()
assert.ok(password, 'Pass the isolated QA-owner credential on stdin.')
const browser = await chromium.launch({
  headless: false,
  executablePath: '/ms-playwright/chromium-1193/chrome-linux/chrome',
  args: [
    '--no-sandbox',
    '--force-renderer-accessibility',
    '--disable-dev-shm-usage',
  ],
})
const page = await browser.newPage()
const pause = (ms) => new Promise((r) => setTimeout(r, ms))
const readLog = () => readFile('/tmp/orca-events.log', 'utf8').catch(() => '')
let csrf = '',
  record,
  priorProvider,
  audio
const api = async (method, path, data) => {
  const response = await page.request.fetch(base + '/api' + path, {
    method,
    headers: { Origin: base, 'X-CSRF-Token': csrf },
    data,
  })
  assert.equal(
    response.status(),
    200,
    method + ' ' + path + ' status ' + response.status(),
  )
  return await response.json()
}
const tabTo = async (locator, name) => {
  await locator.waitFor({ state: 'visible' })
  for (let i = 0; i < 150; i++) {
    if (await locator.evaluate((el) => el === document.activeElement)) {
      await pause(950)
      return
    }
    await page.keyboard.press('Tab')
    await pause(35)
  }
  throw new Error('Keyboard focus never reached ' + name)
}
try {
  await page.goto(base)
  const user = page.getByRole('textbox', { name: 'Username', exact: true })
  await user.waitFor()
  await page.bringToFront()
  for (let i = 0; i < 50; i++) {
    if (/chromium|chrome/i.test(await readLog())) break
    await pause(200)
  }
  const offset = (await readLog()).length
  audio = spawn('parec', [
    '--device=qa_output.monitor',
    '--format=s16le',
    '--channels=1',
    '--rate=16000',
  ])
  let samples = 0,
    nonSilent = 0
  audio.stdout.on('data', (b) => {
    for (let i = 0; i + 1 < b.length; i += 2) {
      samples++
      if (Math.abs(b.readInt16LE(i)) > 64) nonSilent++
    }
  })
  await user.focus()
  await pause(1200)
  await page.keyboard.press('Tab')
  await pause(1200)
  await page.keyboard.press('Tab')
  await pause(1300)
  // Authenticate by API so password characters never enter Orca's speech or debug log.
  const session = await api('POST', '/login', {
    username: 'synthetic-owner',
    password,
  })
  csrf = session.csrf
  priorProvider = (await api('GET', '/settings')).provider
  const provider = {
    kind: 'demo',
    model: 'synthetic',
    structured: 'auto',
    endpoint: '',
  }
  assert.ok(
    priorProvider?.kind,
    'Run the standard browser suite to provision the QA owner first',
  )
  await api('PUT', '/settings', { provider })
  const concern = 'My nose is runny and I keep sneezing.'
  record = await api('POST', '/encounters', {
    concern,
    age: 35,
    pregnancyRelated: false,
    urgentNow: false,
    background: '',
    noticeVersion: session.noticeVersion,
    requestId: randomUUID(),
    disclosedProvider: provider,
  })
  for (const text of ['Yesterday', 'After being outdoors'])
    record = await api('POST', '/encounters/' + record.id + '/actions', {
      action: 'answer',
      text,
      revision: record.revision,
      requestId: randomUUID(),
    })
  assert.equal(record.state, 'history_review')
  await page.reload()
  const targets = [
    [
      page
        .getByRole('button', { name: /My nose is runny and I keep sneezing/ })
        .first(),
      'Synthetic encounter',
    ],
    ...[
      'Confirm my history',
      'Generate provisional estimate',
      'Open visit report',
      'Clinician brief',
    ].map((name) => [page.getByRole('button', { name, exact: true }), name]),
  ]
  for (const [locator, name] of targets) {
    await tabTo(locator, name)
    await page.keyboard.press('Enter')
    await pause(240)
  }
  const downloadLink = page.getByRole('link', {
    name: 'Download JSON',
    exact: true,
  })
  await tabTo(downloadLink, 'Download JSON')
  const download = page.waitForEvent('download')
  await page.keyboard.press('Enter')
  const exported = JSON.parse(
    await readFile(await (await download).path(), 'utf8'),
  )
  assert.equal(exported.id, record.id)
  assert.equal(exported.assessments.at(-1).items[0].name, 'Nasal allergies')
  await pause(1000)
  const spoken = (await readLog())
    .slice(offset)
    .split('\n')
    .filter((line) => /SPEECH OUTPUT:|SPEECH DISPATCHER.*speak/i.test(line))
  const labels = [
    'Username',
    'Password',
    'Sign in',
    'Confirm my history',
    'Generate provisional estimate',
    'Open visit report',
    'Clinician brief',
    'Download JSON',
  ]
  for (const label of labels)
    assert.ok(
      spoken.some((l) => l.toLowerCase().includes(label.toLowerCase())),
      'Missing speech event: ' + label,
    )
  assert.ok(nonSilent > 300, 'No measurable synthesized speech audio')
  console.log(
    JSON.stringify({
      screenReader: 'Orca',
      engine: 'Speech Dispatcher / eSpeak NG',
      announcements: labels,
      interaction:
        'Keyboard history confirmation, estimate, clinician brief and downloaded report',
      sampleRate: 16000,
      recordedSamples: samples,
      nonSilentSamples: nonSilent,
    }),
  )
} finally {
  audio?.kill('SIGTERM')
  try {
    if (record) {
      const current = await api('GET', '/encounters/' + record.id)
      await api('DELETE', '/encounters/' + record.id, {
        revision: current.revision,
      })
    }
    if (priorProvider)
      await api('PUT', '/settings', { provider: priorProvider })
  } finally {
    await browser.close()
  }
}
