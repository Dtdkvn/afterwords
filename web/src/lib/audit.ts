import type { AuditEntry } from '../types'

const deliveryStateEvents = new Set(['email.dead_lettered', 'email.requeued', 'email.sent'])

// Audit entries are newest first. Keep the exact latest entry so historical
// terminal events cannot accidentally render as current actions.
export function latestDeliveryEntryIDs(entries: AuditEntry[]): Map<string, string> {
  const latest = new Map<string, string>()
  for (const entry of entries) {
    const messageId = typeof entry.meta.messageId === 'string' ? entry.meta.messageId : ''
    if (messageId && !latest.has(messageId) && deliveryStateEvents.has(entry.event)) latest.set(messageId, entry.id)
  }
  return latest
}
