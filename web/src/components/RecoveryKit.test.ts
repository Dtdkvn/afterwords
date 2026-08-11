import { describe, expect, it } from 'vitest'
import { encodeFragmentJSON } from '../lib/crypto'
import { createRecoveryKitCapture, type KitPayload } from './RecoveryKit'

describe('Recovery Kit fragment handling', () => {
  it('survives StrictMode double initialization while scrubbing the fragment once', () => {
    const payload: KitPayload = {
      switchName: 'Family letters',
      trusteeName: 'Taylor',
      trusteeEmail: 'taylor@example.com',
      share: `aw1.s.${'A'.repeat(43)}`,
      threshold: 1,
      shareCount: 1,
    }
    const capture = createRecoveryKitCapture()
    let scrubCount = 0
    const first = capture(encodeFragmentJSON(payload), () => { scrubCount++ })
    // React StrictMode renders the initializer again after history already has a clean URL.
    const second = capture('', () => { scrubCount++ })
    expect(first).toEqual(payload)
    expect(second).toEqual(payload)
    expect(scrubCount).toBe(1)
  })

  it('rejects oversized and malformed fragment payloads', () => {
    const capture = createRecoveryKitCapture()
    expect(capture('x'.repeat(16_385), () => undefined)).toBeNull()
    const malformedCapture = createRecoveryKitCapture()
    expect(malformedCapture(encodeFragmentJSON({ share: 'plaintext' }), () => undefined)).toBeNull()
  })
})
