import { useState } from 'react'
import Dialog from './Dialog'
import { request, ApiError } from '../api/client'

interface CreateAliasDialogProps {
  accountId: string
  open: boolean
  onClose: () => void
  onCreated: (email: string) => void
}

/** 建立 Hide My Email 別名。 */
export default function CreateAliasDialog({
  accountId,
  open,
  onClose,
  onCreated,
}: CreateAliasDialogProps) {
  const [label, setLabel] = useState('')
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)

  function handleClose() {
    setLabel('')
    setError('')
    onClose()
  }

  async function handleSubmit() {
    if (submitting) return
    if (!label.trim()) {
      setError('請輸入標籤')
      return
    }
    setSubmitting(true)
    setError('')
    try {
      const data = await request<{ email: string }>('/api/create', {
        method: 'POST',
        body: JSON.stringify({ account_id: accountId, label: label.trim() }),
      })
      setLabel('')
      onCreated(data.email)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '網路連線失敗，請檢查服務狀態')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog
      title="建立別名"
      description="建立後會產生一個新的隱藏信箱，可用來註冊網站。"
      open={open}
      onClose={handleClose}
    >
      {error && (
        <div className="alert alert-error" role="alert">
          {error}
        </div>
      )}
      <div className="form-field">
        <label htmlFor="alias-label">標籤</label>
        <input
          id="alias-label"
          value={label}
          onChange={(e) => setLabel(e.target.value.slice(0, 200))}
          maxLength={200}
          placeholder="例如：購物、訂閱服務"
        />
        <p className="hint">標籤最長 200 字元，方便日後辨識用途。</p>
      </div>
      <div className="form-actions">
        <button onClick={handleClose}>取消</button>
        <button className="primary" onClick={() => void handleSubmit()} disabled={submitting}>
          {submitting ? '建立中…' : '建立別名'}
        </button>
      </div>
    </Dialog>
  )
}
