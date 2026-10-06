import { useState } from 'react'
import Dialog from './Dialog'

interface ConfirmDialogProps {
  title: string
  message: string
  confirmLabel?: string
  requireText?: string
  requireLabel?: string
  open: boolean
  onClose: () => void
  onConfirm: () => void
  busy?: boolean
}

/** 破壞性操作確認對話框：可要求輸入完全相符的文字後才可確認。 */
export default function ConfirmDialog({
  title,
  message,
  confirmLabel = '確認刪除',
  requireText,
  requireLabel = '輸入名稱以確認',
  open,
  onClose,
  onConfirm,
  busy = false,
}: ConfirmDialogProps) {
  const [input, setInput] = useState('')
  const matched = !requireText || input === requireText

  function handleClose() {
    setInput('')
    onClose()
  }

  return (
    <Dialog title={title} open={open} onClose={handleClose}>
      <p className="card-sub" style={{ fontSize: 13.5, color: 'var(--text-2)' }}>
        {message}
      </p>
      {requireText && (
        <div className="form-field" style={{ marginTop: 16 }}>
          <label htmlFor="confirm-text">{requireLabel}</label>
          <input
            id="confirm-text"
            value={input}
            onChange={(e) => setInput(e.target.value)}
            autoComplete="off"
            spellCheck={false}
          />
          <p className="hint">需完全相符：{requireText}</p>
        </div>
      )}
      <div className="form-actions">
        <button onClick={handleClose}>取消</button>
        <button
          className="danger"
          disabled={!matched || busy}
          onClick={() => {
            setInput('')
            onConfirm()
          }}
        >
          {busy ? '處理中…' : confirmLabel}
        </button>
      </div>
    </Dialog>
  )
}
