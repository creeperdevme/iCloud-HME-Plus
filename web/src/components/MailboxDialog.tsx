import { useState } from 'react'
import Dialog from './Dialog'
import { request, ApiError } from '../api/client'
import type { MailboxSummary } from '../api/types'

interface MailboxDialogProps {
  accountId: string
  current?: MailboxSummary
  open: boolean
  onClose: () => void
  onSaved: () => void
}

const PRESETS: Record<string, { host: string; port: number }> = {
  qq: { host: 'imap.qq.com', port: 993 },
  gmail: { host: 'imap.gmail.com', port: 993 },
  outlook: { host: 'outlook.office365.com', port: 993 },
}

/**
 * 接入外部收件信箱（IMAP）。
 *
 * 由呼叫端以條件渲染掛載，因此初始值直接取自 props 即可，不需要 effect 同步。
 */
export default function MailboxDialog({
  accountId,
  current,
  open,
  onClose,
  onSaved,
}: MailboxDialogProps) {
  const [provider, setProvider] = useState(current?.provider || 'qq')
  const [email, setEmail] = useState(current?.email || '')
  const [host, setHost] = useState(current?.imap_host || PRESETS.qq.host)
  const [port, setPort] = useState(String(current?.imap_port || PRESETS.qq.port))
  const [code, setCode] = useState('')
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)

  function changeProvider(value: string) {
    setProvider(value)
    const preset = PRESETS[value]
    if (preset) {
      setHost(preset.host)
      setPort(String(preset.port))
    }
  }

  async function handleSubmit() {
    if (submitting) return
    if (!email.trim() || !host.trim() || !code.trim()) {
      setError('請填寫收件信箱、IMAP 伺服器與授權碼')
      return
    }
    setSubmitting(true)
    setError('')
    try {
      await request(`/api/accounts/${accountId}/mailbox`, {
        method: 'PUT',
        body: JSON.stringify({
          provider,
          email: email.trim(),
          imap_host: host.trim(),
          imap_port: Number(port),
          authorization_code: code.trim(),
        }),
      })
      onSaved()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '收件信箱接入失敗')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog
      title="接入收件信箱"
      description="別名收到的郵件會轉寄到這個信箱，之後可直接在收件匣頁面讀取。"
      open={open}
      onClose={onClose}
    >
      {error && (
        <div className="alert alert-error" role="alert">
          {error}
        </div>
      )}
      <div className="form-field">
        <label htmlFor="mailbox-provider">信箱服務商</label>
        <select
          id="mailbox-provider"
          value={provider}
          onChange={(e) => changeProvider(e.target.value)}
        >
          <option value="qq">QQ 信箱</option>
          <option value="gmail">Gmail</option>
          <option value="outlook">Outlook</option>
          <option value="custom">其他（自訂 IMAP）</option>
        </select>
      </div>

      <div className="form-row">
        <div className="form-field">
          <label htmlFor="mailbox-email">收件信箱</label>
          <input
            id="mailbox-email"
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            autoComplete="off"
          />
        </div>
        <div className="form-field">
          <label htmlFor="mailbox-code">信箱授權碼</label>
          <input
            id="mailbox-code"
            type="password"
            autoComplete="off"
            value={code}
            onChange={(e) => setCode(e.target.value)}
          />
        </div>
      </div>

      <div className="form-row">
        <div className="form-field">
          <label htmlFor="mailbox-host">IMAP 伺服器</label>
          <input
            id="mailbox-host"
            value={host}
            onChange={(e) => setHost(e.target.value)}
            autoComplete="off"
            spellCheck={false}
          />
        </div>
        <div className="form-field">
          <label htmlFor="mailbox-port">SSL 連接埠</label>
          <input
            id="mailbox-port"
            type="number"
            min="1"
            max="65535"
            value={port}
            onChange={(e) => setPort(e.target.value)}
          />
        </div>
      </div>

      <div className="form-actions">
        <button onClick={onClose}>取消</button>
        <button className="primary" onClick={() => void handleSubmit()} disabled={submitting}>
          {submitting ? '驗證中…' : '驗證並接入'}
        </button>
      </div>
    </Dialog>
  )
}
