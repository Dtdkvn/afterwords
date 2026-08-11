import { FormEvent, useState } from 'react'
import { Save, X } from 'lucide-react'
import { api } from '../lib/api'
import { formatReminderOffsets, parseReminderOffsets } from '../lib/schedule'
import type { SwitchRecord } from '../types'
import { Spinner } from './Brand'

export function SwitchSettings({ value, onClose, onSaved }: { value: SwitchRecord; onClose: () => void; onSaved: (value: SwitchRecord) => void }) {
  const [name, setName] = useState(value.name)
  const [intervalDays, setIntervalDays] = useState(value.intervalDays)
  const [graceDays, setGraceDays] = useState(value.graceDays)
  const [beforeDays, setBeforeDays] = useState(formatReminderOffsets(value.reminders.beforeDays))
  const [overdueDays, setOverdueDays] = useState(formatReminderOffsets(value.reminders.overdueDays))
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  async function save(event: FormEvent) {
    event.preventDefault()
    setBusy(true)
    setError('')
    try {
      const result = await api.updateSwitch(value.id, {
        name,
        intervalDays,
        graceDays,
        reminders: { beforeDays: parseReminderOffsets(beforeDays), overdueDays: parseReminderOffsets(overdueDays) },
      })
      onSaved(result.switch)
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : 'Could not save these settings.')
    } finally {
      setBusy(false)
    }
  }

  return <div className="modal-backdrop"><section className="settings-modal paper-card" role="dialog" aria-modal="true" aria-label="Switch settings"><button className="modal-close" onClick={onClose}><X /></button><p className="card-kicker">Timing and reminders</p><h2>Edit {value.name}</h2><p>Changing timing gives an active or overdue switch a fresh full deadline. Released switches cannot be changed.</p><form className="form-stack" onSubmit={save}><label><span>Switch name</span><input value={name} maxLength={80} onChange={(event) => setName(event.target.value)} required /></label><div className="form-grid-two"><label><span>Check-in interval · days</span><input type="number" min={1} max={365} value={intervalDays} onChange={(event) => setIntervalDays(Number(event.target.value))} /></label><label><span>Grace period · days</span><input type="number" min={0} max={90} value={graceDays} onChange={(event) => setGraceDays(Number(event.target.value))} /></label></div><div className="form-grid-two"><label><span>Before deadline · days</span><input value={beforeDays} onChange={(event) => setBeforeDays(event.target.value)} placeholder="3, 1" /></label><label><span>When overdue · days</span><input value={overdueDays} onChange={(event) => setOverdueDays(event.target.value)} placeholder="0, 1, 2" /></label></div>{error && <div className="form-error">{error}</div>}<div className="wizard-actions"><button type="button" className="ghost-button" onClick={onClose}>Cancel</button><button className="primary-button" disabled={busy}>{busy ? <Spinner label="Saving" /> : <><Save />Save settings</>}</button></div></form></section></div>
}
