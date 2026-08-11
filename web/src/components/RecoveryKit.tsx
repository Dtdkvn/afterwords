import { useEffect, useState } from 'react'
import { Copy, Download, Printer, ShieldCheck } from 'lucide-react'
import QRCode from 'qrcode'
import { decodeFragmentJSON } from '../lib/crypto'
import { Brand } from './Brand'

export interface KitPayload {
  switchName: string
  trusteeName: string
  trusteeEmail: string
  share: string
  threshold: number
  shareCount: number
}

export function createRecoveryKitCapture() {
  let captured: KitPayload | null | undefined
  return (fragment: string, scrub: () => void): KitPayload | null => {
    if (captured !== undefined) return captured
    scrub()
    try {
      if (!fragment || fragment.length > 16_384) {
        captured = null
        return captured
      }
      const decoded = decodeFragmentJSON<KitPayload>(fragment)
      if (typeof decoded.switchName !== 'string' || decoded.switchName.length > 80 ||
        typeof decoded.trusteeName !== 'string' || decoded.trusteeName.length > 100 ||
        typeof decoded.trusteeEmail !== 'string' || decoded.trusteeEmail.length > 254 ||
        typeof decoded.share !== 'string' || decoded.share.length > 1024 || !/^aw1\.[sp]\.[A-Za-z0-9_-]+$/u.test(decoded.share) ||
        !Number.isInteger(decoded.threshold) || !Number.isInteger(decoded.shareCount) || decoded.threshold < 1 ||
        decoded.shareCount < 1 || decoded.shareCount > 255 || decoded.threshold > decoded.shareCount) {
        captured = null
        return captured
      }
      captured = decoded
      return captured
    } catch {
      captured = null
      return captured
    }
  }
}

const captureRecoveryKit = createRecoveryKitCapture()

export function RecoveryKit() {
  // The closure survives React StrictMode's development-only remount while the hash is scrubbed once.
  const payload = captureRecoveryKit(window.location.hash.slice(1), () => {
    window.history.replaceState(null, '', '/kit')
  })
  const [qr, setQR] = useState('')
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    if (!payload) return
    void QRCode.toDataURL(payload.share, { errorCorrectionLevel: 'M', margin: 2, width: 360, color: { dark: '#20302b', light: '#ffffff' } }).then(setQR)
  }, [payload])

  if (!payload) {
    return <main className="kit-missing"><Brand /><h1>This Recovery Kit is unavailable.</h1><p>Return to the setup window and open it again. Afterwords does not keep a copy of recovery shares.</p></main>
  }

  const filename = `afterwords-${payload.switchName.toLowerCase().replaceAll(/[^a-z0-9]+/gu, '-')}-${payload.trusteeName.toLowerCase().replaceAll(/[^a-z0-9]+/gu, '-')}.txt`
  const downloadURL = `data:text/plain;charset=utf-8,${encodeURIComponent(`AFTERWORDS RECOVERY KIT\n\nFor: ${payload.trusteeName}\nSwitch: ${payload.switchName}\nPolicy: ${payload.threshold} of ${payload.shareCount}\n\nRecovery share:\n${payload.share}\n\nKeep this private. Visit the release link from Afterwords only when instructed.`)}`
  return (
    <main className="kit-page">
      <article className="recovery-kit">
        <header className="kit-header">
          <Brand />
          <span className="kit-label">Recovery Kit · {payload.threshold} of {payload.shareCount}</span>
        </header>
        <section className="kit-intro">
          <p className="eyebrow">Prepared privately for {payload.trusteeName}</p>
          <h1>If you’re reading this,<br />someone trusts you.</h1>
          <p>You hold one piece of the key to <strong>{payload.switchName}</strong>. Keep it private. By itself, it cannot open anything.</p>
        </section>
        <section className="kit-steps">
          <div><span>1</span><p><strong>Keep this kit safe.</strong> A locked drawer or password manager is a good home.</p></div>
          <div><span>2</span><p><strong>Wait for instructions.</strong> Do not enter the share unless an Afterwords release email leads you to the recipient page.</p></div>
          <div><span>3</span><p><strong>Help when asked.</strong> The share is combined in the browser. It is never uploaded to Afterwords.</p></div>
        </section>
        <section className="share-panel">
          {qr ? <img src={qr} alt="QR code containing the private recovery share" /> : <div className="qr-placeholder" />}
          <div>
            <span className="micro-label">Private recovery share</span>
            <code>{payload.share}</code>
            <button className="text-button" onClick={() => { void navigator.clipboard.writeText(payload.share); setCopied(true) }}><Copy size={16} />{copied ? 'Copied' : 'Copy share'}</button>
          </div>
        </section>
        <footer className="kit-footer"><ShieldCheck size={20} /><p>The server stores only a fingerprint used to recognize this share. It cannot recreate it.</p></footer>
      </article>
      <nav className="kit-actions no-print" aria-label="Recovery Kit actions">
        <button className="secondary-button" onClick={() => window.print()}><Printer size={17} />Print or save PDF</button>
        <a className="secondary-button" href={downloadURL} download={filename}><Download size={17} />Download text</a>
      </nav>
    </main>
  )
}
