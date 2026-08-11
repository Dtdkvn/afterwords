import { defineConfig, devices } from '@playwright/test'

const externalBaseURL = process.env.AFTERWORDS_E2E_BASE_URL
const baseURL = externalBaseURL || 'http://127.0.0.1:18080'

export default defineConfig({
  testDir: './tests',
  timeout: 90_000,
  expect: { timeout: 15_000 },
  fullyParallel: false,
  retries: process.env.CI ? 1 : 0,
  workers: 1,
  reporter: process.env.CI ? [['github'], ['html', { open: 'never' }]] : [['list'], ['html', { open: 'never' }]],
  use: {
    baseURL,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: process.env.CI ? 'retain-on-failure' : 'off',
    ...devices['Desktop Chrome'],
    channel: process.platform === 'win32' ? 'chrome' : undefined,
  },
  webServer: externalBaseURL ? undefined : {
    command: 'go run ./cmd/afterwords',
    cwd: '..',
    url: `${baseURL}/healthz`,
    reuseExistingServer: false,
    timeout: 120_000,
    env: {
      ...process.env,
      PORT: '18080',
      BASE_URL: baseURL,
      DB_PATH: 'file:afterwords-e2e?mode=memory&cache=shared',
      SMTP_HOST: process.env.SMTP_HOST ?? '',
      SMTP_PORT: process.env.SMTP_PORT ?? '1025',
      SMTP_TLS_POLICY: process.env.SMTP_TLS_POLICY ?? 'mandatory',
      TIME_SCALE: process.env.TIME_SCALE ?? '86400',
      SECURE_COOKIES: 'false',
    },
  },
})
