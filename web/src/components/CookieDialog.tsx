import { useState } from 'react'
import Dialog from './Dialog'
import CookieFields, {
  emptyCookieValue,
  toCookiePayload,
  type CookieValue,
} from './CookieFields'
import { request, ApiError } from '../api/client'

interface CookieDialogProps {
  accountId: string
  open: boolean
  onClose: () => void
  onSaved: () => void
}

/** 更新既有帳號的 Cookie：逐項填寫或貼上匯入。 */
export default function CookieDialog({ accountId, open, onClose, onSaved }: CookieDialogProps) {
  const [cookies, setCookies] = useState<CookieValue>(emptyCookieValue())
  // CookieFields 的「貼上匯入」內容是自己的區域狀態，父層清空 value 時不會跟著清。
  // 每次重置就換掉 key 讓它重新掛載，避免表單已清空但貼上框還留著舊內容。
  const [resetKey, setResetKey] = useState(0)
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)

  function resetFields() {
    setCookies(emptyCookieValue())
    setResetKey((key) => key + 1)
  }

  function handleClose() {
    resetFields()
    setError('')
    onClose()
  }

  async function handleSubmit() {
    if (submitting) return
    const payload = toCookiePayload(cookies)
    if (Object.keys(payload).length === 0) {
      setError('請至少填入一個 Cookie，或貼上匯出檔')
      return
    }
    setSubmitting(true)
    setError('')
    try {
      await request(`/api/accounts/${accountId}/cookies`, {
        method: 'PUT',
        body: JSON.stringify({ cookies: JSON.stringify(payload) }),
      })
      resetFields()
      onSaved()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '網路連線失敗，請檢查服務狀態')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog
      title="更新 Cookie"
      description="iCloud 的 Cookie 約 24 小時過期，失效後請重新匯入。"
      open={open}
      onClose={handleClose}
      wide
    >
      {error && (
        <div className="alert alert-error" role="alert">
          {error}
        </div>
      )}

      <CookieFields key={resetKey} value={cookies} onChange={setCookies} />

      <div className="form-actions">
        <button onClick={handleClose}>取消</button>
        <button className="primary" onClick={() => void handleSubmit()} disabled={submitting}>
          {submitting ? '儲存中…' : '儲存 Cookie'}
        </button>
      </div>
    </Dialog>
  )
}
