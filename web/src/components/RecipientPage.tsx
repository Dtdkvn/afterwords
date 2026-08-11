import { useEffect, useMemo, useState } from 'react'
import { Check, Download, FileText, Heart, KeyRound, LockKeyhole, ShieldCheck, Upload } from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import { api } from '../lib/api'
import { base64ToBytes, combineDataKey, decryptVaultItem, shareFingerprint } from '../lib/crypto'
import type { ReleaseBundle, VaultContent } from '../types'
import { Brand, Spinner, ThemeButton } from './Brand'

export function RecipientPage({ token, dark, onTheme }: { token: string; dark: boolean; onTheme: () => void }) {
  const [bundle, setBundle] = useState<ReleaseBundle | null>(null)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState('')
  const [shareInput, setShareInput] = useState('')
  const [matched, setMatched] = useState<string[]>([])
  const [contents, setContents] = useState<Array<{ id: string; value: VaultContent }>>([])
  const [opening, setOpening] = useState(false)
  const [openError, setOpenError] = useState('')

  useEffect(() => {
    void api.releaseBundle(token)
      .then(setBundle)
      .catch((failure) => setLoadError(failure instanceof Error ? failure.message : 'This release could not be opened.'))
      .finally(() => setLoading(false))
  }, [token])

  const submittedShares = useMemo(() => shareInput.split(/\r?\n/gu).map((share) => share.trim()).filter(Boolean), [shareInput])

  async function inspectShares() {
    if (!bundle) return
    setOpenError('')
    setOpening(true)
    try {
      const known = new Set(bundle.trustees.map((trustee) => trustee.shareFingerprint))
      const accepted: string[] = []
      const seen = new Set<string>()
      for (const share of submittedShares) {
        const fingerprint = await shareFingerprint(share)
        if (!known.has(fingerprint)) throw new Error('One of these shares does not belong to this release. Check the Recovery Kit and try again.')
        if (!seen.has(fingerprint)) {
          seen.add(fingerprint)
          accepted.push(share)
        }
      }
      setMatched(Array.from(seen))
      if (accepted.length < bundle.threshold) {
        throw new Error(`${bundle.threshold - accepted.length} more valid ${bundle.threshold - accepted.length === 1 ? 'share is' : 'shares are'} needed.`)
      }
      const dataKey = await combineDataKey(accepted, bundle.threshold)
      try {
        const decrypted = await Promise.all(bundle.items.map(async (item) => ({ id: item.id, value: await decryptVaultItem(dataKey, item) })))
        setContents(decrypted)
      } finally {
        dataKey.fill(0)
      }
    } catch (failure) {
      setOpenError(failure instanceof Error ? failure.message : 'The shares could not open this release.')
    } finally {
      setOpening(false)
    }
  }

  async function importShareFile(file: File | undefined) {
    if (!file) return
    if (file.size > 64 * 1024) {
      setOpenError('Recovery share text files must be smaller than 64 KB.')
      return
    }
    const text = (await file.text()).trim()
    if (!text) {
      setOpenError('That text file is empty.')
      return
    }
    // This reads the local file directly in the browser. It is never uploaded.
    setShareInput((current) => [current.trim(), text].filter(Boolean).join('\n'))
    setOpenError('')
  }

  if (loading) return <main className="recipient-loading"><Brand /><Spinner label="Preparing this private page" /></main>
  if (loadError || !bundle) {
    return <main className="recipient-loading"><Brand /><LockKeyhole size={42} /><h1>This private page is not available.</h1><p>{loadError || 'The link may be incomplete or expired.'}</p></main>
  }

  const opened = contents.length > 0 || (bundle.items.length === 0 && matched.length >= bundle.threshold && !openError)
  return (
    <main className="recipient-shell">
      <header className="recipient-header"><Brand compact /><ThemeButton dark={dark} onToggle={onTheme} /></header>
      {bundle.testMode && <div className="test-banner"><ShieldCheck size={18} /><strong>Safe rehearsal:</strong> this is a fire drill. No real switch state changed.</div>}
      {!opened ? (
        <section className="recipient-grid">
          <article className="recipient-intro">
            <span className="heart-seal"><Heart fill="currentColor" /></span>
            <p className="eyebrow">A private message is waiting</p>
            <h1>Someone who cared about you left you something.</h1>
            <p className="lede">Take your time. Nothing on this page is sent elsewhere, and Afterwords cannot read what you are about to open.</p>
            <div className="privacy-note"><ShieldCheck /><p><strong>Decryption happens here, in this browser.</strong><br />Recovery shares and readable messages never go to the server.</p></div>
          </article>
          <article className="release-steps paper-card">
            <p className="card-kicker">{bundle.switchName}</p>
            <h2>Open the message</h2>
            <ol>
              <li className="done"><span><Check /></span><div><strong>Open this trusted link</strong><small>You have already completed this step.</small></div></li>
              <li className={submittedShares.length ? 'active' : ''}><span><KeyRound /></span><div><strong>Gather {bundle.threshold} of {bundle.shareCount} Recovery Kits</strong><small>Ask the trustees named below for their private shares.</small></div></li>
              <li><span><LockKeyhole /></span><div><strong>Unlock in this browser</strong><small>The pieces reconstruct the key only on this device.</small></div></li>
            </ol>
            <div className="trustee-chips">
              {bundle.trustees.map((trustee) => <span key={trustee.shareFingerprint}>{trustee.name}<small>share {trustee.shareIndex}</small></span>)}
            </div>
            <label className="share-input">
              <span>Paste one recovery share per line</span>
              <textarea data-testid="release-shares" rows={6} value={shareInput} onChange={(event) => setShareInput(event.target.value)} spellCheck={false} placeholder={'aw1.p.…\naw1.p.…'} />
            </label>
            <label className="share-file-button"><Upload size={16} /><span>Read a share from a text Recovery Kit</span><small>Stays on this device</small><input type="file" accept=".txt,text/plain" onChange={(event) => void importShareFile(event.target.files?.[0])} /></label>
            {openError && <div className="form-error" role="alert">{openError}</div>}
            <button data-testid="open-release" className="primary-button full-button" disabled={opening || submittedShares.length === 0} onClick={() => void inspectShares()}>
              {opening ? <Spinner label="Combining privately" /> : <><KeyRound size={18} />Open what was left for me</>}
            </button>
          </article>
        </section>
      ) : (
        <section className="opened-release">
          <header><span className="heart-seal"><Heart fill="currentColor" /></span><p className="eyebrow">Opened privately</p><h1>{bundle.switchName}</h1><p>These words were decrypted only on your device.</p></header>
          <div className="message-list">
            {contents.length === 0 && <article className="message-card"><h2>There are no vault items in this rehearsal.</h2></article>}
            {contents.map(({ id, value }) => value.type === 'message' ? (
              <article className="message-card" key={id} data-testid="decrypted-message">
                <div className="message-type"><FileText size={16} />A message</div>
                <h2>{value.title}</h2>
                <div className="markdown"><ReactMarkdown>{value.body}</ReactMarkdown></div>
              </article>
            ) : <FileCard key={id} content={value} />)}
          </div>
          <footer className="recipient-footer"><Brand compact /><p>Afterwords cannot see this content. Close this tab when you are finished.</p></footer>
        </section>
      )}
    </main>
  )
}

function FileCard({ content }: { content: Extract<VaultContent, { type: 'file' }> }) {
  function download() {
    const bytes = base64ToBytes(content.dataBase64)
    const blob = new Blob([bytes], { type: content.mimeType || 'application/octet-stream' })
    const url = URL.createObjectURL(blob)
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = content.filename
    anchor.click()
    window.setTimeout(() => URL.revokeObjectURL(url), 1000)
  }
  return (
    <article className="message-card file-card">
      <div className="message-type"><Download size={16} />A file</div>
      <h2>{content.title}</h2>
      <p>{content.filename}</p>
      <button className="secondary-button" onClick={download}><Download size={17} />Download privately</button>
    </article>
  )
}
