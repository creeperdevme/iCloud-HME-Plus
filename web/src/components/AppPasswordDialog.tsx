import { useState } from 'react'
import Dialog from './Dialog'
import { request, ApiError } from '../api/client'

interface AppPasswordDialogProps {
  accountId: string
  open: boolean
  onClose: () => void
  onSaved: () => void
}

/**
 * 設定 App 專用密碼。
 *
 * 設定後讀信會優先走 IMAP（可用 App 專用密碼），比 Cookie 路徑更穩定。
 */
export default function AppPasswordDialog({
  accountId,
  open,
  onClose,
  onSaved,
}: AppPasswordDialogProps) {
  const [email, setEmail] = useState('')
  const [appPassword, setAppPassword] = useState('')
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)

  function handleClose() {
    setEmail('')
    setAppPassword('')
    setError('')
    onClose()
  }

  async function handleSubmit() {
    if (submitting) return
    if (!email.trim() || !appPassword.trim()) {
      setError('請輸入完整信箱與 App 專用密碼')
      return
    }
    setSubmitting(true)
    setError('')
    try {
      await request(`/api/accounts/${accountId}/password`, {
        method: 'POST',
        body: JSON.stringify({ icloud_email: email.trim(), app_password: appPassword.trim() }),
      })
      setEmail('')
      setAppPassword('')
      onSaved()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '網路連線失敗，請檢查服務狀態')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog
      title="設定 App 專用密碼"
      description="可於 appleid.apple.com → 登入與安全性 → App 專用密碼 產生。"
      open={open}
      onClose={handleClose}
    >
      {error && (
        <div className="alert alert-error" role="alert">
          {error}
        </div>
      )}
      <div className="form-field">
        <label htmlFor="apppwd-email">完整 iCloud 信箱</label>
        <input
          id="apppwd-email"
          type="email"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          placeholder="yourname@icloud.com"
          autoComplete="off"
        />
      </div>
      <div className="form-field">
        <label htmlFor="apppwd-value">App 專用密碼</label>
        <input
          id="apppwd-value"
          type="password"
          autoComplete="off"
          value={appPassword}
          onChange={(e) => setAppPassword(e.target.value)}
          placeholder="xxxx-xxxx-xxxx-xxxx"
        />
        <p className="hint">設定後會立即以 IMAP 驗證，通過才儲存。</p>
      </div>
      <div className="form-actions">
        <button onClick={handleClose}>取消</button>
        <button className="primary" onClick={() => void handleSubmit()} disabled={submitting}>
          {submitting ? '驗證中…' : '驗證並儲存'}
        </button>
      </div>
    </Dialog>
  )
}
