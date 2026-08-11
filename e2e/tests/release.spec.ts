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
  await page.getByTestId('wizard-next').click()

  await page.getByTestId('vault-title').fill('For a rainy afternoon')
  await page.getByTestId('vault-message').fill('## You found it\n\nThe important thing is simple: **you were loved, every day.**')
  await page.getByTestId('vault-passphrase').fill(vaultPassphrase)
  await page.getByLabel('Repeat passphrase').fill(vaultPassphrase)
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
  await recipientPage.getByTestId('release-shares').fill(shares.join('\n'))
  await recipientPage.getByTestId('open-release').click()
  await expect(recipientPage.getByTestId('decrypted-message')).toContainText('you were loved, every day')
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
