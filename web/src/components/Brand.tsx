import { Moon, Sun } from 'lucide-react'

export function Mark({ small = false }: { small?: boolean }) {
  return (
    <span className={small ? 'brand-mark brand-mark-small' : 'brand-mark'} aria-hidden="true">
      <span />
      <span />
      <span />
    </span>
  )
}

export function Brand({ compact = false }: { compact?: boolean }) {
  return (
    <div className="brand-lockup">
      <Mark small={compact} />
      <span className={compact ? 'brand-name brand-name-small' : 'brand-name'}>Afterwords</span>
    </div>
  )
}

export function ThemeButton({ dark, onToggle }: { dark: boolean; onToggle: () => void }) {
  return (
    <button type="button" className="icon-button" aria-label={dark ? 'Use light mode' : 'Use dark mode'} onClick={onToggle}>
      {dark ? <Sun size={18} /> : <Moon size={18} />}
    </button>
  )
}

export function Spinner({ label = 'Loading' }: { label?: string }) {
  return (
    <span className="spinner-wrap" role="status">
      <span className="spinner" aria-hidden="true" />
      <span>{label}</span>
    </span>
  )
}
