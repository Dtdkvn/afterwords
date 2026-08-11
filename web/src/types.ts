export type SwitchState = 'ACTIVE' | 'OVERDUE' | 'RELEASING' | 'RELEASED' | 'PAUSED'

export interface ReminderSchedule {
  beforeDays: number[]
  overdueDays: number[]
}

export interface Owner {
  id: string
  email: string
  createdAt: string
}

export interface SwitchRecord {
  id: string
  name: string
  intervalDays: number
  graceDays: number
  state: SwitchState
  nextDeadline: string
  lastCheckin: string
  reminders: ReminderSchedule
  recoveryThreshold: number
  recoveryShareCount: number
  fireDrillAt?: string
  createdAt: string
  updatedAt: string
}

export interface VaultItem {
  id: string
  switchId: string
  iv: string
  ciphertext: string
  aad: string
  size: number
  createdAt: string
  updatedAt: string
}

export interface KDFParams {
  memorySize: number
  iterations: number
  parallelism: number
  hashLength: number
}

export interface Keyslot {
  id: string
  switchId: string
  wrappedDk: string
  iv: string
  salt: string
  kdfParams: KDFParams
  createdAt: string
  updatedAt: string
}

export interface Recipient {
  id: string
  name: string
  email: string
  createdAt: string
}

export interface Trustee {
  id: string
  name: string
  email: string
  shareIndex: number
  shareFingerprint: string
  kitDeliveredAt?: string
  createdAt: string
}

export interface AuditEntry {
  id: string
  event: string
  meta: Record<string, unknown>
  createdAt: string
}

export type VaultContent =
  | { version: 1; type: 'message'; title: string; body: string }
  | { version: 1; type: 'file'; title: string; filename: string; mimeType: string; dataBase64: string }

export interface ReleaseBundle {
  version: number
  switchId: string
  switchName: string
  threshold: number
  shareCount: number
  trustees: Array<{ name: string; shareIndex: number; shareFingerprint: string }>
  items: VaultItem[]
  testMode: boolean
}
