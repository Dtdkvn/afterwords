import { describe, expect, it } from 'vitest'
import type { AuditEntry } from '../types'
import { latestDeliveryEntryIDs } from './audit'

function auditEntry(id: string, event: string, messageId: string): AuditEntry {
  return { id, event, meta: { messageId }, createdAt: '2026-08-12T09:00:00Z' }
}

describe('delivery audit actions', () => {
  it('selects only the newest terminal entry after multiple retry cycles', () => {
    const entries = [
      auditEntry('dead-letter-2', 'email.dead_lettered', 'message-1'),
      auditEntry('requeued-1', 'email.requeued', 'message-1'),
      auditEntry('dead-letter-1', 'email.dead_lettered', 'message-1'),
      auditEntry('sent-1', 'email.sent', 'message-2'),
      auditEntry('dead-letter-other', 'email.dead_lettered', 'message-2'),
    ]

    const latest = latestDeliveryEntryIDs(entries)
    const retryable = entries
      .filter((entry) => entry.event === 'email.dead_lettered' && latest.get(entry.meta.messageId as string) === entry.id)
      .map((entry) => entry.id)

    expect(retryable).toEqual(['dead-letter-2'])
  })
})
