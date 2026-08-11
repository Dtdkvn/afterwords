import { useEffect, useState } from 'react'
import type { Owner } from './types'
import { api } from './lib/api'
import { AuthScreen } from './components/AuthScreen'
import { Brand, Spinner } from './components/Brand'
import { RecoveryKit } from './components/RecoveryKit'
import { RecipientPage } from './components/RecipientPage'
import { Dashboard } from './components/Dashboard'

function initialDarkMode() {
  const stored = localStorage.getItem('afterwords-theme')
  return stored ? stored === 'dark' : window.matchMedia('(prefers-color-scheme: dark)').matches
}

export default function App() {
  const [dark, setDark] = useState(initialDarkMode)
  const [loading, setLoading] = useState(true)
  const [setupRequired, setSetupRequired] = useState(false)
  const [setupTokenRequired, setSetupTokenRequired] = useState(false)
  const [owner, setOwner] = useState<Owner | null>(null)
  const [fatal, setFatal] = useState('')

  const path = window.location.pathname
  const releaseMatch = path.match(/^\/r\/([^/]+)$/u)

  useEffect(() => {
    document.documentElement.classList.toggle('dark', dark)
    localStorage.setItem('afterwords-theme', dark ? 'dark' : 'light')
  }, [dark])

  useEffect(() => {
    if (releaseMatch || path === '/kit') {
      setLoading(false)
      return
    }
    void api.bootstrap()
      .then((result) => {
        setSetupRequired(result.setupRequired)
        setSetupTokenRequired(result.setupTokenRequired)
        setOwner(result.authenticated ? result.user ?? null : null)
      })
      .catch((failure) => setFatal(failure instanceof Error ? failure.message : 'Afterwords could not start.'))
      .finally(() => setLoading(false))
  }, [path])

  if (path === '/kit') return <RecoveryKit />
  if (releaseMatch) return <RecipientPage token={decodeURIComponent(releaseMatch[1])} dark={dark} onTheme={() => setDark(!dark)} />
  if (loading) return <main className="loading-screen"><Brand /><Spinner label="Waking up gently" /></main>
  if (fatal) return <main className="loading-screen"><Brand /><h1>Afterwords could not open.</h1><p>{fatal}</p><button className="primary-button" onClick={() => window.location.reload()}>Try again</button></main>
  if (!owner) return <AuthScreen setupRequired={setupRequired} setupTokenRequired={setupTokenRequired} dark={dark} onTheme={() => setDark(!dark)} onAuthenticated={setOwner} />
  return <Dashboard owner={owner} dark={dark} onTheme={() => setDark(!dark)} onLogout={() => setOwner(null)} />
}
