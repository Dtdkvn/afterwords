import { describe, expect, it } from 'vitest'
import {
  checkCryptoSupport,
  combineDataKey,
  decryptVaultItem,
  encryptVaultContent,
  generateDataKey,
  shareFingerprint,
  splitDataKey,
  unwrapDataKey,
  wrapDataKey,
} from './crypto'

const testKDF = { memorySize: 8192, iterations: 1, parallelism: 1, hashLength: 32 }

describe('Afterwords client cryptography', () => {
  it('round-trips an AES-GCM vault message and authenticates AAD', async () => {
    const dataKey = generateDataKey()
    const envelope = await encryptVaultContent(dataKey, { version: 1, type: 'message', title: 'Hello', body: '**Still here.**' }, 'switch:item')
    await expect(decryptVaultItem(dataKey, envelope)).resolves.toEqual({ version: 1, type: 'message', title: 'Hello', body: '**Still here.**' })
    await expect(decryptVaultItem(dataKey, { ...envelope, aad: 'switch:other' })).rejects.toThrow()
  })

  it('wraps a Data Key with Argon2id and rejects a wrong passphrase', async () => {
    const dataKey = generateDataKey()
    const wrapped = await wrapDataKey(dataKey, 'a memorable vault passphrase', testKDF)
    await expect(unwrapDataKey(wrapped, 'a memorable vault passphrase')).resolves.toEqual(dataKey)
    await expect(unwrapDataKey(wrapped, 'the wrong passphrase')).rejects.toThrow()
  })

  it('reconstructs any 2 of 3 audited Shamir shares and rejects k-1', async () => {
    const dataKey = generateDataKey()
    const shares = await splitDataKey(dataKey, 3, 2)
    await expect(combineDataKey([shares[0], shares[2]], 2)).resolves.toEqual(dataKey)
    await expect(combineDataKey([shares[1]], 2)).rejects.toThrow(/At least 2/u)
    expect(await shareFingerprint(shares[0])).toMatch(/^[a-f0-9]{64}$/u)
  })

  it('supports the explicit 1-of-1 convenience path', async () => {
    const dataKey = generateDataKey()
    const [share] = await splitDataKey(dataKey, 1, 1)
    expect(share).toMatch(/^aw1\.s\./u)
    await expect(combineDataKey([share], 1)).resolves.toEqual(dataKey)
  })

  describe('secure-context detection', () => {
    it('accepts a secure context that exposes Web Crypto', () => {
      expect(checkCryptoSupport()).toEqual({ supported: true })
    })

    it('refuses an insecure origin instead of failing silently', () => {
      const verdict = checkCryptoSupport({ isSecureContext: false, crypto } as Partial<Window>)
      expect(verdict.supported).toBe(false)
      if (verdict.supported) throw new Error('expected an unsupported verdict')
      expect(verdict.reason).toMatch(/secure context/iu)
      expect(verdict.reason).toMatch(/localhost|127\.0\.0\.1/u)
    })

    it('refuses a browser without crypto.subtle', () => {
      const verdict = checkCryptoSupport({
        isSecureContext: true,
        crypto: { getRandomValues: crypto.getRandomValues.bind(crypto) },
      } as unknown as Partial<Window>)
      expect(verdict.supported).toBe(false)
      if (verdict.supported) throw new Error('expected an unsupported verdict')
      expect(verdict.reason).toMatch(/Web Crypto API/u)
    })
  })
})
