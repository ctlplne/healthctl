import { defineConfig } from '@playwright/test'
const baseURL = process.env.CLINICAL_E2E_URL
if (!baseURL)
  throw new Error('Set CLINICAL_E2E_URL to the isolated local QA installation.')
export default defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  workers: 1,
  retries: 0,
  timeout: 90000,
  use: {
    baseURL,
    headless: true,
    actionTimeout: 10000,
    launchOptions: {
      args: ['--use-fake-device-for-media-stream'],
      executablePath:
        process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH ||
        process.env.CHROMIUM_PATH,
    },
    screenshot: 'only-on-failure',
    trace: 'off',
  },
  projects: [
    { name: 'desktop', use: { viewport: { width: 1360, height: 940 } } },
    {
      name: 'mobile',
      use: {
        viewport: { width: 390, height: 844 },
        isMobile: true,
        hasTouch: true,
      },
    },
  ],
})
