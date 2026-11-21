import { expect, test, type Page } from '@playwright/test'
import { readFile, writeFile } from 'node:fs/promises'
import { readFileSync } from 'node:fs'
import { regionalMeasurement } from '../src/region'
import { randomUUID } from 'node:crypto'
const sample = 'My nose is runny and I keep sneezing.'
const owner = process.env.CLINICAL_E2E_OWNER || 'synthetic-owner'
const password = process.env.CLINICAL_E2E_PASSWORD
if (!password)
  throw new Error('Set CLINICAL_E2E_PASSWORD for the disposable QA owner.')
const fixture =
  process.env.CLINICAL_E2E_PROVIDER_URL || 'http://fixture:8090/v1'

const guidance = readFileSync(
  new URL('../../internal/clinical/model-guidance.txt', import.meta.url),
  'utf8',
).trim()
async function guidanceVisible(page: Page, scroll = false) {
  const banner = page.getByRole('note', { name: 'Model guidance' })
  if (scroll)
    await page.evaluate(() =>
      window.scrollTo(0, document.documentElement.scrollHeight),
    )
  await expect(banner).toHaveText(guidance)
  await expect(page.locator('#preview-notice')).toContainText('Free public preview (beta)')
  const header = page.locator('#installation-guidance')
  await expect(header).toBeInViewport({ ratio: 1 })
  await expect
    .poll(() =>
      banner.evaluate((el) => {
        const b = el.getBoundingClientRect()
        const top = document.elementFromPoint(
          b.x + b.width / 2,
          b.y + b.height / 2,
        )
        const header = el.closest('#installation-guidance')!
        return Math.abs(header.getBoundingClientRect().top) <= 1 && el.contains(top)
      }),
    )
    .toBe(true)
  if (scroll) await page.evaluate(() => window.scrollTo(0, 0))
}
async function enter(page: Page) {
  await page.goto('/')
  await guidanceVisible(page)
  await page.getByLabel('Username', { exact: true }).fill(owner)
  await page.getByLabel('Password', { exact: true }).fill(password!)
  const setup = page.getByRole('button', { name: 'Create owner account' })
  if (await setup.count()) await setup.click()
  else await page.getByRole('button', { name: 'Sign in', exact: true }).click()
  await expect(
    page.getByRole('heading', { name: 'Encounter journal' }),
  ).toBeVisible()
}
async function configure(page: Page, kind: 'demo' | 'compatible', model = '') {
  await page
    .getByRole('button', { name: 'Model settings', exact: true })
    .click()
  await guidanceVisible(page, true)
  await page
    .getByRole('combobox', { name: 'Provider', exact: true })
    .selectOption(kind)
  if (kind === 'compatible') {
    await page.getByLabel('Model identifier').fill(model)
    await page.getByLabel('Endpoint base URL').fill(fixture)
  }
  await page
    .getByRole('button', { name: 'Test connection', exact: true })
    .click()
  await expect(
    page.getByRole('status').filter({ hasText: /available|verified/ }),
  ).toBeVisible()
  await page.getByRole('button', { name: 'Save provider' }).click()
  await expect(
    page.getByRole('button', { name: 'New encounter' }),
  ).toBeVisible()
}
async function start(
  page: Page,
  opts: {
    concern?: string
    age?: number
    urgent?: boolean
    reportFirst?: boolean
  } = {},
) {
  await page
    .getByRole('button', {
      name: opts.reportFirst ? 'Start with a report' : 'New encounter',
      exact: true,
    })
    .click()
  await guidanceVisible(page, true)
  if (opts.concern)
    await page
      .getByLabel(
        opts.reportFirst
          ? 'What would you like to understand?'
          : 'Chief concern',
      )
      .fill(opts.concern)
  if (opts.age) await page.getByLabel('Age in years').fill(String(opts.age))
  await page
    .getByRole('group', { name: 'Is the concern related to pregnancy?' })
    .getByLabel('No', { exact: true })
    .check()
  await page
    .getByRole('group', {
      name: 'Are you reporting a possible emergency right now?',
    })
    .getByLabel(opts.urgent ? 'Yes' : 'No', { exact: true })
    .check()
  await page.getByRole('checkbox').check()
  const response = page.waitForResponse(
    (r) =>
      new URL(r.url()).pathname === '/api/encounters' &&
      r.request().method() === 'POST',
  )
  await page
    .getByRole('button', {
      name: opts.reportFirst
        ? 'Create encounter and add report'
        : 'Start interview',
      exact: true,
    })
    .click()
  const creation = await response
  expect([200, 202]).toContain(creation.status())
  const e = await creation.json()
  await expect(
    page.getByRole('heading', { name: 'Encounter record' }),
  ).toBeVisible()
  await guidanceVisible(page, true)
  return e.id as string
}
async function remove(page: Page) {
  await page.getByText('Manage encounter', { exact: true }).click()
  await page
    .getByRole('button', { name: 'Delete encounter', exact: true })
    .click()
  await page.getByRole('button', { name: 'Delete permanently' }).click()
  await expect(
    page.getByRole('heading', { name: 'Encounter journal' }),
  ).toBeVisible()
}
async function finishBackground(page: Page) {
  for (const [question, answer] of [
    [
      'What medicines or supplements do you take?',
      'I take no medicines or supplements.',
    ],
    ['What allergies do you know you have?', 'Not sure'],
    ['What ongoing health conditions do you have?', 'Not applicable'],
    ['What operations have you had in the past?', 'Skip this question'],
  ]) {
    await expect(
      page.getByRole('heading', { name: question, exact: true }),
    ).toBeVisible()
    await expect(page.getByLabel('Your answer', { exact: true })).toHaveCount(0)
    await page.getByRole('button', { name: answer, exact: true }).click()
    await page.getByRole('button', { name: 'Save answer' }).click()
  }
}
async function reviewAndAssess(page: Page) {
  await page.getByRole('button', { name: 'Confirm my history' }).click()
  await page
    .getByRole('button', { name: 'Generate provisional estimate' })
    .click()
}
async function grabReport(page: Page) {
  await page
    .getByRole('navigation', { name: 'Encounter sections' })
    .getByRole('button', { name: 'Report', exact: true })
    .click()
  await guidanceVisible(page, true)
  const download = page.waitForEvent('download')
  await page.getByRole('link', { name: 'Download JSON' }).click()
  return JSON.parse(await readFile((await (await download).path())!, 'utf8'))
}
async function layoutFits(page: Page) {
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBeTruthy()
}
test('Owner setup and synthetic encounter through correction, print, export, resume and deletion', async ({
  page,
}, info) => {
  const unexpected: string[] = []
  page.on('pageerror', (e) => unexpected.push(e.message))
  await enter(page)
  await configure(page, 'demo')
  const id = await start(page)
  await layoutFits(page)
  await page.getByRole('button', { name: 'Yesterday', exact: true }).click()
  await page.getByRole('button', { name: 'Save answer' }).click()
  await page
    .getByRole('button', { name: 'After being outdoors', exact: true })
    .click()
  await page.getByRole('button', { name: 'Save answer' }).click()
  await finishBackground(page)
  await reviewAndAssess(page)
  await expect(
    page.getByRole('heading', { name: 'Nasal allergies', exact: true }),
  ).toBeVisible()
  const first = await grabReport(page)
  expect(
    first.answers
      .filter(
        (a: { questionSource?: { key: string } }) =>
          a.questionSource?.key === 'background',
      )
      .map((a: { disposition: string }) => a.disposition),
  ).toEqual(['none', 'unknown', 'not_applicable', 'skipped'])
  expect(first.exportVersion).toBe('clinical-history/1')
  expect(first.modelGuidance).toBe(guidance)
  expect(first.reviewStatus).toBe('not_clinician_reviewed')
  expect(first.notice).toContain('verify the findings before making care decisions')
  expect(first.previewNotice).toContain('a ctlplne studio product')
  expect(first.assessments.at(-1).items).toHaveLength(2)
  await page.emulateMedia({ media: 'print' })
  await expect(page.locator('.print-only')).toContainText('Nasal allergies')
  await expect(page.locator('.print-only')).toContainText(
    'not reviewed by a healthcare professional',
    { ignoreCase: true },
  )
  const pdf = await page.pdf({
    format: 'A4',
    path: info.outputPath('synthetic-report.pdf'),
  })
  expect(pdf.subarray(0, 4).toString()).toBe('%PDF')
  await page.emulateMedia({ media: 'screen' })
  if (info.project.name === 'desktop') {
    const viewport = page.viewportSize()!
    await page.setViewportSize({ width: 1280, height: 720 })
    await page
      .getByRole('button', { name: 'Clinician brief', exact: true })
      .click()
    await page.emulateMedia({ media: 'print' })
    await expect(page.locator('.print-only article.brief')).toContainText(
      'Nasal allergies',
    )
    await page.evaluate(() => document.fonts.ready)
    await page.pdf({
      format: 'A4',
      preferCSSPageSize: true,
      printBackground: true,
      path: info.outputPath('synthetic-nasal-clinician-brief.pdf'),
    })
    await page.emulateMedia({ media: 'screen' })
    await page.setViewportSize(viewport)
  }
  await page.getByRole('button', { name: 'Pause and return' }).click()
  await page.reload()
  await page
    .getByRole('button', { name: new RegExp(sample.replace(/[.]/g, '')) })
    .first()
    .click()
  await page
    .getByRole('navigation', { name: 'Encounter sections' })
    .getByRole('button', { name: 'History review' })
    .click()
  await page
    .getByRole('button', { name: 'Correct finding: Runny nose' })
    .click()
  const correctedWording =
    'My nose was runny yesterday. <b>synthetic wording</b>'
  await page.getByLabel('Corrected wording').fill(correctedWording)
  await page.getByRole('button', { name: 'Save correction' }).click()
  await expect(page.locator('.screen-encounter')).toContainText(
    'The earlier estimate is outdated',
  )
  await expect(
    page
      .locator('.screen-encounter blockquote')
      .filter({ hasText: correctedWording }),
  ).toBeVisible()
  await expect(
    page.locator('b').filter({ hasText: 'synthetic wording' }),
  ).toHaveCount(0)
  await reviewAndAssess(page)
  await expect(
    page.getByRole('heading', {
      name: 'More information is needed for a useful estimate',
    }),
  ).toBeVisible()
  await page.getByText('Compare assessment revisions', { exact: true }).click()
  const differences = page.locator('details').filter({
    has: page.getByText('Compare assessment revisions', { exact: true }),
  })
  await expect(differences).toContainText('Nasal allergies')
  await expect(differences).toContainText(correctedWording)
  const next = await grabReport(page)
  expect(next.id).toBe(id)
  expect(next.historyRevision).toBeGreaterThan(first.historyRevision)
  expect(next.assessments[0]).toEqual(first.assessments[0])
  expect(next.assessments.at(-1).historyRevision).toBe(next.historyRevision)
  await layoutFits(page)
  await remove(page)
  const deleted = await page.request.get('/api/encounters/' + id + '/export')
  expect(deleted.status()).toBe(404)
  await page.getByRole('button', { name: 'Sign out' }).click()
  await expect(
    page.getByRole('button', { name: 'Sign in', exact: true }),
  ).toBeVisible()
  expect(
    (await (await page.request.get('/api/session')).json()).authenticated,
  ).toBe(false)
  expect(
    (await page.request.get('/api/encounters/' + id + '/export')).status(),
  ).toBe(401)
  expect(unexpected).toEqual([])
})
test('Urgent screen interrupts the interview and persists in print and JSON after patient requests an estimate', async ({
  page,
}, info) => {
  await enter(page)
  await configure(page, 'compatible', 'fixture-urgent-supported')
  await start(page, { concern: sample, age: 35, urgent: true })
  const warning = page.getByRole('alert').filter({ hasText: 'Urgent warning' })
  await warning.scrollIntoViewIfNeeded()
  await expect(warning).toBeVisible()
  await guidanceVisible(page)
  await page.screenshot({
    path: info.outputPath('guidance-and-urgent-warning.png'),
  })
  await page.getByRole('button', { name: 'Review my history' }).click()
  await reviewAndAssess(page)
  await expect(
    page.getByRole('alert').filter({ hasText: 'Urgent warning' }),
  ).toBeVisible()
  await expect(
    page
      .locator('.encounter-content')
      .getByRole('complementary', { name: 'Active urgent warning' }),
  ).toBeVisible()
  const r = await grabReport(page)
  expect(r.warnings.some((w: { active: boolean }) => w.active)).toBeTruthy()
  expect(r.assessments.length).toBe(1)
  expect(
    r.assessments[0].items.map((item: { name: string }) => item.name),
  ).toContain('Nasal allergies')
  expect(r.assessments[0].destination.model).toBe('fixture-urgent-supported')
  await page.emulateMedia({ media: 'print' })
  await expect(page.locator('.print-only')).toContainText('Active warning')
  await page.pdf({
    format: 'A4',
    path: info.outputPath('synthetic-urgent-report.pdf'),
  })
  await page.emulateMedia({ media: 'screen' })
  await remove(page)
})
test('Compatible endpoint rejection, recovery and explicit destination changes', async ({
  page,
}, info) => {
  const concern =
    'Synthetic timing question ' + info.project.name + ' ' + randomUUID()
  await enter(page)
  await configure(page, 'compatible', 'fixture-fail-once')
  const id = await start(page, { concern, age: 38 })
  await expect(
    page.getByRole('button', { name: 'Retry provider request' }),
  ).toBeVisible()
  await guidanceVisible(page, true)
  await page.reload()
  await page.getByRole('button', { name: new RegExp(concern) }).click()
  await page.getByRole('button', { name: 'Retry provider request' }).click()
  await expect(
    page.getByRole('heading', {
      name: 'When did you first notice the change?',
    }),
  ).toBeVisible()
  await page.getByRole('button', { name: 'Pause and return' }).click()
  await configure(page, 'compatible', 'fixture-ok')
  const before = await (await page.request.get('/api/encounters/' + id)).json()
  expect(before.provider.model).toBe('fixture-fail-once')
  await page.getByRole('button', { name: new RegExp(concern) }).click()
  await page.getByText('Manage encounter', { exact: true }).click()
  await page
    .getByRole('button', { name: 'Change provider', exact: true })
    .click()
  const change = page.getByRole('button', { name: 'Confirm provider change' })
  await expect(change).toBeDisabled()
  await page.getByLabel('I agree to send this encounter').check()
  await change.click()
  await expect(
    page.locator('.record-aside dd').filter({ hasText: 'fixture-ok' }),
  ).toBeVisible()
  const after = await (await page.request.get('/api/encounters/' + id)).json()
  expect(after.providerChanges).toHaveLength(1)
  const changeEntry = page
    .locator('.manage li')
    .filter({ hasText: 'From OpenAI-compatible endpoint' })
  await expect(changeEntry).toContainText('fixture-fail-once')
  await expect(changeEntry).toContainText('fixture-ok')
  await expect(changeEntry).toContainText(fixture)
  expect(
    after.answers.filter(
      (a: { kind: string; text: string }) =>
        a.kind === 'concern' && a.text === concern,
    ),
  ).toHaveLength(1)
  await page
    .getByRole('button', { name: 'Delete encounter', exact: true })
    .click()
  await page.getByRole('button', { name: 'Delete permanently' }).click()
  await expect(
    page.getByRole('heading', { name: 'Encounter journal' }),
  ).toBeVisible()
})

test('Outside-scope history stays exportable and a corrected intake reopens the adult workflow', async ({
  page,
}) => {
  await enter(page)
  await configure(page, 'demo')
  await start(page, { age: 16 })
  await expect(page.locator('.screen-encounter')).toContainText(
    'History-only encounter',
  )
  const initial = await grabReport(page)
  expect(initial.age).toBe(16)
  expect(initial.state).toBe('unsupported_scope')
  expect(initial.assessments).toHaveLength(0)
  await page
    .getByRole('navigation', { name: 'Encounter sections' })
    .getByRole('button', { name: 'History review' })
    .click()
  await page.getByRole('button', { name: 'Correct intake details' }).click()
  await page.getByLabel('Corrected age in years').fill('35')
  await page.getByRole('button', { name: 'Save intake correction' }).click()
  await expect(
    page.getByRole('heading', {
      name: 'When did the runny nose and sneezing begin?',
    }),
  ).toBeVisible()
  const corrected = await grabReport(page)
  expect(corrected.age).toBe(35)
  expect(corrected.historyRevision).toBeGreaterThan(initial.historyRevision)
  await remove(page)
})

const openSection = (page: Page, name: string) =>
  page
    .getByRole('navigation', { name: 'Encounter sections' })
    .getByRole('button', { name, exact: true })
    .click()
async function pasteReport(
  page: Page,
  text: string,
  identity: string,
  priority: string,
) {
  const upload = page.locator('.source-workspace form').first()
  await upload.getByRole('radio', { name: 'Paste text' }).check()
  await upload.getByLabel('Report text').fill(text)
  await upload
    .getByRole('combobox', { name: 'Whose report is this?' })
    .selectOption(identity)
  await upload
    .getByLabel('What prompted this test or visit?')
    .fill('Synthetic pre-visit test')
  await upload.getByLabel('What would you like to understand?').fill(priority)
  await upload.getByRole('button', { name: 'Read report locally' }).click()
  await expect(
    page.getByText('Original passages and transcriptions', { exact: true }),
  ).toBeVisible()
}
test('Report-first visit keeps an inspected source through questions, handoff, scoped access, correction and follow-up', async ({
  page,
  request,
}, info) => {
  await enter(page)
  await configure(page, 'demo')
  const id = await start(page, {
    reportFirst: true,
    age: 34,
    concern: 'It burns when I urinate.',
  })
  await pasteReport(
    page,
    'Patient name: Fictional Example\nSpecimen collection: 2026-11-18\nLeukocyte esterase | Trace',
    'unknown',
    'Understand the sample in the context of symptoms',
  )
  const row = page
    .locator('.passage-card')
    .filter({ hasText: 'Leukocyte esterase' })
  const originalDownload = page.waitForEvent('download')
  await page.getByRole('link', { name: 'Download original' }).click()
  const original = await originalDownload
  expect(original.suggestedFilename()).toBe('pasted-report.txt')
  expect(await readFile((await original.path())!, 'utf8')).toBe(
    'Patient name: Fictional Example\nSpecimen collection: 2026-11-18\nLeukocyte esterase | Trace',
  )
  await expect(row.getByRole('button', { name: 'Review entry' })).toBeDisabled()
  await page
    .getByRole('combobox', { name: 'Report belongs to' })
    .selectOption('self')
  await row.getByRole('button', { name: 'Review entry' }).click()
  await row.getByLabel('Printed name or finding').fill('Leukocyte esterase')
  await row.getByLabel('Result as printed').fill('Trace')
  await row.getByLabel('Specimen collection date').fill('2026-11-18')
  await row.getByLabel('I’m unsure of this transcription').uncheck()
  await row.getByRole('button', { name: 'Confirm transcription' }).click()
  await row.getByLabel('Select for model').check()
  await page
    .getByRole('button', { name: 'Preview selected extracts (1)' })
    .click()
  const disclosure = page.locator('.source-workspace .preview-box')
  await expect(disclosure).toContainText('Trace')
  await expect(disclosure).not.toContainText('Fictional Example')
  await disclosure.getByRole('checkbox').focus()
  await page.keyboard.press('Space')
  await disclosure
    .getByRole('button', { name: 'Approve selected extracts' })
    .click()
  await page.getByRole('button', { name: 'Continue to interview' }).click()
  await expect(
    page.getByRole('heading', {
      name: /Did your symptoms start before or after the sample/,
    }),
  ).toBeVisible()
  await page.getByText('Why this question?', { exact: true }).click()
  await page.getByRole('button', { name: 'Open the report passage' }).click()
  await expect(row).toBeFocused()
  await openSection(page, 'Interview')
  await page
    .getByRole('button', { name: 'Before the sample', exact: true })
    .click()
  await page.getByRole('button', { name: 'Save answer' }).click()
  await finishBackground(page)
  await reviewAndAssess(page)
  await expect(
    page.getByRole('heading', { name: 'Bladder infection', exact: true }),
  ).toBeVisible()
  const first = await grabReport(page)
  expect(first.assessments.at(-1).items[0].supporting).toContain(
    first.reportReviews.at(-1).factId,
  )
  await page
    .getByRole('button', { name: 'Clinician brief', exact: true })
    .click()
  await expect(page.locator('.screen-encounter article.brief')).toContainText(
    'Understand the sample',
  )
  await guidanceVisible(page, true)
  await page.emulateMedia({ media: 'print' })
  await expect(page.locator('.print-only')).toContainText('Pre-visit brief')
  await page.pdf({
    format: 'A4',
    path: info.outputPath('synthetic-clinician-brief.pdf'),
  })
  await page.emulateMedia({ media: 'screen' })
  await page.getByRole('button', { name: 'Preview FHIR document' }).click()
  await expect(
    page.getByText('Mapping limitations', { exact: true }),
  ).toBeVisible()
  await expect(
    page.getByRole('link', { name: 'Download FHIR R4' }),
  ).toBeVisible()
  await page.getByText('Read-only brief access', { exact: true }).click()
  await page.getByRole('button', { name: 'Grant access to this brief' }).click()
  const token = await page.getByLabel('One-time token').inputValue()
  const external = await request.get('/api/integration/brief', {
    headers: { Authorization: 'Bearer ' + token },
  })
  expect(external.status()).toBe(200)
  expect((await external.json()).id).toBe(id)
  await page.getByRole('button', { name: 'Revoke brief access' }).click()
  await expect(
    page.getByRole('button', { name: 'Grant access to this brief' }),
  ).toBeVisible()
  expect(
    (
      await request.get('/api/integration/brief', {
        headers: { Authorization: 'Bearer ' + token },
      })
    ).ok(),
  ).toBeFalsy()
  await openSection(page, 'Follow-up')
  await page.getByLabel('Outcome date').fill('2026-11-20')
  await page
    .getByLabel('What happened?')
    .fill('I reviewed the fictional sample at my appointment.')
  await page.getByRole('button', { name: 'Save follow-up' }).click()
  await expect(
    page
      .locator('.history-row')
      .filter({ hasText: 'I reviewed the fictional sample' }),
  ).toBeVisible()
  await openSection(page, 'Source reports')
  await row.getByRole('button', { name: 'Withdraw entry' }).click()
  await expect(page.locator('.screen-encounter')).toContainText(
    'The earlier estimate is outdated',
  )
  await page.getByRole('button', { name: 'Add report', exact: true }).click()
  await pasteReport(
    page,
    'Visit note: I had a blood clot.',
    'self',
    'Correct the family history',
  )
  const second = page
    .locator('.passage-card')
    .filter({ hasText: 'I had a blood clot' })
  await second.getByRole('button', { name: 'Review entry' }).click()
  await second.getByLabel('Printed name or finding').fill('Prior clot')
  await second
    .getByLabel('Who does the finding describe?')
    .selectOption('family')
  await second
    .getByLabel('Correction or disagreement')
    .fill('My father had a blood clot. This line concerns family history.')
  await second.getByRole('button', { name: 'Confirm transcription' }).click()
  await expect(second).toContainText('Family history')
  const after = await grabReport(page)
  expect(after.assessments[0]).toEqual(first.assessments[0])
  expect(after.outcomes).toHaveLength(1)
  expect(
    after.answers.some(
      (a: { sourceOutdated?: boolean; text: string }) =>
        a.sourceOutdated && a.text === 'Before the sample',
    ),
  ).toBeTruthy()
  expect(
    after.reportReviews.some(
      (r: { active: boolean; row: { subject: string } }) =>
        r.active && r.row.subject === 'family',
    ),
  ).toBeTruthy()
  await page.getByRole('button', { name: 'Provenance appendix' }).click()
  await expect(
    page.locator('.screen-encounter .report-document'),
  ).toContainText('My father had a blood clot')
  await guidanceVisible(page, true)
  await page.emulateMedia({ media: 'print' })
  await page.pdf({
    format: 'A4',
    path: info.outputPath('synthetic-provenance-appendix.pdf'),
  })
  await page.emulateMedia({ media: 'screen' })
  await expect(
    page.locator('.screen-encounter .report-document'),
  ).toContainText('Earlier answer linked to a superseded report entry')
  await openSection(page, 'Interview')
  await page
    .getByLabel('Your answer', { exact: true })
    .fill('I have an urgent concern now in this fictional case.')
  await page
    .getByLabel('I may be having a medical emergency right now.')
    .check()
  await page.getByRole('button', { name: 'Save answer' }).click()
  await page.getByRole('button', { name: 'Review my history' }).click()
  await reviewAndAssess(page)
  const corrected = await grabReport(page)
  expect(
    corrected.warnings.some((w: { active: boolean }) => w.active),
  ).toBeTruthy()
  expect(corrected.assessments[0]).toEqual(first.assessments[0])
  await page
    .getByRole('button', { name: 'Clinician brief', exact: true })
    .click()
  const brief = page.locator('.screen-encounter article.brief')
  await expect(brief).toContainText('Active urgent warning')
  await expect(brief).toContainText('My father had a blood clot')
  await expect(brief).toContainText('Open questions and conflicts')
  await page.emulateMedia({ media: 'print' })
  await expect(page.locator('.print-only article')).toHaveCount(1)
  await page.pdf({
    format: 'A4',
    path: info.outputPath('synthetic-corrected-urgent-brief.pdf'),
  })
  await page.emulateMedia({ media: 'screen' })
  await layoutFits(page)
  await remove(page)
})
test('Local evidence lookup supplies reference metadata and abstains without a match', async ({
  page,
}) => {
  await enter(page)
  await page
    .getByRole('navigation', { name: 'Workspace' })
    .getByRole('button', { name: 'Medical evidence', exact: true })
    .click()
  await page
    .getByRole('textbox', { name: 'Medical question', exact: true })
    .fill('common cold')
  await page.getByRole('button', { name: 'Search evidence' }).click()
  await page.getByRole('button', { name: 'Inspect citation' }).first().click()
  await expect(page.locator('.citation')).toContainText('MedlinePlus')
  await expect(page.locator('.citation')).toContainText('Applicability')
  await page
    .getByRole('textbox', { name: 'Medical question', exact: true })
    .fill('zzqxq unmapped-synthetic-context')
  await page.getByRole('button', { name: 'Search evidence' }).click()
  await expect(
    page.getByText('No supporting passage found', { exact: true }),
  ).toBeVisible()
  await page.getByRole('radio', { name: 'Browse passages' }).check()
  await page.getByRole('textbox', { name: 'Search terms' }).fill('cold')
  await page.getByRole('button', { name: 'Search evidence' }).click()
  await expect(
    page.getByRole('button', { name: 'Inspect citation' }).first(),
  ).toBeVisible()
})

test('Returning patient reviews a dated medication and groups two encounters into one episode', async ({
  page,
}, info) => {
  await enter(page)
  await configure(page, 'demo')
  const sourceId = await start(page),
    episode = 'Synthetic follow-up ' + randomUUID(),
    medication = 'Fictional study medication ' + info.project.name
  await page.getByRole('button', { name: 'Review history now' }).click()
  await page
    .getByRole('button', { name: 'Correct finding: Runny nose' })
    .click()
  await page
    .getByLabel('Corrected wording')
    .fill('I previously took a fictional study medication.')
  await page.getByLabel('Finding', { exact: true }).fill(medication)
  await page.getByRole('button', { name: 'Save correction' }).click()
  await page.getByRole('button', { name: 'Confirm my history' }).click()
  await page.getByRole('button', { name: 'Reconcile earlier history' }).click()
  await page
    .getByText('Group this visit into an episode', { exact: true })
    .click()
  await page.getByLabel('Episode name', { exact: true }).fill(episode)
  await page.getByRole('button', { name: 'Save episode' }).click()
  await expect(page.getByText('Current episode · ' + episode)).toBeVisible()
  const source = await (
    await page.request.get('/api/encounters/' + sourceId)
  ).json()
  await page.getByRole('button', { name: 'Pause and return' }).click()
  const id = await start(page, {
    concern: 'Synthetic follow-up visit ' + info.project.name,
    age: 35,
  })
  await openSection(page, 'History review')
  await page.getByRole('button', { name: 'Reconcile earlier history' }).click()
  await page
    .getByText('Group this visit into an episode', { exact: true })
    .click()
  await page
    .getByRole('combobox', { name: 'Episode action' })
    .selectOption('join')
  await page
    .getByRole('combobox', { name: 'Earlier episode' })
    .selectOption(sourceId)
  await page.getByRole('button', { name: 'Save episode' }).click()
  await expect(page.getByText('Current episode · ' + episode)).toBeVisible()
  const finding = page
    .locator('.timeline-entry .history-row')
    .filter({
      has: page.getByRole('heading', { name: medication, exact: true }),
    })
    .first()
  await finding.getByRole('button', { name: 'Reconcile this finding' }).click()
  await finding
    .getByRole('combobox', { name: 'History category' })
    .selectOption('medication')
  await finding
    .getByRole('combobox', { name: 'Current status' })
    .selectOption('no_longer_applies')
  await finding.getByLabel('Reconciliation date').fill('2026-11-21')
  const note = 'I am no longer taking the fictional study medication.'
  await finding.getByLabel('How does this apply now?').fill(note)
  await finding.getByRole('button', { name: 'Add to current history' }).click()
  await expect(
    page.getByRole('button', { name: 'Confirm my history' }),
  ).toBeEnabled()
  await page.getByRole('button', { name: 'Confirm my history' }).click()
  const result = await grabReport(page)
  expect(result.id).toBe(id)
  expect(result.episodeId).toBe(source.episodeId)
  expect(
    result.facts.find(
      (f: { origin?: { note: string } }) => f.origin?.note === note,
    ).origin,
  ).toMatchObject({
    category: 'medication',
    reconciliationStatus: 'no_longer_applies',
    encounterId: sourceId,
    date: '2026-11-21',
  })
  expect(
    (await (await page.request.get('/api/encounters/' + sourceId)).json())
      .facts,
  ).toEqual(source.facts)
  await page
    .getByRole('navigation', { name: 'Workspace' })
    .getByRole('button', { name: 'Personal timeline' })
    .click()
  await page.getByLabel('Search timeline').fill(episode)
  await page
    .getByRole('combobox', { name: 'Show', exact: true })
    .selectOption('medication')
  const group = page
    .locator('.episode')
    .filter({ has: page.getByRole('heading', { name: episode, exact: true }) })
  await expect(group).toContainText('2 encounters in this episode')
  await expect(group).toContainText(note)
  await group.getByRole('button', { name: 'Open encounter' }).first().click()
  await remove(page)
  await page
    .getByRole('navigation', { name: 'Workspace' })
    .getByRole('button', { name: 'Personal timeline' })
    .click()
  await page.getByLabel('Search timeline').fill(episode)
  await group.getByRole('button', { name: 'Open encounter' }).click()
  await remove(page)
})

test('Throttled connection retains the draft and retries a committed answer exactly once', async ({
  page,
}, info) => {
  await enter(page)
  await configure(page, 'demo')
  const id = await start(page)
  const before = await (await page.request.get('/api/encounters/' + id)).json()
  const client = await page.context().newCDPSession(page)
  await client.send('Network.enable')
  await client.send('Network.emulateNetworkConditions', {
    offline: false,
    latency: 150,
    downloadThroughput: 16000,
    uploadThroughput: 8000,
    connectionType: 'cellular3g',
  })
  const url = '/api/encounters/' + id + '/actions'
  await page.route(
    '**' + url,
    async (route) => {
      const result = await route.fetch()
      expect(result.status()).toBe(200)
      await route.abort('connectionreset')
    },
    { times: 1 },
  )
  const begin = Date.now()
  await page.getByRole('button', { name: 'Yesterday', exact: true }).click()
  await page.getByRole('button', { name: 'Save answer' }).click()
  await expect(
    page.getByRole('alert').filter({ hasText: 'Reconnect, then retry' }),
  ).toBeVisible()
  await expect(
    page.getByRole('button', { name: 'Yesterday', exact: true }),
  ).toHaveAttribute('aria-pressed', 'true')
  await page.getByRole('button', { name: 'Save answer' }).click()
  await expect(
    page.getByRole('button', { name: 'After being outdoors', exact: true }),
  ).toBeVisible()
  const after = await (await page.request.get('/api/encounters/' + id)).json()
  expect(
    after.answers.filter((a: { text: string }) => a.text === 'Yesterday'),
  ).toHaveLength(1)
  expect(after.historyRevision).toBe(before.historyRevision + 1)
  await writeFile(
    info.outputPath('low-bandwidth.json'),
    JSON.stringify({
      configuredLatencyMs: 150,
      downloadBytesPerSecond: 16000,
      uploadBytesPerSecond: 8000,
      elapsedMs: Date.now() - begin,
      committedAnswers: 1,
      retries: 1,
    }),
  )
  await client.send('Network.emulateNetworkConditions', {
    offline: false,
    latency: 0,
    downloadThroughput: -1,
    uploadThroughput: -1,
  })
  await remove(page)
})

test('Regional equivalents preserve reported body measurements and leave ambiguous input untouched', () => {
  const us = {
      locale: 'en-US',
      units: 'us-customary',
      emergency: '',
      careDestination: '',
    },
    metric = { ...us, units: 'metric' },
    row = { name: 'Body weight', value: '70', units: 'kg' }
  expect(regionalMeasurement(row, us)).toBe(
    'Body weight ≈ 154.3 lb (rounded to 0.1 lb)',
  )
  expect(row.value).toBe('70')
  expect(
    regionalMeasurement(
      { name: 'Standing height', value: '64', units: 'in' },
      metric,
    ),
  ).toContain('162.6 cm')
  expect(
    regionalMeasurement(
      { name: 'Body temperature', value: '37.5', units: '°C' },
      us,
    ),
  ).toContain('99.5 °F')
  for (const fields of [
    { name: 'Urine creatinine' },
    { value: '<70' },
    { value: '65–70' },
    { units: 'mg/dL' },
    { name: 'Sample temperature' },
  ])
    expect(regionalMeasurement({ ...row, ...fields }, us)).toBe('')
  expect(regionalMeasurement(row, { ...us, units: 'as-reported' })).toBe('')
})

test('Mid-interview PDF review opens the exact printed page on demand and keeps reported units', async ({
  page,
  context,
}) => {
  const source = await context.newPage()
  await source.setContent(
    '<html><body><p style="break-after:page">Synthetic visit report, page one.</p><p>Body weight: 70 kg</p></body></html>',
  )
  const contents = await source.pdf({ format: 'A4' })
  await source.close()
  // Vary only the display preference; source, extraction and raster use the real QA server.
  await page.route('**/api/region', async (route) => {
    const response = await route.fetch()
    await route.fulfill({
      response,
      json: {
        ...(await response.json()),
        locale: 'en-US',
        units: 'us-customary',
        careDestination: 'Fictional regional care desk',
      },
    })
  })
  const rasterRequests: string[] = []
  page.on('request', (r) => {
    if (/\/documents\/[^/]+\/page\/\d+$/.test(new URL(r.url()).pathname))
      rasterRequests.push(r.url())
  })
  await enter(page)
  await configure(page, 'demo')
  await start(page)
  await openSection(page, 'Source reports')
  await page.getByLabel('Report file').setInputFiles({
    name: 'synthetic-vitals.pdf',
    mimeType: 'application/pdf',
    buffer: contents,
  })
  await page
    .getByRole('combobox', { name: 'Whose report is this?' })
    .selectOption('self')
  await page.getByRole('button', { name: 'Read report locally' }).click()
  const row = page
    .locator('.passage-card')
    .filter({ hasText: 'Body weight: 70 kg' })
  await expect(row).toContainText('Page 2')
  expect(rasterRequests).toHaveLength(0)
  await row.getByRole('button', { name: 'View original page' }).focus()
  await page.keyboard.press('Enter')
  const img = row.getByRole('img', {
    name: 'synthetic-vitals.pdf · original page 2',
  })
  await expect
    .poll(() => img.evaluate((el) => (el as HTMLImageElement).naturalWidth))
    .toBeGreaterThan(200)
  expect(rasterRequests[0]).toMatch(/\/page\/2$/)
  await row.getByRole('button', { name: 'Enlarge page' }).focus()
  await page.keyboard.press('Enter')
  await expect(
    row.getByRole('button', { name: 'Fit page to width' }),
  ).toHaveAttribute('aria-pressed', 'true')
  await row.getByRole('button', { name: 'Close original page' }).click()
  await expect(img).toHaveCount(0)
  await row.getByRole('button', { name: 'Review entry' }).click()
  await row.getByLabel('I’m unsure of this transcription').uncheck()
  await row.getByRole('button', { name: 'Confirm transcription' }).click()
  await expect(row).toContainText('Body weight ≈ 154.3 lb')
  await expect(row).toContainText('Body weight: 70 kg')
  await page.getByRole('button', { name: 'Browse original pages' }).click()
  await page.getByLabel('Page number', { exact: true }).fill('2')
  await page.getByRole('button', { name: 'Show page', exact: true }).click()
  await expect
    .poll(() =>
      page
        .getByRole('img', { name: 'synthetic-vitals.pdf · original page 2' })
        .evaluate((el) => (el as HTMLImageElement).naturalHeight),
    )
    .toBeGreaterThan(200)
  await grabReport(page)
  await page
    .getByRole('button', { name: 'Clinician brief', exact: true })
    .click()
  await expect(page.locator('.screen-encounter article.brief')).toContainText(
    'Fictional regional care desk',
  )
  await expect(page.locator('.screen-encounter article.brief')).toContainText(
    'Body weight ≈ 154.3 lb',
  )
  await remove(page)
})

test('Model guidance is present before JavaScript and remains visible during loading, setup and expanded notice', async ({
  page,
  browser,
  baseURL,
}, info) => {
  const plain = await browser.newContext({
    javaScriptEnabled: false,
    viewport: info.project.use.viewport,
  })
  try {
    const startup = await plain.newPage()
    await startup.goto(baseURL!)
    await expect(
      startup.getByRole('note', { name: 'Model guidance' }),
    ).toHaveText(guidance)
    await expect(startup.locator('#preview-notice')).toContainText('a ctlplne studio product')
    await startup.goto(new URL('/privacy.html', baseURL).href)
    await expect(startup.getByRole('heading', {name: 'Where your healthctl data goes'})).toBeVisible()
  } finally {
    await plain.close()
  }
  let release!: () => void
  const gate = new Promise<void>((r) => {
    release = r
  })
  await page.route(
    '**/api/session',
    async (route) => {
      const response = await route.fetch()
      await gate
      await route.fulfill({
        response,
        json: {
          ...(await response.json()),
          authenticated: false,
          setupRequired: true,
        },
      })
    },
    { times: 1 },
  )
  await page.goto('/')
  await expect(
    page.getByText('Opening your workspace…', { exact: true }),
  ).toBeVisible()
  await guidanceVisible(page)
  release()
  await expect(
    page.getByRole('button', { name: 'Create owner account' }),
  ).toBeVisible()
  await guidanceVisible(page, true)
  await enter(page)
  const session = await (await page.request.get('/api/session')).json()
  expect(session.modelGuidance).toBe(guidance)
  await page.getByRole('button', { name: 'Experimental · Unreviewed' }).click()
  await expect(page.getByRole('button', { name: 'Close notice' })).toBeVisible()
  await guidanceVisible(page, true)
  for (const name of [
    'Personal timeline',
    'Medical evidence',
    'Encounter journal',
  ]) {
    await page
      .getByRole('navigation', { name: 'Workspace' })
      .getByRole('button', { name, exact: true })
      .click()
    await guidanceVisible(page, true)
  }
})

test('Patient adds medicine in their own words, resumes at allergies and flags an emergency without answering it', async ({
  page,
}, info) => {
  const concern =
    'Synthetic interview about a change ' +
    info.project.name +
    ' ' +
    randomUUID()
  const medicine = 'I take the fictional blue tablet with breakfast.'
  await enter(page)
  await configure(page, 'compatible', 'fixture-ok')
  await start(page, { concern, age: 42 })
  await expect(
    page.getByRole('button', { name: 'Yesterday', exact: true }),
  ).toHaveCount(0)
  await page.getByRole('button', { name: 'Show answer choices' }).click()
  await page.getByRole('button', { name: 'Yesterday', exact: true }).click()
  await page.getByRole('button', { name: 'Save answer' }).click()
  await page
    .getByRole('button', { name: 'Add a medicine or supplement' })
    .click()
  const input = page.getByRole('textbox', { name: 'Your answer', exact: true })
  await expect(input).toBeFocused()
  await input.fill(medicine)
  await page.getByRole('button', { name: 'Save answer' }).click()
  const allergy = page.getByRole('heading', {
    name: 'What allergies do you know you have?',
  })
  await expect(allergy).toBeVisible()
  await page.getByRole('button', { name: 'Pause and return' }).click()
  await page.reload()
  await page.getByRole('button', { name: new RegExp(concern) }).click()
  await expect(allergy).toBeVisible()
  await page
    .getByLabel('I may be having a medical emergency right now.')
    .check()
  await page.getByRole('button', { name: 'Save emergency warning' }).click()
  await expect(
    page.getByRole('heading', { name: 'Routine questions are paused' }),
  ).toBeVisible()
  const exported = await grabReport(page)
  expect(exported.answers).toContainEqual(
    expect.objectContaining({
      text: medicine,
      disposition: 'provided',
      active: true,
    }),
  )
  expect(
    exported.answers.some((a: { question: string }) =>
      a.question.includes('What allergies'),
    ),
  ).toBe(false)
  expect(exported.warnings.some((w: { active: boolean }) => w.active)).toBe(
    true,
  )
  await remove(page)
})

test('A slow model leaves intake immediately and completes the same saved encounter after reopening', async ({
  page,
}, info) => {
  const concern =
    'Fictional slow-model check ' + info.project.name + ' ' + randomUUID()
  await enter(page)
  await configure(page, 'compatible', 'fixture-slow-interview')
  const id = await start(page, { concern, age: 42 })
  await expect(
    page.getByText('Preparing your next interview question', { exact: true }),
  ).toBeVisible()
  await expect(page.getByRole('button', { name: 'Save answer' })).toHaveCount(0)
  await page.getByRole('button', { name: 'Review saved history now' }).click()
  await expect(
    page.getByRole('button', { name: 'Confirm my history' }),
  ).toBeDisabled()
  await page
    .getByRole('navigation', { name: 'Encounter sections' })
    .getByRole('button', { name: 'Interview', exact: true })
    .click()
  await page
    .getByRole('button', { name: 'Pause and return to journal', exact: true })
    .click()
  await page.getByRole('button', { name: new RegExp(concern) }).click()
  await expect(
    page.getByRole('heading', {
      name: 'When did you first notice the change?',
    }),
  ).toBeVisible({ timeout: 10000 })
  expect(
    (await (await page.request.get('/api/encounters/' + id)).json()).turns,
  ).toHaveLength(1)
  await remove(page)
})

test('A patient records an episode, reviews dictated words and selectively includes it in a return visit with a witness', async ({
  page,
  context,
}, info) => {
  await enter(page)
  await configure(page, 'compatible', 'fixture-ok')
  const suffix = randomUUID()
  const episode = 'My left hand shook for 15 minutes. ' + suffix
  await page
    .getByRole('button', { name: 'It happened again', exact: true })
    .click()
  await page
    .getByRole('button', { name: 'Tell me what happened aloud' })
    .click()
  const mode = page.getByLabel('Transcription method')
  await expect(mode.locator('option[value="openai"]')).toHaveJSProperty(
    'disabled',
    true,
  )
  await context.grantPermissions(['microphone'])
  let posted = false
  await page.route('**/api/voice?destination=local', async (route) => {
    const pcm = route.request().postDataBuffer()
    expect(pcm?.subarray(0, 4).toString()).toBe('RIFF')
    expect(pcm!.length).toBeGreaterThan(44)
    expect(route.request().headers()['x-audio-consent']).toBeUndefined()
    posted = true
    await route.fulfill({
      json: { draft: 'My right hand shook for 15 minutes.' },
    })
  })
  await page.getByRole('button', { name: 'Record up to 90 seconds' }).click()
  await expect(
    page.getByRole('button', { name: 'Stop recording' }),
  ).toBeEnabled()
  await page.waitForTimeout(350) // Accumulate PCM from the simulated microphone.
  await page.getByRole('button', { name: 'Stop recording' }).click()
  await page.getByRole('button', { name: 'Transcribe locally' }).click()
  await expect(page.getByLabel('Transcript to review')).toHaveValue(
    /right hand/,
  )
  expect(posted).toBe(true)
  await expect(page.getByLabel('What happened?', { exact: true })).toHaveValue(
    '',
  )
  await expect(
    page.getByRole('button', { name: 'Use reviewed words' }),
  ).toBeDisabled()
  await page.getByLabel('Transcript to review').fill(episode)
  await page.getByLabel('I checked names').check()
  await page.getByRole('button', { name: 'Use reviewed words' }).click()
  await page.getByLabel('These times are approximate').check()
  await page.getByRole('button', { name: 'Save episode locally' }).click()
  await expect(page.getByText(episode, { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Back to journal' }).click()
  await page.getByRole('button', { name: 'Start a return visit' }).click()
  await page.getByLabel('Chief concern').fill('Fictional repeated hand shaking')
  await page
    .getByLabel('What question do you want this visit to answer?')
    .fill('When did the pattern change?')
  await page.getByLabel('Interview length').selectOption('short')
  await page.getByRole('checkbox', { name: new RegExp(suffix) }).check()
  await page.getByLabel('Include my selections').check()
  await page.getByLabel('Age in years').fill('48')
  await page
    .getByRole('group', { name: 'Is the concern related to pregnancy?' })
    .getByLabel('No', { exact: true })
    .check()
  await page
    .getByRole('group', {
      name: 'Are you reporting a possible emergency right now?',
    })
    .getByLabel('No', { exact: true })
    .check()
  await page.getByLabel('I understand the experimental-use notice').check()
  const creation = page.waitForResponse(
    (r) =>
      new URL(r.url()).pathname === '/api/encounters' &&
      r.request().method() === 'POST',
  )
  await page
    .getByRole('button', { name: 'Start interview', exact: true })
    .click()
  const created = await (await creation).json()
  expect(
    created.answers.find((a: { kind: string }) => a.kind === 'episode_log')
      .text,
  ).toBe(episode)
  await page
    .getByRole('button', { name: 'Visit preparation', exact: true })
    .click()
  await page
    .getByLabel('What would you like to record?')
    .selectOption('witness')
  const sent = page.waitForEvent('download')
  await page
    .getByRole('button', { name: 'Download blank witness form' })
    .click()
  const invitation = await sent
  await invitation.saveAs(info.outputPath('witness.html'))
  const html = await readFile(info.outputPath('witness.html'), 'utf8')
  expect(html).not.toContain(episode)
  const offline = await context.newPage()
  await offline.setContent(html)
  await offline
    .getByLabel('Your name or relationship')
    .fill('Fictional observer')
  await offline
    .getByLabel('In your own words')
    .fill('I saw their left hand move.')
  const received = offline.waitForEvent('download')
  await offline.getByRole('button', { name: 'Save response file' }).click()
  await (await received).saveAs(info.outputPath('reply.json'))
  await offline.close()
  await page
    .getByLabel('Open returned response')
    .setInputFiles(info.outputPath('reply.json'))
  await expect(page.getByLabel('Witness’s own words')).toHaveValue(
    'I saw their left hand move.',
  )
  await page.getByLabel('Include this attributed account').check()
  await page.getByRole('button', { name: 'Save reviewed entry' }).click()
  await expect(page.getByText('Saved to this encounter.')).toBeVisible()
  const after = await (
    await page.request.get('/api/encounters/' + created.id + '/brief')
  ).json()
  expect(after.agenda).toBe('When did the pattern change?')
  expect(
    after.changes.some(
      (a: { detail: { speaker?: string } }) =>
        a.detail?.speaker === 'Fictional observer',
    ),
  ).toBe(true)
})

test('An unavailable local speech engine leaves cloud submission under per-recording control', async ({
  page,
  context,
}) => {
  await enter(page)
  await context.grantPermissions(['microphone'])
  await page.route('**/api/voice', (route) =>
    route.fulfill({ json: { local: false, openai: true, chatgptPlan: false } }),
  )
  let attempts = 0
  await page.route('**/api/voice?destination=openai', (route) => {
    attempts++
    expect(route.request().headers()['x-audio-consent']).toBe('openai-api')
    expect(route.request().postDataBuffer()?.subarray(0, 4).toString()).toBe(
      'RIFF',
    )
    return attempts === 1
      ? route.fulfill({
          status: 429,
          json: { error: 'Synthetic busy service. Retry the recording.' },
        })
      : route.fulfill({
          json: { draft: 'I took the fictional tablet on Friday.' },
        })
  })
  await page
    .getByRole('button', { name: 'It happened again', exact: true })
    .click()
  await page
    .getByRole('button', { name: 'Tell me what happened aloud' })
    .click()
  await expect(
    page.getByText('Whisper is unavailable on this installation.', {
      exact: false,
    }),
  ).toBeVisible()
  await expect(
    page.getByRole('button', { name: 'Record up to 90 seconds' }),
  ).toBeDisabled()
  await page.getByLabel('Transcription method').selectOption('openai')
  await page.getByRole('button', { name: 'Record up to 90 seconds' }).click()
  await expect(
    page.getByRole('button', { name: 'Stop recording' }),
  ).toBeEnabled()
  await page.waitForTimeout(300)
  await page.getByRole('button', { name: 'Stop recording' }).click()
  const send = page.getByRole('button', {
    name: 'Send this recording to the OpenAI API',
  })
  await expect(send).toBeEnabled()
  expect(attempts).toBe(0)
  await send.click()
  await expect(
    page.getByRole('alert').filter({ hasText: 'Synthetic busy' }),
  ).toBeVisible()
  expect(attempts).toBe(1)
  await send.click()
  await expect(page.getByLabel('Transcript to review')).toHaveValue(
    /tablet on Friday/,
  )
  await expect(
    page.getByRole('button', { name: 'Use reviewed words' }),
  ).toBeDisabled()
  await expect(page.getByLabel('What happened?', { exact: true })).toHaveValue(
    '',
  )
  expect(attempts).toBe(2)
})
