import { FormEvent, useState } from 'react'
import { ArrowRight, Eye, EyeOff, LockKeyhole, Server, ShieldCheck } from 'lucide-react'
import { api } from '../lib/api'
import type { Owner } from '../types'
import { Brand, Spinner, ThemeButton } from './Brand'

interface Props {
  setupRequired: boolean
  setupTokenRequired: boolean
  dark: boolean
  onTheme: () => void
  onAuthenticated: (owner: Owner) => void
}

export function AuthScreen({ setupRequired, setupTokenRequired, dark, onTheme, onAuthenticated }: Props) {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [setupToken, setSetupToken] = useState('')
  const [visible, setVisible] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  async function submit(event: FormEvent) {
    event.preventDefault()
    setError('')
    setBusy(true)
    try {
      const result = setupRequired ? await api.setup(email, password, setupToken) : await api.login(email, password)
      onAuthenticated(result.user)
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : 'Something went wrong. Please try again.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <main className="auth-shell">
      <div className="ambient ambient-one" />
      <div className="ambient ambient-two" />
      <header className="public-header">
        <Brand compact />
        <ThemeButton dark={dark} onToggle={onTheme} />
      </header>
      <section className="auth-grid">
        <div className="auth-story">
          <p className="eyebrow">Private by design. Present when needed.</p>
          <h1>Say what matters,<br /><em>when it matters.</em></h1>
          <p className="lede">
            Check in on your schedule. If you cannot, Afterwords delivers your encrypted messages to the people you chose—without ever knowing what they say.
          </p>
          <div className="trust-row">
            <div><ShieldCheck /><span><strong>Zero knowledge</strong><small>Encryption stays in your browser</small></span></div>
            <div><Server /><span><strong>Self-hosted</strong><small>Your machine, your timeline</small></span></div>
            <div><LockKeyhole /><span><strong>Trustee recovery</strong><small>No single key holder</small></span></div>
          </div>
        </div>
        <div className="auth-card paper-card">
          <span className="card-kicker">{setupRequired ? 'First-run setup' : 'Welcome back'}</span>
          <h2>{setupRequired ? 'Create your private space' : 'Open your dashboard'}</h2>
          <p>{setupRequired ? 'This becomes the only owner account on this instance.' : 'Your vault passphrases are separate and never sent here.'}</p>
          <form onSubmit={submit} className="form-stack">
            <label>
              <span>Email address</span>
              <input data-testid="auth-email" type="email" autoComplete="email" value={email} onChange={(event) => setEmail(event.target.value)} placeholder="you@example.com" required />
            </label>
            {setupRequired && setupTokenRequired && <label>
              <span>Instance setup token</span>
              <input data-testid="setup-token" type="password" autoComplete="off" value={setupToken} onChange={(event) => setSetupToken(event.target.value)} placeholder="From your server environment" required />
            </label>}
            <label>
              <span>{setupRequired ? 'Account password' : 'Password'}</span>
              <span className="password-field">
                <input data-testid="auth-password" type={visible ? 'text' : 'password'} autoComplete={setupRequired ? 'new-password' : 'current-password'} minLength={12} value={password} onChange={(event) => setPassword(event.target.value)} placeholder={setupRequired ? 'At least 12 characters' : 'Your account password'} required />
                <button type="button" aria-label={visible ? 'Hide password' : 'Show password'} onClick={() => setVisible(!visible)}>{visible ? <EyeOff size={18} /> : <Eye size={18} />}</button>
              </span>
            </label>
            {error && <div className="form-error" role="alert">{error}</div>}
            <button data-testid="auth-submit" className="primary-button full-button" disabled={busy}>
              {busy ? <Spinner label={setupRequired ? 'Creating account' : 'Signing in'} /> : <>{setupRequired ? 'Begin setup' : 'Sign in'}<ArrowRight size={18} /></>}
            </button>
          </form>
          <p className="fine-print">Account passwords are Argon2id-hashed. Vault passphrases never leave this browser.</p>
        </div>
      </section>
    </main>
  )
}
