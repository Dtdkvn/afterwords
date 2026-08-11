import { useEffect, useState } from 'react'
import { Activity, Archive, ChevronRight, CirclePause, CirclePlay, Clock3, Copy, FlaskConical, HeartPulse, KeyRound, LockKeyhole, LogOut, Mail, MoreHorizontal, Plus, Settings, ShieldCheck } from 'lucide-react'
import { api } from '../lib/api'
import type { Owner, SwitchRecord } from '../types'
import { Brand, Spinner, ThemeButton } from './Brand'
import { CreateWizard } from './CreateWizard'
import { VaultModal } from './VaultModal'
import { SwitchSettings } from './SwitchSettings'

export function Dashboard({ owner, dark, onTheme, onLogout }: { owner: Owner; dark: boolean; onTheme: () => void; onLogout: () => void }) {
  const [switches, setSwitches] = useState<SwitchRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [wizard, setWizard] = useState(false)
  const [selected, setSelected] = useState<SwitchRecord | null>(null)
  const [editing, setEditing] = useState<SwitchRecord | null>(null)
  const [busy, setBusy] = useState('')
  const [toast, setToast] = useState('')
  const [tokenReveal, setTokenReveal] = useState<{ switchName: string; token: string } | null>(null)

  async function refresh() {
    try {
      const result = await api.listSwitches()
      setSwitches(result.switches)
    } catch (failure) {
      setToast(failure instanceof Error ? failure.message : 'Could not load switches.')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { void refresh() }, [])
  useEffect(() => {
    if (!toast) return
    const timeout = window.setTimeout(() => setToast(''), 4500)
    return () => window.clearTimeout(timeout)
  }, [toast])

  async function action(id: string, task: () => Promise<unknown>, success: string) {
    setBusy(id)
    try {
      await task()
      setToast(success)
      await refresh()
    } catch (failure) {
      setToast(failure instanceof Error ? failure.message : 'That action could not be completed.')
    } finally {
      setBusy('')
    }
  }

  async function logout() {
    await api.logout()
    onLogout()
  }

  return (
    <main className="dashboard-shell">
      <header className="app-header">
        <Brand compact />
        <nav className="main-nav"><a className="active" href="#switches"><HeartPulse />Switches</a><a href="#security"><ShieldCheck />Security</a></nav>
        <div className="header-actions"><ThemeButton dark={dark} onToggle={onTheme} /><span className="owner-chip">{owner.email.slice(0, 1).toUpperCase()}</span><button className="icon-button" aria-label="Sign out" onClick={() => void logout()}><LogOut size={18} /></button></div>
      </header>
      <div className="dashboard-main" id="switches">
        <section className="dashboard-title">
          <div><p className="eyebrow">Your quiet safety net</p><h1>Good to see you.</h1><p>Each check-in is a small promise kept. Everything is private until it truly needs to be shared.</p></div>
          <button data-testid="new-switch" className="primary-button" onClick={() => setWizard(true)}><Plus />New switch</button>
        </section>
        {loading ? <div className="dashboard-loading"><Spinner label="Loading your switches" /></div> : switches.length === 0 ? (
          <section className="empty-state paper-card">
            <span className="empty-illustration"><span /><span /><HeartPulse /></span>
            <p className="eyebrow">Nothing ticking yet</p>
            <h2>Create a promise to your future people.</h2>
            <p>You choose a check-in rhythm, encrypt what matters, and give trusted people the pieces needed to open it. The server never learns the contents.</p>
            <button className="primary-button" onClick={() => setWizard(true)}><Plus />Create my first switch</button>
          </section>
        ) : (
          <section className="switch-grid">
            {switches.map((value) => (
              <SwitchCard key={value.id} value={value} busy={busy === value.id} onOpen={() => setSelected(value)}
                onEdit={() => setEditing(value)}
                onCheckin={() => void action(value.id, () => api.checkin(value.id), 'Checked in. The full interval starts again now.')}
                onPause={() => void action(value.id, () => value.state === 'PAUSED' ? api.resume(value.id) : api.pause(value.id), value.state === 'PAUSED' ? 'Switch resumed with a fresh deadline.' : 'Switch paused. The scheduler will ignore it.')}
                onMagic={() => void action(value.id, () => api.sendMagic(value.id), 'A single-use check-in link was queued for email.')}
                onFireDrill={async () => {
                  setBusy(value.id)
                  try { const result = await api.fireDrill(value.id); setToast('Fire drill queued only to your owner email.'); window.open(result.previewURL, '_blank', 'noopener,noreferrer'); await refresh() }
                  catch (failure) { setToast(failure instanceof Error ? failure.message : 'Could not start the fire drill.') }
                  finally { setBusy('') }
                }}
                onRotate={async () => {
                  if (!window.confirm('Rotate the API token? The previous token will stop working immediately.')) return
                  setBusy(value.id)
                  try { const result = await api.rotateAPIToken(value.id); setTokenReveal({ switchName: value.name, token: result.apiToken }) }
                  catch (failure) { setToast(failure instanceof Error ? failure.message : 'Could not rotate the token.') }
                  finally { setBusy('') }
                }}
                onDelete={() => {
                  if (window.confirm(`Permanently delete “${value.name}” and its encrypted vault? This cannot be undone.`)) void action(value.id, () => api.deleteSwitch(value.id), 'Switch and encrypted data deleted.')
                }} />
            ))}
          </section>
        )}
        <section className="system-strip" id="security">
          <div><ShieldCheck /><span><strong>Zero-knowledge vault</strong><small>The server stores ciphertext, never readable messages or shares.</small></span></div>
          <div><Activity /><span><strong>Scheduler running</strong><small>Deadlines and durable email delivery are monitored in UTC.</small></span></div>
          <div><ShieldCheck /><span><strong>Threat model included</strong><small>Review SECURITY.md from the source or release archive.</small></span></div>
        </section>
      </div>
      {wizard && <CreateWizard onClose={() => { setWizard(false); void refresh() }} onCompleted={() => { setWizard(false); void refresh(); setToast('Your switch is active. Keep every Recovery Kit safe.') }} />}
      {selected && <VaultModal switchValue={selected} onClose={() => { setSelected(null); void refresh() }} />}
      {editing && <SwitchSettings value={editing} onClose={() => setEditing(null)} onSaved={() => { setEditing(null); void refresh(); setToast('Switch settings and reminder schedule saved.') }} />}
      {tokenReveal && <div className="modal-backdrop"><section className="token-reveal paper-card"><button className="modal-close" onClick={() => setTokenReveal(null)}>×</button><KeyRound /><p className="card-kicker">Shown once</p><h2>New API token for {tokenReveal.switchName}</h2><p>Save this in a password manager. The old token has already been revoked.</p><code>{tokenReveal.token}</code><button className="primary-button" onClick={() => void navigator.clipboard.writeText(tokenReveal.token)}><Copy />Copy token</button></section></div>}
      {toast && <div className="toast" role="status">{toast}</div>}
    </main>
  )
}

function SwitchCard({ value, busy, onOpen, onEdit, onCheckin, onPause, onMagic, onFireDrill, onRotate, onDelete }: {
  value: SwitchRecord; busy: boolean; onOpen: () => void; onEdit: () => void; onCheckin: () => void; onPause: () => void; onMagic: () => void; onFireDrill: () => void; onRotate: () => void; onDelete: () => void
}) {
  const [menu, setMenu] = useState(false)
  const now = Date.now()
  const deadline = new Date(value.nextDeadline).getTime()
  const total = Math.max(1, deadline - new Date(value.lastCheckin).getTime())
  const remaining = deadline - now
  const progress = Math.max(0, Math.min(1, remaining / total))
  const radius = 54
  const circumference = 2 * Math.PI * radius
  const label = value.state === 'PAUSED' ? 'Paused' : value.state === 'RELEASED' ? 'Released' : remaining <= 0 ? `${Math.max(0, Math.ceil(-remaining / 86400000))}d overdue` : remaining > 86400000 ? `${Math.ceil(remaining / 86400000)} days` : `${Math.max(1, Math.ceil(remaining / 3600000))} hours`
  const badgeClass = value.state.toLowerCase()

  return (
    <article className={`switch-card paper-card state-${badgeClass}`}>
      <header><div><span className={`state-badge ${badgeClass}`}>{value.state === 'ACTIVE' ? <span className="pulse-dot" /> : null}{value.state}</span><h2>{value.name}</h2></div><div className="menu-wrap"><button className="icon-button" aria-label="Switch options" onClick={() => setMenu(!menu)}><MoreHorizontal /></button>{menu && <div className="context-menu"><button onClick={onEdit}><Settings />Timing and reminders</button><button onClick={onMagic}><Mail />Email check-in link</button><button onClick={onRotate}><KeyRound />Rotate API token</button><button onClick={onFireDrill}><FlaskConical />Run fire drill</button><button className="danger" onClick={onDelete}><Archive />Delete switch</button></div>}</div></header>
      <div className="countdown-area">
        <div className="countdown-ring">
          <svg viewBox="0 0 124 124" aria-hidden="true"><circle className="ring-track" cx="62" cy="62" r={radius} /><circle className="ring-value" cx="62" cy="62" r={radius} strokeDasharray={circumference} strokeDashoffset={circumference * (1 - progress)} /></svg>
          <div><strong>{label.split(' ')[0]}</strong><small>{label.split(' ').slice(1).join(' ') || value.state.toLowerCase()}</small></div>
        </div>
        <div className="deadline-copy"><span><Clock3 />Next deadline</span><strong>{new Date(value.nextDeadline).toLocaleString([], { dateStyle: 'medium', timeStyle: 'short' })}</strong><small>{value.graceDays} day grace period · every {value.intervalDays} days</small></div>
      </div>
      {value.state === 'RELEASED' && <div className="release-warning"><strong>This switch has fired.</strong> A late check-in cannot retract release email already queued or delivered.</div>}
      <div className="card-actions">
        <button data-testid={`checkin-${value.id}`} className="alive-button" disabled={busy || !['ACTIVE', 'OVERDUE'].includes(value.state)} onClick={onCheckin}>{busy ? <Spinner label="Working" /> : <><HeartPulse />I'm here</>}</button>
        <button className="secondary-button" disabled={busy || ['RELEASED', 'RELEASING'].includes(value.state)} onClick={onPause}>{value.state === 'PAUSED' ? <CirclePlay /> : <CirclePause />}{value.state === 'PAUSED' ? 'Resume' : 'Pause'}</button>
      </div>
      <button className="open-vault" onClick={onOpen}><LockKeyholeIcon />Open encrypted vault<ChevronRight /></button>
    </article>
  )
}

function LockKeyholeIcon() { return <LockKeyhole size={17} /> }
