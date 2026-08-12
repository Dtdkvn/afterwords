import type { AuditEntry, KDFParams, Keyslot, Owner, Recipient, ReleaseBundle, SwitchRecord, Trustee, VaultItem } from '../types'

let csrfToken = ''

export class APIError extends Error {
  status: number
  code: string

  constructor(status: number, code: string, message: string) {
    super(message)
    this.name = 'APIError'
    this.status = status
    this.code = code
  }
}

interface RequestOptions extends RequestInit {
  json?: unknown
}

async function request<T>(url: string, options: RequestOptions = {}): Promise<T> {
  const headers = new Headers(options.headers)
  if (options.json !== undefined) {
    headers.set('Content-Type', 'application/json')
  }
  const method = (options.method ?? 'GET').toUpperCase()
  if (!['GET', 'HEAD', 'OPTIONS'].includes(method) && csrfToken) {
    headers.set('X-CSRF-Token', csrfToken)
  }
  const response = await fetch(url, {
    ...options,
    headers,
    credentials: 'same-origin',
    body: options.json === undefined ? options.body : JSON.stringify(options.json),
  })
  if (!response.ok) {
    let code = 'request_failed'
    let message = `Request failed (${response.status})`
    try {
      const payload = await response.json()
      code = payload.error?.code ?? code
      message = payload.error?.message ?? message
    } catch {
      // Preserve the generic error for non-JSON proxy failures.
    }
    throw new APIError(response.status, code, message)
  }
  if (response.status === 204) return undefined as T
  return response.json() as Promise<T>
}

export const api = {
  async bootstrap() {
    const data = await request<{ setupRequired: boolean; setupTokenRequired: boolean; authenticated: boolean; user?: Owner; csrfToken?: string }>('/api/bootstrap')
    csrfToken = data.csrfToken ?? ''
    return data
  },
  async setup(email: string, password: string, setupToken = '') {
    const data = await request<{ user: Owner; csrfToken: string }>('/api/setup', { method: 'POST', json: { email, password, setupToken } })
    csrfToken = data.csrfToken
    return data
  },
  async login(email: string, password: string) {
    const data = await request<{ user: Owner; csrfToken: string }>('/api/login', { method: 'POST', json: { email, password } })
    csrfToken = data.csrfToken
    return data
  },
  async logout() {
    await request<void>('/api/logout', { method: 'POST', json: {} })
    csrfToken = ''
  },
  listSwitches: () => request<{ switches: SwitchRecord[] }>('/api/switches'),
  createSwitch: (input: { name: string; intervalDays: number; graceDays: number; reminders: { beforeDays: number[]; overdueDays: number[] } }) =>
    request<{ switch: SwitchRecord; apiToken: string }>('/api/switches', { method: 'POST', json: input }),
  updateSwitch: (id: string, input: { name: string; intervalDays: number; graceDays: number; reminders: { beforeDays: number[]; overdueDays: number[] } }) =>
    request<{ switch: SwitchRecord }>(`/api/switches/${id}`, { method: 'PATCH', json: input }),
  deleteSwitch: (id: string) => request<void>(`/api/switches/${id}`, { method: 'DELETE' }),
  checkin: (id: string) => request<{ switch: SwitchRecord }>(`/api/switches/${id}/checkin`, { method: 'POST', json: {} }),
  pause: (id: string) => request<{ switch: SwitchRecord }>(`/api/switches/${id}/pause`, { method: 'POST', json: {} }),
  resume: (id: string) => request<{ switch: SwitchRecord }>(`/api/switches/${id}/resume`, { method: 'POST', json: {} }),
  sendMagic: (id: string) => request<{ queued: boolean }>(`/api/switches/${id}/magic`, { method: 'POST', json: {} }),
  rotateAPIToken: (id: string) => request<{ apiToken: string }>(`/api/switches/${id}/rotate-api-token`, { method: 'POST', json: {} }),
  fireDrill: (id: string) => request<{ queued: boolean; previewURL: string }>(`/api/switches/${id}/firedrill`, { method: 'POST', json: {} }),
  reserveVault: (id: string) => request<{ itemId: string; aad: string }>(`/api/switches/${id}/vault/reserve`, { method: 'POST', json: {} }),
  putVault: (switchId: string, itemId: string, envelope: { iv: string; ciphertext: string; aad: string; size: number }) =>
    request<{ item: VaultItem }>(`/api/switches/${switchId}/vault/${itemId}`, { method: 'PUT', json: envelope }),
  listVault: (id: string) => request<{ items: VaultItem[] }>(`/api/switches/${id}/vault`),
  deleteVault: (switchId: string, itemId: string) => request<void>(`/api/switches/${switchId}/vault/${itemId}`, { method: 'DELETE' }),
  putKeyslot: (id: string, keyslot: { wrappedDk: string; iv: string; salt: string; kdfParams: KDFParams }) =>
    request<{ keyslot: Keyslot }>(`/api/switches/${id}/keyslot`, { method: 'PUT', json: keyslot }),
  getKeyslot: (id: string) => request<{ keyslot: Keyslot }>(`/api/switches/${id}/keyslot`),
  listRecipients: (id: string) => request<{ recipients: Recipient[] }>(`/api/switches/${id}/recipients`),
  addRecipient: (id: string, person: { name: string; email: string }) =>
    request<{ recipient: Recipient }>(`/api/switches/${id}/recipients`, { method: 'POST', json: person }),
  deleteRecipient: (id: string, personId: string) => request<void>(`/api/switches/${id}/recipients/${personId}`, { method: 'DELETE' }),
  listTrustees: (id: string) => request<{ trustees: Trustee[] }>(`/api/switches/${id}/trustees`),
  addTrustee: (id: string, person: { name: string; email: string; shareIndex: number; shareFingerprint: string; threshold: number; shareCount: number }) =>
    request<{ trustee: Trustee }>(`/api/switches/${id}/trustees`, { method: 'POST', json: person }),
  markKitDelivered: (id: string, personId: string) => request<void>(`/api/switches/${id}/trustees/${personId}/delivered`, { method: 'POST', json: {} }),
  deleteTrustee: (id: string, personId: string) => request<void>(`/api/switches/${id}/trustees/${personId}`, { method: 'DELETE' }),
  listAudit: (id: string) => request<{ entries: AuditEntry[] }>(`/api/switches/${id}/audit`),
  retryDeadLetter: (switchId: string, messageId: string) => request<{ queued: boolean }>(`/api/switches/${switchId}/outbox/${messageId}/retry`, { method: 'POST', json: {} }),
  releaseBundle: (token: string) => request<ReleaseBundle>(`/api/release/${encodeURIComponent(token)}/bundle`),
}
