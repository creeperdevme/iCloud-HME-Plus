import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../auth/AuthProvider'
import { ApiError } from '../api/client'
import { IconAlert, IconLock, IconShield } from '../components/icons'

export default function LoginPage() {
  const { login } = useAuth()
  const navigate = useNavigate()
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (submitting || !password) return
    setSubmitting(true)
    setError('')
    try {
      await login(password)
      navigate('/accounts', { replace: true })
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '網路連線失敗，請檢查服務狀態')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="login-page">
      <div className="login-card">
        <div className="login-brand">
          <span className="brand-mark" aria-hidden="true">
            <IconShield size={26} />
          </span>
          <h1>iCloud Hide My Email Dashboard</h1>
          <p>管理 iCloud 隱藏郵件別名與收件匣</p>
        </div>

        <form onSubmit={handleSubmit} className="login-form">
          {error && (
            <div className="alert alert-error" role="alert">
              <IconAlert size={16} />
              <span>{error}</span>
            </div>
          )}

          <div className="form-field">
            <label htmlFor="admin-password">管理員密碼</label>
            <div style={{ position: 'relative' }}>
              <input
                id="admin-password"
                type="password"
                autoComplete="current-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
                placeholder="請輸入管理員密碼"
                style={{ paddingRight: 40 }}
              />
              <span
                aria-hidden="true"
                style={{
                  position: 'absolute',
                  right: 12,
                  top: '50%',
                  transform: 'translateY(-50%)',
                  color: 'var(--text-3)',
                  display: 'flex',
                }}
              >
                <IconLock size={18} />
              </span>
            </div>
          </div>

          <div className="form-actions">
            <button type="submit" className="primary" disabled={submitting}>
              {submitting ? '登入中…' : '登入'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
