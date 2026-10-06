import { useState } from 'react'
import Dialog from './Dialog'
import { request, ApiError } from '../api/client'

interface AppPasswordDialogProps {
  accountId: string
  /**
   * 帳號已儲存的 iCloud 信箱。
   *
   * 由既有帳號開啟時一定會有值，此時使用者不必再打一次完整信箱；只有在
   * 帳號真的還沒有信箱時才需要手動輸入。
   */
  icloudEmail?: string
  open: boolean
  onClose: () => void
  onSaved: () => void
}

/**
 * 設定 App 專用密碼。
 *
 * 設定後讀信會優先走 IMAP（可用 App 專用密碼），比 Cookie 路徑更穩定。
 *
 * 這是從「既有帳號」開啟的對話框，所以信箱預設沿用帳號上已存的值並唯讀
 * 顯示；後端在沒帶 icloud_email 時也會自己沿用，兩邊行為一致。
 */
export default function AppPasswordDialog({
  accountId,
  icloudEmail = '',
  open,
  onClose,
  onSaved,
}: AppPasswordDialogProps) {
  const storedEmail = icloudEmail.trim()
  const hasStoredEmail = storedEmail !== ''

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
    if (!hasStoredEmail && !email.trim()) {
      setError('請輸入完整 iCloud 信箱')
      return
    }
    if (!appPassword.trim()) {
      setError('請輸入 App 專用密碼')
      return
    }
    setSubmitting(true)
    setError('')
    try {
      await request(`/api/accounts/${accountId}/password`, {
        method: 'POST',
        body: JSON.stringify({
          // 已經有信箱時留空，交由後端沿用帳號上的位址
          icloud_email: hasStoredEmail ? '' : email.trim(),
          app_password: appPassword.trim(),
        }),
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

      {hasStoredEmail ? (
        <div className="form-field">
          <span className="hint">iCloud 信箱</span>
          <p className="cell-strong" id="apppwd-email-readonly">
            {storedEmail}
          </p>
          <p className="hint">沿用此帳號已設定的信箱，不必重新輸入。</p>
        </div>
      ) : (
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
      )}

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
