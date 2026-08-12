import { expect, test, type Page } from '@playwright/test'

const ownerEmail = 'owner-e2e@example.com'
const ownerPassword = 'correct horse battery staple'
const vaultPassphrase = 'a quiet cedar bridge at sunrise'
const recipientEmail = 'recipient-e2e@example.com'

test('owner to Recovery Kits to recipient plaintext @demo', async ({ page, context, request, baseURL }) => {
  page.on('pageerror', (error) => console.error('browser page error:', error.message))
  page.on('console', (message) => { if (message.type() === 'error') console.error('browser console:', message.text()) })
  await page.goto('/')
  await authenticate(page)
  await page.getByTestId('new-switch').click()
  const uniqueName = `Family letters ${Date.now()}`
  await page.getByTestId('switch-name').fill(uniqueName)
  await page.getByTestId('interval-days').fill('1')
  await page.getByTestId('grace-days').fill('0')
  const createdSwitch = page.waitForResponse((response) => response.url().endsWith('/api/switches') && response.request().method() === 'POST')
  await page.getByTestId('wizard-next').click()
  const createdPayload = await (await createdSwitch).json() as { switch: { id: string } }

  await page.getByTestId('vault-title').fill('For a rainy afternoon')
  await page.getByTestId('vault-message').fill('## You found it\n\nThe important thing is simple: **you were loved, every day.**')
  await page.getByTestId('vault-passphrase').fill(vaultPassphrase)
  await page.getByLabel('Repeat passphrase').fill(vaultPassphrase)

  // TIME_SCALE can make a one-day switch overdue while Argon2 is deriving the
  // browser key. A dashboard action races with the wizard only in accelerated
  // demo mode, so keep this flow active until setup has committed atomically.
  await browserAPI(page, `/api/switches/${createdPayload.switch.id}/checkin`, { method: 'POST', json: {} })
  await page.getByTestId('encrypt-vault').click()

  await page.getByLabel('Name').nth(0).fill('Morgan Recipient')
  await page.getByLabel('Email').nth(0).fill(recipientEmail)
  await page.getByLabel('Name').nth(1).fill('Taylor Trustee')
  await page.getByLabel('Email').nth(1).fill('taylor@example.com')
  await page.getByLabel('Name').nth(2).fill('Jordan Trustee')
  await page.getByLabel('Email').nth(2).fill('jordan@example.com')
  await page.getByTestId('prepare-kits').click()

  await expect(page.locator('.kit-row')).toHaveCount(2)
  const shares: string[] = []
  for (let index = 0; index < 2; index++) {
    const kitOpened = context.waitForEvent('page')
    const delivered = page.waitForResponse((response) => response.url().includes('/delivered') && response.request().method() === 'POST')
    await page.locator('.kit-row').nth(index).click()
    const kitPage = await kitOpened
    await delivered
    shares.push((await kitPage.locator('.share-panel code').textContent())!.trim())
    await expect(kitPage.getByText('If you’re reading this')).toBeVisible()
    await expect(kitPage).toHaveURL(/\/kit$/u)
    await kitPage.close()
  }
  await page.getByTestId('finish-wizard').click()
  await expect(page.getByText(uniqueName)).toBeVisible()

  let releaseURL = ''
  const mailpitURL = process.env.MAILPIT_URL
  if (mailpitURL) {
    await expect.poll(async () => {
      await page.reload()
      return page.locator('.state-badge.released').count()
    }, { timeout: 25_000, intervals: [1000] }).toBeGreaterThan(0)
    releaseURL = await releaseURLFromMailpit(request, mailpitURL, recipientEmail)
  } else {
    await page.getByRole('button', { name: 'Switch options' }).click()
    const responsePromise = page.waitForResponse((response) => response.url().includes('/firedrill') && response.request().method() === 'POST')
    await page.getByRole('button', { name: 'Run fire drill' }).click()
    const response = await responsePromise
    const payload = await response.json() as { previewURL: string }
    releaseURL = payload.previewURL
  }

  const recipientPage = await context.newPage()
  await recipientPage.goto(releaseURL.startsWith('http') ? releaseURL : new URL(releaseURL, baseURL).toString())
  await expect(recipientPage.getByText('Someone who cared about you')).toBeVisible()

  await recipientPage.getByTestId('release-shares').fill('aw1.not-a-share-from-this-release')
  await recipientPage.getByTestId('open-release').click()
  await expect(recipientPage.getByRole('alert')).toContainText('does not belong to this release')

  await recipientPage.getByTestId('release-shares').fill(`${shares[0]}\n${shares[0]}`)
  await recipientPage.getByTestId('open-release').click()
  await expect(recipientPage.getByRole('alert')).toContainText('1 more valid share is needed')

  await recipientPage.getByTestId('release-shares').fill(shares.join('\n'))
  await recipientPage.getByTestId('open-release').click()
  await expect(recipientPage.getByTestId('decrypted-message')).toContainText('you were loved, every day')
})

test('pause, resume, and fire drill preserve the real switch state', async ({ page, context }) => {
  await page.goto('/')
  await authenticate(page)
  const uniqueName = `State controls ${Date.now()}`
  const created = await browserAPI<{ switch: { id: string } }>(page, '/api/switches', {
    method: 'POST',
    json: { name: uniqueName, intervalDays: 30, graceDays: 5, reminders: { beforeDays: [3, 1], overdueDays: [0, 1, 2] } },
  })
  expect(created.status).toBe(201)
  const switchID = created.body.switch.id
  await page.reload()

  const card = page.locator('article.switch-card').filter({ hasText: uniqueName })
  await expect(card).toBeVisible()
  await card.getByRole('button', { name: 'Pause' }).click()
  await expect(card.locator('.state-badge')).toHaveText('PAUSED')
  await card.getByRole('button', { name: 'Resume' }).click()
  await expect(card.locator('.state-badge')).toHaveText('ACTIVE')

  const before = await browserAPI<{ switch: { state: string; nextDeadline: string } }>(page, `/api/switches/${switchID}`)
  const popupPromise = context.waitForEvent('page')
  await card.getByRole('button', { name: 'Switch options' }).click()
  await card.getByRole('button', { name: 'Run fire drill' }).click()
  const rehearsal = await popupPromise
  await expect(rehearsal.getByText('Safe rehearsal:')).toBeVisible()
  await expect(rehearsal.getByText('Someone who cared about you')).toBeVisible()

  const after = await browserAPI<{ switch: { state: string; nextDeadline: string } }>(page, `/api/switches/${switchID}`)
  expect(after.body.switch.state).toBe(before.body.switch.state)
  expect(after.body.switch.nextDeadline).toBe(before.body.switch.nextDeadline)
  const audit = await browserAPI<{ entries: Array<{ event: string }> }>(page, `/api/switches/${switchID}/audit`)
  expect(audit.body.entries.map((entry) => entry.event)).toEqual(expect.arrayContaining(['switch.paused', 'switch.resumed', 'fire_drill.started', 'email.queued']))
})

async function authenticate(page: Page) {
  const setup = page.getByText('Create your private space')
  const login = page.getByText('Open your dashboard')
  await expect(setup.or(login)).toBeVisible()
  await page.getByTestId('auth-email').fill(ownerEmail)
  await page.getByTestId('auth-password').fill(ownerPassword)
  await page.getByTestId('auth-submit').click()
  await expect(page.getByText('Good to see you.')).toBeVisible()
}

async function browserAPI<T>(page: Page, path: string, options: { method?: string; json?: unknown } = {}): Promise<{ status: number; body: T }> {
  return page.evaluate(async ({ requestPath, method, json }) => {
    const bootstrap = await fetch('/api/bootstrap', { credentials: 'same-origin' })
    const credentials = await bootstrap.json() as { csrfToken?: string }
    const response = await fetch(requestPath, {
      method: method ?? 'GET',
      credentials: 'same-origin',
      headers: json === undefined ? undefined : { 'Content-Type': 'application/json', 'X-CSRF-Token': credentials.csrfToken ?? '' },
      body: json === undefined ? undefined : JSON.stringify(json),
    })
    return { status: response.status, body: await response.json() as T }
  }, { requestPath: path, method: options.method, json: options.json })
}

async function releaseURLFromMailpit(request: import('@playwright/test').APIRequestContext, mailpitURL: string, recipient: string): Promise<string> {
  return expect.poll(async () => {
    const listResponse = await request.get(`${mailpitURL}/api/v1/messages`)
    if (!listResponse.ok()) return ''
    const listing = await listResponse.json() as { messages?: Array<{ ID: string; Subject: string; To?: Array<{ Address?: string }> }> }
    const message = listing.messages?.find((candidate) => candidate.Subject.includes('Someone left you something') && candidate.To?.some((to) => to.Address === recipient))
    if (!message) return ''
    const detailResponse = await request.get(`${mailpitURL}/api/v1/message/${message.ID}`)
    if (!detailResponse.ok()) return ''
    const detail = await detailResponse.json() as { HTML?: string; Text?: string }
    const content = `${detail.HTML ?? ''}\n${detail.Text ?? ''}`.replaceAll('&amp;', '&')
    return content.match(/https?:\/\/[^\s"'<>]+\/r\/[^\s"'<>?]+/u)?.[0] ?? ''
  }, { timeout: 30_000, intervals: [1000] }).not.toBe('').then(() => {
    // expect.poll does not expose its last value, so perform one deterministic final lookup.
    return extractLatestReleaseURL(request, mailpitURL, recipient)
  })
}

async function extractLatestReleaseURL(request: import('@playwright/test').APIRequestContext, mailpitURL: string, recipient: string): Promise<string> {
  const listing = await (await request.get(`${mailpitURL}/api/v1/messages`)).json() as { messages: Array<{ ID: string; Subject: string; To?: Array<{ Address?: string }> }> }
  const message = listing.messages.find((candidate) => candidate.Subject.includes('Someone left you something') && candidate.To?.some((to) => to.Address === recipient))!
  const detail = await (await request.get(`${mailpitURL}/api/v1/message/${message.ID}`)).json() as { HTML?: string; Text?: string }
  const content = `${detail.HTML ?? ''}\n${detail.Text ?? ''}`.replaceAll('&amp;', '&')
  const match = content.match(/https?:\/\/[^\s"'<>]+\/r\/[^\s"'<>?]+/u)
  if (!match) throw new Error('Release email did not contain a recipient URL')
  return match[0]
}
