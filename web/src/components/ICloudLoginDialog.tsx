import { useState } from 'react'
import Dialog from './Dialog'
import { request, ApiError } from '../api/client'

interface ICloudLoginDialogProps {
  accountId: string
  open: boolean
  onClose: () => void
  onSaved: () => void
}

/** 以 Apple ID 密碼登入 iCloud，取得可用的 Cookie；支援兩階段 OTP。 */
export default function ICloudLoginDialog({
  accountId,
  open,
  onClose,
  onSaved,
}: ICloudLoginDialogProps) {
  const [password, setPassword] = useState('')
  const [otp, setOtp] = useState('')
  const [otpRequired, setOtpRequired] = useState(false)
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)

  function handleClose() {
    setPassword('')
    setOtp('')
    setOtpRequired(false)
    setError('')
    onClose()
  }

  async function handleSubmit() {
    if (submitting) return
    if (!otpRequired && !password.trim()) {
      setError('請輸入 Apple ID 密碼')
      return
    }
    if (otpRequired && !otp.trim()) {
      setError('請輸入驗證碼')
      return
    }
    setSubmitting(true)
    setError('')
    try {
      await request(`/api/accounts/${accountId}/login`, {
        method: 'POST',
        body: JSON.stringify({
          password,
          ...(otpRequired ? { otp_code: otp } : {}),
        }),
      })
      setPassword('')
      setOtp('')
      setOtpRequired(false)
      onSaved()
    } catch (err) {
      if (err instanceof ApiError && err.code === 'OTP_REQUIRED') {
        setOtpRequired(true)
      } else {
        setError(err instanceof ApiError ? err.message : '網路連線失敗，請檢查服務狀態')
      }
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog
      title="iCloud 登入"
      description="使用 Apple ID 密碼換取新的 Cookie，登入後會自動儲存。"
      open={open}
      onClose={handleClose}
    >
      {error && (
        <div className="alert alert-error" role="alert">
          {error}
        </div>
      )}

      {otpRequired && (
        <div className="alert alert-info">
          <span>此帳號啟用了雙重認證，請輸入傳送到你裝置的 6 位數驗證碼。</span>
        </div>
      )}

      {!otpRequired && (
        <div className="form-field">
          <label htmlFor="icloud-login-password">Apple ID 密碼</label>
          <input
            id="icloud-login-password"
            type="password"
            autoComplete="current-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </div>
      )}

      {otpRequired && (
        <div className="form-field">
          <label htmlFor="icloud-login-otp">驗證碼</label>
          <input
            id="icloud-login-otp"
            type="text"
            inputMode="numeric"
            maxLength={6}
            value={otp}
            onChange={(e) => setOtp(e.target.value.replace(/\D/g, ''))}
            autoComplete="one-time-code"
            placeholder="123456"
          />
        </div>
      )}

      <div className="form-actions">
        <button onClick={handleClose}>取消</button>
        <button className="primary" onClick={() => void handleSubmit()} disabled={submitting}>
          {submitting ? '登入中…' : otpRequired ? '驗證' : '登入'}
        </button>
      </div>
    </Dialog>
  )
}
