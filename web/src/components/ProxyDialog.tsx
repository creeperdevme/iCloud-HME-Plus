import { useState } from 'react'
import Dialog from './Dialog'
import { request, ApiError } from '../api/client'

interface ProxyDialogProps {
  accountId: string
  open: boolean
  onClose: () => void
  onSaved: () => void
}

/** 設定帳號專用代理；基於安全考量不回首目前值。 */
export default function ProxyDialog({ accountId, open, onClose, onSaved }: ProxyDialogProps) {
  const [proxy, setProxy] = useState('')
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)

  function handleClose() {
    setProxy('')
    setError('')
    onClose()
  }

  async function handleSubmit() {
    if (submitting) return
    setSubmitting(true)
    setError('')
    try {
      await request(`/api/accounts/${accountId}/proxy`, {
        method: 'PUT',
        body: JSON.stringify({ proxy: proxy.trim() }),
      })
      setProxy('')
      onSaved()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '網路連線失敗，請檢查服務狀態')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog
      title="設定代理"
      description="支援 http、https 與 socks5。留空送出即可清除代理。"
      open={open}
      onClose={handleClose}
    >
      {error && (
        <div className="alert alert-error" role="alert">
          {error}
        </div>
      )}
      <div className="form-field">
        <label htmlFor="proxy-input">代理位址</label>
        <input
          id="proxy-input"
          type="text"
          value={proxy}
          onChange={(e) => setProxy(e.target.value)}
          placeholder="http://user:pass@host:port"
          autoComplete="off"
          spellCheck={false}
        />
        <p className="hint">基於安全考量，目前的值不會回顯；留空並儲存即清除。</p>
      </div>
      <div className="form-actions">
        <button onClick={handleClose}>取消</button>
        <button className="primary" onClick={() => void handleSubmit()} disabled={submitting}>
          {submitting ? '儲存中…' : '儲存'}
        </button>
      </div>
    </Dialog>
  )
}
