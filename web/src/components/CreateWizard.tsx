import { FormEvent, useEffect, useMemo, useRef, useState } from 'react'
import { ArrowLeft, ArrowRight, Check, Copy, KeyRound, LockKeyhole, Mail, Plus, ShieldCheck, Trash2, Users, X } from 'lucide-react'
import { api } from '../lib/api'
import { encodeFragmentJSON, encryptVaultContent, generateDataKey, shareFingerprint, splitDataKey, wrapDataKey } from '../lib/crypto'
import { parseReminderOffsets } from '../lib/schedule'
import type { SwitchRecord } from '../types'
import type { KitPayload } from './RecoveryKit'
import { Spinner } from './Brand'

type PersonDraft = { name: string; email: string }
type Kit = KitPayload & { trusteeId: string; url: string }
type PreparedPeople = {
  recipients: PersonDraft[]
  trustees: Array<PersonDraft & { share: string; fingerprint: string; shareIndex: number }>
  threshold: number
  shareCount: number
}

export function CreateWizard({ onClose, onCompleted }: { onClose: () => void; onCompleted: (value: SwitchRecord) => void }) {
  const [step, setStep] = useState(1)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [name, setName] = useState('My letters')
  const [intervalDays, setIntervalDays] = useState(7)
  const [graceDays, setGraceDays] = useState(3)
  const [beforeDays, setBeforeDays] = useState('3, 1')
  const [overdueDays, setOverdueDays] = useState('0, 1, 2')
  const [switchValue, setSwitchValue] = useState<SwitchRecord | null>(null)
  const [apiToken, setAPIToken] = useState('')
  const [title, setTitle] = useState('For the people I love')
  const [message, setMessage] = useState('If you are reading this, there are a few things I wanted you to know…')
  const [passphrase, setPassphrase] = useState('')
  const [confirmPassphrase, setConfirmPassphrase] = useState('')
  const [recipients, setRecipients] = useState<PersonDraft[]>([{ name: '', email: '' }])
  const [trustees, setTrustees] = useState<PersonDraft[]>([{ name: '', email: '' }, { name: '', email: '' }])
  const [threshold, setThreshold] = useState(2)
  const [kits, setKits] = useState<Kit[]>([])
  const [copied, setCopied] = useState(false)
  const dataKey = useRef<Uint8Array | null>(null)
  const preparedPeople = useRef<PreparedPeople | null>(null)

  useEffect(() => () => { dataKey.current?.fill(0) }, [])

  const thresholdChoices = useMemo(() => trustees.length === 1 ? [1] : Array.from({ length: trustees.length - 1 }, (_, index) => index + 2), [trustees.length])

  async function createBasics(event: FormEvent) {
    event.preventDefault()
    setBusy(true)
    setError('')
    try {
      const reminders = { beforeDays: parseReminderOffsets(beforeDays), overdueDays: parseReminderOffsets(overdueDays) }
      const result = await api.createSwitch({ name, intervalDays, graceDays, reminders })
      setSwitchValue(result.switch)
      setAPIToken(result.apiToken)
      setStep(2)
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : 'Could not create the switch.')
    } finally {
      setBusy(false)
    }
  }

  async function saveVault(event: FormEvent) {
    event.preventDefault()
    if (!switchValue) return
    if (passphrase.length < 12) {
      setError('Use at least 12 characters for the vault passphrase.')
      return
    }
    if (passphrase !== confirmPassphrase) {
      setError('The vault passphrases do not match.')
      return
    }
    setBusy(true)
    setError('')
    try {
      const key = generateDataKey()
      dataKey.current = key
      const wrapped = await wrapDataKey(key, passphrase)
      await api.putKeyslot(switchValue.id, wrapped)
      const reservation = await api.reserveVault(switchValue.id)
      const envelope = await encryptVaultContent(key, { version: 1, type: 'message', title, body: message }, reservation.aad)
      await api.putVault(switchValue.id, reservation.itemId, envelope)
      setPassphrase('')
      setConfirmPassphrase('')
      setStep(3)
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : 'Could not encrypt the vault.')
    } finally {
      setBusy(false)
    }
  }

  async function savePeople(event: FormEvent) {
    event.preventDefault()
    if (!switchValue || !dataKey.current) return
    const validRecipients = recipients.filter((person) => person.name.trim() && person.email.trim())
    const validTrustees = trustees.filter((person) => person.name.trim() && person.email.trim())
    if (validRecipients.length < 1) {
      setError('Add at least one recipient.')
      return
    }
    if (validTrustees.length !== trustees.length) {
      setError('Complete every trustee name and email, or remove the empty row.')
      return
    }
    const selectedThreshold = validTrustees.length === 1 ? 1 : Math.min(Math.max(threshold, 2), validTrustees.length)
    setBusy(true)
    setError('')
    try {
      if (!preparedPeople.current) {
        const shares = await splitDataKey(dataKey.current, validTrustees.length, selectedThreshold)
        preparedPeople.current = {
          recipients: validRecipients.map((person) => ({ name: person.name.trim(), email: person.email.trim() })),
          trustees: await Promise.all(validTrustees.map(async (trustee, index) => ({
            name: trustee.name.trim(),
            email: trustee.email.trim(),
            share: shares[index],
            fingerprint: await shareFingerprint(shares[index]),
            shareIndex: index + 1,
          }))),
          threshold: selectedThreshold,
          shareCount: validTrustees.length,
        }
      }
      const prepared = preparedPeople.current
      for (const recipient of prepared.recipients) await api.addRecipient(switchValue.id, recipient)
      const createdKits: Kit[] = []
      for (const trustee of prepared.trustees) {
        const created = await api.addTrustee(switchValue.id, {
          name: trustee.name,
          email: trustee.email,
          shareIndex: trustee.shareIndex,
          shareFingerprint: trustee.fingerprint,
          threshold: prepared.threshold,
          shareCount: prepared.shareCount,
        })
        const payload: KitPayload = {
          switchName: switchValue.name,
          trusteeName: trustee.name,
          trusteeEmail: trustee.email,
          share: trustee.share,
          threshold: prepared.threshold,
          shareCount: prepared.shareCount,
        }
        createdKits.push({ ...payload, trusteeId: created.trustee.id, url: `/kit#${encodeFragmentJSON(payload)}` })
      }
      dataKey.current.fill(0)
      dataKey.current = null
      preparedPeople.current = null
      setKits(createdKits)
      setStep(4)
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : 'Could not prepare Recovery Kits.')
    } finally {
      setBusy(false)
    }
  }

  function updatePerson(kind: 'recipient' | 'trustee', index: number, field: keyof PersonDraft, value: string) {
    const setter = kind === 'recipient' ? setRecipients : setTrustees
    setter((current) => current.map((person, candidate) => candidate === index ? { ...person, [field]: value } : person))
  }

  function removePerson(kind: 'recipient' | 'trustee', index: number) {
    if (kind === 'recipient') setRecipients((current) => current.filter((_, candidate) => candidate !== index))
    else {
      setTrustees((current) => {
        const next = current.filter((_, candidate) => candidate !== index)
        setThreshold(next.length === 1 ? 1 : Math.min(Math.max(2, threshold), next.length))
        return next
      })
    }
  }

  return (
    <div className="modal-backdrop" role="presentation">
      <section className="wizard-modal" role="dialog" aria-modal="true" aria-labelledby="wizard-title">
        <button className="modal-close" aria-label="Close setup" onClick={onClose}><X /></button>
        <header className="wizard-header">
          <div><span className="card-kicker">New private switch</span><h2 id="wizard-title">{['Choose your rhythm', 'Write the first message', 'Choose your people', 'Keep these somewhere safe'][step - 1]}</h2></div>
          <ol className="stepper" aria-label="Setup progress">{[1, 2, 3, 4].map((number) => <li key={number} className={number === step ? 'active' : number < step ? 'done' : ''}>{number < step ? <Check /> : number}</li>)}</ol>
        </header>
        <div className="wizard-body">
          {step === 1 && (
            <form className="form-stack wizard-form" onSubmit={createBasics}>
              <p className="wizard-copy">Give this switch a name you will recognize. Checking in resets its full interval; the grace period begins only after a missed deadline.</p>
              <label><span>Switch name</span><input data-testid="switch-name" value={name} maxLength={80} onChange={(event) => setName(event.target.value)} required /></label>
              <div className="form-grid-two">
                <label><span>Check in every</span><span className="input-suffix"><input data-testid="interval-days" type="number" min={1} max={365} value={intervalDays} onChange={(event) => setIntervalDays(Number(event.target.value))} /><em>days</em></span></label>
                <label><span>Grace period</span><span className="input-suffix"><input data-testid="grace-days" type="number" min={0} max={90} value={graceDays} onChange={(event) => setGraceDays(Number(event.target.value))} /><em>days</em></span></label>
              </div>
              <div className="form-grid-two">
                <label><span>Remind before · days</span><input data-testid="before-reminders" value={beforeDays} onChange={(event) => setBeforeDays(event.target.value)} placeholder="3, 1" /></label>
                <label><span>Remind when overdue · days</span><input data-testid="overdue-reminders" value={overdueDays} onChange={(event) => setOverdueDays(event.target.value)} placeholder="0, 1, 2" /></label>
              </div>
              <div className="quiet-info"><ShieldCheck /><p>Use comma-separated day offsets. Day 0 means as soon as the deadline becomes overdue. Every reminder uses a single-use check-in link.</p></div>
              {error && <div className="form-error">{error}</div>}
              <div className="wizard-actions"><button className="ghost-button" type="button" onClick={onClose}>Not now</button><button data-testid="wizard-next" className="primary-button" disabled={busy}>{busy ? <Spinner label="Creating" /> : <>Continue<ArrowRight size={18} /></>}</button></div>
            </form>
          )}
          {step === 2 && (
            <form className="form-stack wizard-form" onSubmit={saveVault}>
              <p className="wizard-copy">This message is encrypted before it leaves the page. Markdown is supported. You can add files and more messages later.</p>
              <label><span>Message title</span><input data-testid="vault-title" value={title} maxLength={200} onChange={(event) => setTitle(event.target.value)} required /></label>
              <label><span>Your message</span><textarea data-testid="vault-message" rows={7} value={message} onChange={(event) => setMessage(event.target.value)} required /></label>
              <div className="passphrase-panel">
                <div><LockKeyhole /><p><strong>Create a separate vault passphrase.</strong><small>It protects the encryption key. Afterwords cannot reset it.</small></p></div>
                <div className="form-grid-two">
                  <label><span>Vault passphrase</span><input data-testid="vault-passphrase" type="password" minLength={12} autoComplete="new-password" value={passphrase} onChange={(event) => setPassphrase(event.target.value)} required /></label>
                  <label><span>Repeat passphrase</span><input type="password" minLength={12} autoComplete="new-password" value={confirmPassphrase} onChange={(event) => setConfirmPassphrase(event.target.value)} required /></label>
                </div>
              </div>
              {error && <div className="form-error">{error}</div>}
              <div className="wizard-actions"><span /><button data-testid="encrypt-vault" className="primary-button" disabled={busy}>{busy ? <Spinner label="Encrypting in your browser" /> : <><LockKeyhole size={18} />Encrypt and continue</>}</button></div>
            </form>
          )}
          {step === 3 && (
            <form className="people-layout" onSubmit={savePeople}>
              <section>
                <div className="section-heading"><div><Mail /><span><strong>Recipients</strong><small>People who receive the release link</small></span></div><button type="button" className="text-button" onClick={() => setRecipients([...recipients, { name: '', email: '' }])}><Plus />Add</button></div>
                <div className="person-list">{recipients.map((person, index) => <PersonRow key={index} person={person} onChange={(field, value) => updatePerson('recipient', index, field, value)} onRemove={() => removePerson('recipient', index)} removable={recipients.length > 1} />)}</div>
              </section>
              <section>
                <div className="section-heading"><div><Users /><span><strong>Trustees</strong><small>Each safely holds one recovery share</small></span></div><button type="button" className="text-button" onClick={() => { setTrustees([...trustees, { name: '', email: '' }]); if (trustees.length === 1) setThreshold(2) }}><Plus />Add</button></div>
                <div className="person-list">{trustees.map((person, index) => <PersonRow key={index} person={person} onChange={(field, value) => updatePerson('trustee', index, field, value)} onRemove={() => removePerson('trustee', index)} removable={trustees.length > 1} />)}</div>
                <label className="threshold-select"><span>Shares needed to open</span><select value={trustees.length === 1 ? 1 : threshold} onChange={(event) => setThreshold(Number(event.target.value))}>{thresholdChoices.map((choice) => <option value={choice} key={choice}>{choice} of {trustees.length}</option>)}</select></label>
              </section>
              <div className="quiet-info full-span"><KeyRound /><p>Shares are generated here and never uploaded. The next screen is your only chance to save every Recovery Kit.</p></div>
              {error && <div className="form-error full-span">{error}</div>}
              <div className="wizard-actions full-span"><span /><button data-testid="prepare-kits" className="primary-button" disabled={busy}>{busy ? <Spinner label="Splitting the key privately" /> : <>Prepare Recovery Kits<ArrowRight size={18} /></>}</button></div>
            </form>
          )}
          {step === 4 && switchValue && (
            <div className="finish-step">
              <div className="success-seal"><Check /></div>
              <p className="wizard-copy">Your switch is active. Open, print, download, and hand off every kit securely before leaving this page.</p>
              <div className="kit-list">{kits.map((kit, index) => <a key={kit.trusteeId} className="kit-row" href={kit.url} target="_blank" rel="noreferrer" onClick={() => void api.markKitDelivered(switchValue.id, kit.trusteeId)}><span>{index + 1}</span><div><strong>{kit.trusteeName}</strong><small>{kit.trusteeEmail}</small></div><ArrowRight /></a>)}</div>
              <div className="token-panel"><div><KeyRound /><p><strong>API check-in token</strong><small>Shown once. Save it in your password manager for cron, shortcuts, or scripts.</small></p></div><code>{apiToken}</code><button className="text-button" onClick={() => { void navigator.clipboard.writeText(apiToken); setCopied(true) }}><Copy />{copied ? 'Copied' : 'Copy'}</button></div>
              <div className="form-error warning"><strong>Do not skip the kits.</strong> The server keeps only fingerprints and cannot recreate a lost recovery share.</div>
              <div className="wizard-actions"><span /><button data-testid="finish-wizard" className="primary-button" onClick={() => onCompleted(switchValue)}>Open my dashboard<ArrowRight /></button></div>
            </div>
          )}
        </div>
      </section>
    </div>
  )
}

function PersonRow({ person, onChange, onRemove, removable }: { person: PersonDraft; onChange: (field: keyof PersonDraft, value: string) => void; onRemove: () => void; removable: boolean }) {
  return (
    <div className="person-row">
      <input aria-label="Name" placeholder="Name" value={person.name} onChange={(event) => onChange('name', event.target.value)} required />
      <input aria-label="Email" type="email" placeholder="name@example.com" value={person.email} onChange={(event) => onChange('email', event.target.value)} required />
      <button type="button" className="row-remove" aria-label="Remove person" disabled={!removable} onClick={onRemove}><Trash2 /></button>
    </div>
  )
}
