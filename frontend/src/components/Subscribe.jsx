import { useState } from 'react'
import { subscribe } from '../api'

export default function Subscribe() {
  const [email, setEmail] = useState('')
  const [status, setStatus] = useState(null) // null | 'loading' | 'ok' | 'error'
  const [message, setMessage] = useState('')

  async function handleSubmit(e) {
    e.preventDefault()
    if (!email) return
    setStatus('loading')
    try {
      await subscribe(email)
      setStatus('ok')
      setMessage('订阅成功！后续资讯将发送到您的邮箱。')
      setEmail('')
    } catch (err) {
      setStatus('error')
      setMessage(err.message || '订阅失败，请稍后重试。')
    }
  }

  return (
    <div className="subscribe-box">
      <div className="subscribe-icon">✉</div>
      <h3 className="subscribe-title">邮件订阅</h3>
      <p className="subscribe-desc">订阅后，每次更新的技术资讯将直接发送到您的邮箱。</p>
      {status === 'ok' ? (
        <p className="subscribe-success">{message}</p>
      ) : (
        <form className="subscribe-form" onSubmit={handleSubmit}>
          <input
            className="subscribe-input"
            type="email"
            placeholder="your@email.com"
            value={email}
            onChange={e => setEmail(e.target.value)}
            required
            disabled={status === 'loading'}
          />
          <button
            className="subscribe-btn"
            type="submit"
            disabled={status === 'loading'}
          >
            {status === 'loading' ? '订阅中…' : '订阅'}
          </button>
        </form>
      )}
      {status === 'error' && <p className="subscribe-error">{message}</p>}
    </div>
  )
}
