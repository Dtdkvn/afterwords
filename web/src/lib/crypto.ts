import { argon2id } from 'hash-wasm'
import { combine, split } from 'shamir-secret-sharing'
import type { KDFParams, Keyslot, VaultContent, VaultItem } from '../types'

export const defaultKDFParams: KDFParams = {
  memorySize: 65536,
  iterations: 3,
  parallelism: 1,
  hashLength: 32,
}

const encoder = new TextEncoder()
const decoder = new TextDecoder('utf-8', { fatal: true })
const ownerWrapAAD = encoder.encode('afterwords:owner-key:v1')

export function randomBytes(length: number): Uint8Array {
  return crypto.getRandomValues(new Uint8Array(length))
}

export function generateDataKey(): Uint8Array {
  return randomBytes(32)
}

export function toBase64URL(value: Uint8Array): string {
  let binary = ''
  for (let offset = 0; offset < value.length; offset += 0x8000) {
    binary += String.fromCharCode(...value.subarray(offset, offset + 0x8000))
  }
  return btoa(binary).replaceAll('+', '-').replaceAll('/', '_').replace(/=+$/u, '')
}

export function fromBase64URL(value: string): Uint8Array {
  if (!/^[A-Za-z0-9_-]*$/u.test(value)) throw new Error('Invalid base64url value')
  const padded = value.replaceAll('-', '+').replaceAll('_', '/') + '='.repeat((4 - (value.length % 4)) % 4)
  const binary = atob(padded)
  return Uint8Array.from(binary, (character) => character.charCodeAt(0))
}

export function bytesToBase64(value: Uint8Array): string {
  let binary = ''
  for (let offset = 0; offset < value.length; offset += 0x8000) {
    binary += String.fromCharCode(...value.subarray(offset, offset + 0x8000))
  }
  return btoa(binary)
}

export function base64ToBytes(value: string): Uint8Array {
  return Uint8Array.from(atob(value), (character) => character.charCodeAt(0))
}

async function deriveKEK(passphrase: string, salt: Uint8Array, params: KDFParams): Promise<Uint8Array> {
  if (passphrase.length < 1) throw new Error('A vault passphrase is required')
  const result = await argon2id({
    password: passphrase.normalize('NFKC'),
    salt,
    parallelism: params.parallelism,
    iterations: params.iterations,
    memorySize: params.memorySize,
    hashLength: params.hashLength,
    outputType: 'binary',
  })
  return result
}

export async function wrapDataKey(dataKey: Uint8Array, passphrase: string, params: KDFParams = defaultKDFParams) {
  if (dataKey.length !== 32) throw new Error('Data Key must be 32 bytes')
  const salt = randomBytes(16)
  const iv = randomBytes(12)
  const kek = await deriveKEK(passphrase, salt, params)
  const key = await crypto.subtle.importKey('raw', kek, 'AES-GCM', false, ['encrypt'])
  const wrapped = await crypto.subtle.encrypt({ name: 'AES-GCM', iv, additionalData: ownerWrapAAD }, key, dataKey)
  kek.fill(0)
  return { wrappedDk: toBase64URL(new Uint8Array(wrapped)), iv: toBase64URL(iv), salt: toBase64URL(salt), kdfParams: params }
}

export async function unwrapDataKey(keyslot: Pick<Keyslot, 'wrappedDk' | 'iv' | 'salt' | 'kdfParams'>, passphrase: string): Promise<Uint8Array> {
  const kek = await deriveKEK(passphrase, fromBase64URL(keyslot.salt), keyslot.kdfParams)
  try {
    const key = await crypto.subtle.importKey('raw', kek, 'AES-GCM', false, ['decrypt'])
    const unwrapped = await crypto.subtle.decrypt(
      { name: 'AES-GCM', iv: fromBase64URL(keyslot.iv), additionalData: ownerWrapAAD },
      key,
      fromBase64URL(keyslot.wrappedDk),
    )
    const result = new Uint8Array(unwrapped)
    if (result.length !== 32) throw new Error('Invalid Data Key length')
    return result
  } finally {
    kek.fill(0)
  }
}

export async function encryptVaultContent(dataKey: Uint8Array, content: VaultContent, aad: string) {
  const iv = randomBytes(12)
  const key = await crypto.subtle.importKey('raw', dataKey, 'AES-GCM', false, ['encrypt'])
  const plaintext = encoder.encode(JSON.stringify(content))
  const ciphertext = new Uint8Array(await crypto.subtle.encrypt({ name: 'AES-GCM', iv, additionalData: encoder.encode(aad) }, key, plaintext))
  plaintext.fill(0)
  return { iv: toBase64URL(iv), ciphertext: toBase64URL(ciphertext), aad, size: ciphertext.length }
}

export async function decryptVaultItem(dataKey: Uint8Array, item: Pick<VaultItem, 'iv' | 'ciphertext' | 'aad'>): Promise<VaultContent> {
  const key = await crypto.subtle.importKey('raw', dataKey, 'AES-GCM', false, ['decrypt'])
  const plaintext = await crypto.subtle.decrypt(
    { name: 'AES-GCM', iv: fromBase64URL(item.iv), additionalData: encoder.encode(item.aad) },
    key,
    fromBase64URL(item.ciphertext),
  )
  const parsed = JSON.parse(decoder.decode(plaintext)) as VaultContent
  if (parsed.version !== 1 || !['message', 'file'].includes(parsed.type) || typeof parsed.title !== 'string') {
    throw new Error('Unsupported vault item')
  }
  return parsed
}

export async function splitDataKey(dataKey: Uint8Array, shareCount: number, threshold: number): Promise<string[]> {
  if (dataKey.length !== 32) throw new Error('Data Key must be 32 bytes')
  if (shareCount === 1 && threshold === 1) return [`aw1.s.${toBase64URL(dataKey)}`]
  if (shareCount < 2 || shareCount > 255 || threshold < 2 || threshold > shareCount) throw new Error('Invalid recovery policy')
  const shares = await split(dataKey, shareCount, threshold)
  return shares.map((share) => `aw1.p.${toBase64URL(share)}`)
}

export function decodeRecoveryShare(value: string): { mode: 'single' | 'shamir'; bytes: Uint8Array } {
  const normalized = value.trim().replaceAll(/\s+/gu, '')
  if (normalized.startsWith('aw1.s.')) {
    const bytes = fromBase64URL(normalized.slice(6))
    if (bytes.length !== 32) throw new Error('Invalid single recovery share')
    return { mode: 'single', bytes }
  }
  if (normalized.startsWith('aw1.p.')) {
    const bytes = fromBase64URL(normalized.slice(6))
    if (bytes.length < 33) throw new Error('Invalid Shamir recovery share')
    return { mode: 'shamir', bytes }
  }
  throw new Error('This does not look like an Afterwords recovery share')
}

export async function combineDataKey(values: string[], threshold: number): Promise<Uint8Array> {
  const decoded = values.map(decodeRecoveryShare)
  if (threshold === 1) {
    if (decoded.length < 1 || decoded[0].mode !== 'single') throw new Error('The 1-of-1 recovery share is required')
    return decoded[0].bytes.slice()
  }
  if (decoded.length < threshold) throw new Error(`At least ${threshold} recovery shares are required`)
  if (decoded.some((share) => share.mode !== 'shamir')) throw new Error('Recovery share types do not match')
  const combined = await combine(decoded.slice(0, threshold).map((share) => share.bytes))
  if (combined.length !== 32) throw new Error('Recovered Data Key has an invalid length')
  return combined
}

export async function shareFingerprint(value: string): Promise<string> {
  const { bytes } = decodeRecoveryShare(value)
  const digest = new Uint8Array(await crypto.subtle.digest('SHA-256', bytes))
  return Array.from(digest, (byte) => byte.toString(16).padStart(2, '0')).join('')
}

export function encodeFragmentJSON(value: unknown): string {
  return toBase64URL(encoder.encode(JSON.stringify(value)))
}

export function decodeFragmentJSON<T>(value: string): T {
  return JSON.parse(decoder.decode(fromBase64URL(value))) as T
}
