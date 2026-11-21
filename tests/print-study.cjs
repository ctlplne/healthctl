// Generate matched fictional handouts through the ordinary authenticated print views.
const { chromium } = require('playwright');
const { readFileSync } = require('node:fs');

(async () => {
  const input = JSON.parse(readFileSync(0, 'utf8'));
  const browser = await chromium.launch({
    executablePath: '/ms-playwright/chromium-1193/chrome-linux/chrome',
    args: ['--no-sandbox'],
  });
  try {
    const viewports = input.restored ? [{ width: 1360, height: 940 }, { width: 390, height: 844 }] : [undefined];
    for (const viewport of viewports) {
      const context = await browser.newContext({ viewport });
      const base = 'http://127.0.0.1:8080';
      const login = await context.request.post(base + '/api/login', { data: input.owner });
      if (!login.ok()) throw new Error('Synthetic study owner login failed');
      const page = await context.newPage();
      await page.goto(base);
      await page.locator('.journal-row').filter({ hasText: input.concern }).click();
      await page.getByRole('navigation', { name: 'Encounter sections' }).getByRole('button', { name: 'Report', exact: true }).click();
      if (input.restored) {
        const report = page.locator('.report-document:visible');
        await report.locator('.report-concern').filter({ hasText: input.concern }).waitFor();
        await report.getByText(input.warning, { exact: false }).first().waitFor();
        await context.close();
        continue;
      }
      if (input.candidate) {
        await page.getByRole('button', { name: 'Clinician brief', exact: true }).click();
      }
      await page.emulateMedia({ media: 'print' });
      await page.locator('.print-only').getByText(input.concern, { exact: false }).first().waitFor({ state: 'visible' });
      await page.evaluate(() => document.fonts.ready);
      process.stdout.write(await page.pdf({ format: 'A4', preferCSSPageSize: true, printBackground: true }));
    }
  } finally {
    await browser.close();
  }
})().catch((err) => { console.error(err.message); process.exitCode = 1; });
