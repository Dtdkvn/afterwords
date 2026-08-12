import { ChangeEvent, useEffect, useRef, useState } from 'react'
import { Download, File, FilePlus2, History, LockKeyhole, Mail, MessageSquarePlus, ShieldCheck, Trash2, Users, X } from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import { APIError, api } from '../lib/api'
import { latestDeliveryEntryIDs } from '../lib/audit'
import { base64ToBytes, bytesToBase64, decryptVaultItem, encryptVaultContent, unwrapDataKey } from '../lib/crypto'
import type { AuditEntry, Keyslot, Recipient, SwitchRecord, Trustee, VaultContent, VaultItem } from '../types'
import { Spinner } from './Brand'

type OpenItem = { item: VaultItem; content: VaultContent }

export function VaultModal({ switchValue, onClose }: { switchValue: SwitchRecord; onClose: () => void }) {
  const [tab, setTab] = useState<'vault' | 'people' | 'audit'>('vault')
  const [keyslot, setKeyslot] = useState<Keyslot | null>(null)
  const [encrypted, setEncrypted] = useState<VaultItem[]>([])
  const [opened, setOpened] = useState<OpenItem[]>([])
  const [recipients, setRecipients] = useState<Recipient[]>([])
  const [trustees, setTrustees] = useState<Trustee[]>([])
  const [audit, setAudit] = useState<AuditEntry[]>([])
  const [passphrase, setPassphrase] = useState('')
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [composer, setComposer] = useState<'message' | 'file' | null>(null)
  const [title, setTitle] = useState('')
  const [body, setBody] = useState('')
  const [file, setFile] = useState<File | null>(null)
  const keyRef = useRef<Uint8Array | null>(null)

  useEffect(() => {
    void Promise.all([
      api.getKeyslot(switchValue.id).then((result) => setKeyslot(result.keyslot)).catch((failure) => { if (!(failure instanceof APIError && failure.status === 404)) throw failure }),
      api.listVault(switchValue.id).then((result) => setEncrypted(result.items)),
      api.listRecipients(switchValue.id).then((result) => setRecipients(result.recipients)),
      api.listTrustees(switchValue.id).then((result) => setTrustees(result.trustees)),
      api.listAudit(switchValue.id).then((result) => setAudit(result.entries)),
    ]).catch((failure) => setError(failure instanceof Error ? failure.message : 'Could not load this switch.')).finally(() => setLoading(false))
    return () => { keyRef.current?.fill(0) }
  }, [switchValue.id])

  async function unlock() {
    if (!keyslot) return
    setBusy(true)
    setError('')
    try {
      const key = await unwrapDataKey(keyslot, passphrase)
      const contents = await Promise.all(encrypted.map(async (item) => ({ item, content: await decryptVaultItem(key, item) })))
      keyRef.current?.fill(0)
      keyRef.current = key
      setOpened(contents)
      setPassphrase('')
    } catch {
      setError('That passphrase could not unlock this vault. Check it and try again.')
    } finally {
      setBusy(false)
    }
  }

  async function saveItem() {
    if (!keyRef.current || !composer) return
    if (composer === 'message' && (!title.trim() || !body.trim())) { setError('Add a title and message.'); return }
    if (composer === 'file' && !file) { setError('Choose a file first.'); return }
    if (file && file.size > 18 * 1024 * 1024) { setError('Files are limited to 18 MB so their encrypted envelope stays within the 25 MB vault quota.'); return }
    setBusy(true)
    setError('')
    try {
      let content: VaultContent
      if (composer === 'message') content = { version: 1, type: 'message', title: title.trim(), body }
      else {
        const bytes = new Uint8Array(await file!.arrayBuffer())
        content = { version: 1, type: 'file', title: title.trim() || file!.name, filename: file!.name, mimeType: file!.type || 'application/octet-stream', dataBase64: bytesToBase64(bytes) }
        bytes.fill(0)
      }
      const reservation = await api.reserveVault(switchValue.id)
      const envelope = await encryptVaultContent(keyRef.current, content, reservation.aad)
      const saved = await api.putVault(switchValue.id, reservation.itemId, envelope)
      setOpened((current) => [...current, { item: saved.item, content }])
      setEncrypted((current) => [...current, saved.item])
      setComposer(null); setTitle(''); setBody(''); setFile(null)
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : 'Could not save this encrypted item.')
    } finally {
      setBusy(false)
    }
  }

  async function remove(itemId: string) {
    if (!window.confirm('Delete this encrypted vault item? This cannot be undone.')) return
    setBusy(true)
    try {
      await api.deleteVault(switchValue.id, itemId)
      setOpened((current) => current.filter(({ item }) => item.id !== itemId))
      setEncrypted((current) => current.filter((item) => item.id !== itemId))
    } catch (failure) { setError(failure instanceof Error ? failure.message : 'Could not delete this item.') }
    finally { setBusy(false) }
  }

  async function retryDeadLetter(messageId: string) {
    setBusy(true)
    setError('')
    try {
      await api.retryDeadLetter(switchValue.id, messageId)
      const result = await api.listAudit(switchValue.id)
      setAudit(result.entries)
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : 'Could not retry this email.')
    } finally {
      setBusy(false)
    }
  }

  const unlocked = keyRef.current !== null
  return (
    <div className="modal-backdrop">
      <section className="workspace-modal" role="dialog" aria-modal="true" aria-label={`${switchValue.name} workspace`}>
        <button className="modal-close" aria-label="Close vault" onClick={onClose}><X /></button>
        <header className="workspace-header"><div><span className={`state-badge ${switchValue.state.toLowerCase()}`}>{switchValue.state}</span><h2>{switchValue.name}</h2></div><nav><button className={tab === 'vault' ? 'active' : ''} onClick={() => setTab('vault')}><LockKeyhole />Vault</button><button className={tab === 'people' ? 'active' : ''} onClick={() => setTab('people')}><Users />People</button><button className={tab === 'audit' ? 'active' : ''} onClick={() => setTab('audit')}><History />Activity</button></nav></header>
        <div className="workspace-body">
          {loading ? <div className="workspace-loading"><Spinner label="Loading encrypted workspace" /></div> : tab === 'vault' ? (
            !keyslot ? <div className="incomplete-vault"><LockKeyhole /><h3>This switch has no vault key yet.</h3><p>It looks like setup was closed early. Delete this incomplete switch and create it again so its key and Recovery Kits are prepared together.</p></div> : !unlocked ? (
              <div className="unlock-panel"><span className="lock-orbit"><LockKeyhole /></span><p className="eyebrow">Client-side unlock</p><h3>Your encrypted vault is ready.</h3><p>Enter the separate vault passphrase. It is used here to unwrap the Data Key and is never sent to the server.</p><label><span>Vault passphrase</span><input data-testid="unlock-passphrase" type="password" autoFocus value={passphrase} onChange={(event) => setPassphrase(event.target.value)} onKeyDown={(event) => { if (event.key === 'Enter') void unlock() }} /></label>{error && <div className="form-error">{error}</div>}<button data-testid="unlock-vault" className="primary-button" disabled={busy || !passphrase} onClick={() => void unlock()}>{busy ? <Spinner label="Deriving key" /> : <><LockKeyhole />Unlock only in this browser</>}</button></div>
            ) : (
              <div className="vault-open">
                <div className="vault-toolbar"><div><ShieldCheck /><span><strong>Vault open in memory</strong><small>{opened.length} encrypted {opened.length === 1 ? 'item' : 'items'} · closes when this window closes</small></span></div><div><button className="secondary-button" onClick={() => setComposer('file')}><FilePlus2 />Add file</button><button className="primary-button" onClick={() => setComposer('message')}><MessageSquarePlus />Add message</button></div></div>
                {error && <div className="form-error">{error}</div>}
                {composer && <div className="composer"><header><div><strong>{composer === 'message' ? 'New private message' : 'Add an encrypted file'}</strong><small>Encrypted locally before upload</small></div><button className="icon-button" onClick={() => setComposer(null)}><X /></button></header><label><span>Title</span><input value={title} onChange={(event) => setTitle(event.target.value)} placeholder={composer === 'message' ? 'A few words for you' : file?.name || 'File title'} /></label>{composer === 'message' ? <label><span>Message · Markdown supported</span><textarea rows={6} value={body} onChange={(event) => setBody(event.target.value)} /></label> : <label className="file-picker"><File /><span>{file ? <><strong>{file.name}</strong><small>{formatBytes(file.size)}</small></> : <><strong>Choose a file</strong><small>Up to 18 MB before encryption</small></>}</span><input type="file" onChange={(event: ChangeEvent<HTMLInputElement>) => setFile(event.target.files?.[0] ?? null)} /></label>}<div className="composer-actions"><button className="ghost-button" onClick={() => setComposer(null)}>Cancel</button><button className="primary-button" disabled={busy} onClick={() => void saveItem()}>{busy ? <Spinner label="Encrypting" /> : <><LockKeyhole />Encrypt and save</>}</button></div></div>}
                <div className="owner-items">{opened.map(({ item, content }) => <OwnerItem key={item.id} content={content} onDelete={() => void remove(item.id)} />)}</div>
              </div>
            )
          ) : tab === 'people' ? <PeoplePanel recipients={recipients} trustees={trustees} threshold={switchValue.recoveryThreshold} total={switchValue.recoveryShareCount} /> : <AuditPanel entries={audit} busy={busy} onRetry={(messageId) => void retryDeadLetter(messageId)} />}
        </div>
      </section>
    </div>
  )
}

function OwnerItem({ content, onDelete }: { content: VaultContent; onDelete: () => void }) {
  function download() {
    if (content.type !== 'file') return
    const blob = new Blob([base64ToBytes(content.dataBase64)], { type: content.mimeType })
    const url = URL.createObjectURL(blob)
    const anchor = document.createElement('a'); anchor.href = url; anchor.download = content.filename; anchor.click()
    window.setTimeout(() => URL.revokeObjectURL(url), 1000)
  }
  return <article className="owner-item"><header><span>{content.type === 'message' ? <MessageSquarePlus /> : <File />}</span><div><small>{content.type}</small><h3>{content.title}</h3></div><button className="row-remove" onClick={onDelete} aria-label="Delete vault item"><Trash2 /></button></header>{content.type === 'message' ? <div className="markdown"><ReactMarkdown>{content.body}</ReactMarkdown></div> : <div className="file-summary"><span>{content.filename}</span><button className="secondary-button" onClick={download}><Download />Download</button></div>}</article>
}

function PeoplePanel({ recipients, trustees, threshold, total }: { recipients: Recipient[]; trustees: Trustee[]; threshold: number; total: number }) {
  return <div className="people-panel"><section><header><Mail /><div><h3>Recipients</h3><p>Receive the calm, guided release link.</p></div></header>{recipients.map((person) => <div className="saved-person" key={person.id}><span>{person.name.slice(0, 1).toUpperCase()}</span><div><strong>{person.name}</strong><small>{person.email}</small></div></div>)}</section><section><header><Users /><div><h3>Trustees · {threshold} of {total}</h3><p>Each holds one share that the server never sees.</p></div></header>{trustees.map((person) => <div className="saved-person" key={person.id}><span>{person.shareIndex}</span><div><strong>{person.name}</strong><small>{person.email} · kit {person.kitDeliveredAt ? 'opened' : 'not marked delivered'}</small></div></div>)}</section><div className="quiet-info full-span"><ShieldCheck /><p>Recovery shares cannot be displayed here. That absence is the zero-knowledge promise working as designed.</p></div></div>
}

function AuditPanel({ entries, busy, onRetry }: { entries: AuditEntry[]; busy: boolean; onRetry: (messageId: string) => void }) {
  const latestDeliveryEntries = latestDeliveryEntryIDs(entries)
  return <div className="audit-panel"><header><History /><div><h3>Private activity log</h3><p>Operational events only. No message text, passphrase, key, or share is logged.</p></div></header><ol>{entries.map((entry) => {
    const messageId = typeof entry.meta.messageId === 'string' ? entry.meta.messageId : ''
    const deadLettered = entry.event === 'email.dead_lettered'
    const retryable = deadLettered && messageId && latestDeliveryEntries.get(messageId) === entry.id
    return <li key={entry.id}><span /><div><strong>{deadLettered ? 'Email delivery stopped after 10 attempts' : humanEvent(entry.event)}</strong><small>{new Date(entry.createdAt).toLocaleString()}{retryable ? ' · Check SMTP settings, then retry.' : ''}</small>{retryable ? <button className="secondary-button" disabled={busy} onClick={() => onRetry(messageId)}><Mail size={15} />Retry delivery</button> : null}</div></li>
  })}</ol></div>
}

function humanEvent(event: string) { return event.replaceAll('.', ' ').replaceAll('_', ' ').replace(/^./u, (letter) => letter.toUpperCase()) }
function formatBytes(bytes: number) { return bytes < 1024 * 1024 ? `${Math.ceil(bytes / 1024)} KB` : `${(bytes / 1024 / 1024).toFixed(1)} MB` }
