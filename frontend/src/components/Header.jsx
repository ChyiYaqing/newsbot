import { useState } from 'react'
import { subscribe } from '../api'

const TABS = [
  { value: '24h', label: '24h' },
  { value: '3days', label: '3 Days' },
  { value: '7days', label: '7 Days' },
]

function SubscribeInline() {
  const [email, setEmail] = useState('')
  const [status, setStatus] = useState(null) // null | 'loading' | 'ok' | 'error'

  async function handleSubmit(e) {
    e.preventDefault()
    if (!email) return
    setStatus('loading')
    try {
      await subscribe(email)
      setStatus('ok')
      setEmail('')
    } catch {
      setStatus('error')
      setTimeout(() => setStatus(null), 3000)
    }
  }

  if (status === 'ok') {
    return <span className="header-subscribe-ok">订阅成功 ✓</span>
  }

  return (
    <form className="header-subscribe" onSubmit={handleSubmit}>
      <input
        className="header-subscribe-input"
        type="email"
        placeholder="邮箱订阅"
        value={email}
        onChange={e => setEmail(e.target.value)}
        required
        disabled={status === 'loading'}
      />
      <button
        className="header-subscribe-btn"
        type="submit"
        disabled={status === 'loading'}
      >
        {status === 'loading' ? '…' : '订阅'}
      </button>
      {status === 'error' && <span className="header-subscribe-err">失败</span>}
    </form>
  )
}

export default function Header({ currentWindow, onWindowChange }) {
  return (
    <header className="header">
      <div className="container">
        <div className="header-inner">
          <div className="logo">
            <svg className="logo-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <path d="M4 22h16a2 2 0 0 0 2-2V4a2 2 0 0 0-2-2H8a2 2 0 0 0-2 2v16a2 2 0 0 1-2 2Zm0 0a2 2 0 0 1-2-2v-9c0-1.1.9-2 2-2h2" />
              <path d="M18 14h-8M15 18h-5M10 6h8v4h-8V6Z" />
            </svg>
            <span className="logo-text">NewsBot</span>
          </div>
          <div className="header-right">
            <nav className="tabs">
              {TABS.map(({ value, label }) => (
                <button
                  key={value}
                  className={`tab${currentWindow === value ? ' active' : ''}`}
                  onClick={() => onWindowChange(value)}
                >
                  {label}
                </button>
              ))}
            </nav>
            <SubscribeInline />
          </div>
        </div>
      </div>
    </header>
  )
}
