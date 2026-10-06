import { useState } from 'react'
import Dialog from './Dialog'
import CookieFields, {
  emptyCookieValue,
  toCookiePayload,
  type CookieValue,
} from './CookieFields'
import { request, addAccount, ApiError } from '../api/client'
import type { AccountSummary } from '../api/types'

interface AccountFormDialogProps {
  open: boolean
  onClose: () => void
  /**
   * 儲存成功後呼叫。
   *
   * created 只在新增成功時帶入（編輯為 null）；warning 是後端的非致命提示,
   * 例如 App 專用密碼沒通過驗證、因此沒有儲存。
   */
  onSaved: (result: { created: AccountSummary | null; warning: string }) => void
  editing?: {
    id: string
    name: string
    icloudEmail: string
    host: string
  } | null
}

const HOSTS = [
  { value: 'icloud.com', label: '全球區', suffix: '@icloud.com' },
  { value: 'icloud.com.cn', label: '中國區', suffix: '@icloud.com.cn' },
]

/** 只保留 Prefix：使用者若貼上完整信箱，自動去掉 @ 之後的部分。 */
function toPrefix(raw: string): string {
  const trimmed = raw.trim()
  const at = trimmed.indexOf('@')
  return at >= 0 ? trimmed.slice(0, at) : trimmed
}

/**
 * 新增 / 編輯帳號對話框。
 *
 * 新增時不必填名稱，iCloud 信箱只需填 Prefix（後綴由區域決定），
 * Cookie 可逐項填寫或貼上匯出檔自動識別，App 專用密碼為選填
 * （填了就一起驗證，省去建立後再另外設定一次）。
 */
export default function AccountFormDialog({
  open,
  onClose,
  onSaved,
  editing,
}: AccountFormDialogProps) {
  const isEdit = Boolean(editing)
  const [name, setName] = useState(editing?.name ?? '')
  const [prefix, setPrefix] = useState(toPrefix(editing?.icloudEmail ?? ''))
  const [host, setHost] = useState(editing?.host ?? 'icloud.com')
  const [cookies, setCookies] = useState<CookieValue>(emptyCookieValue())
  const [appPassword, setAppPassword] = useState('')
  const [proxy, setProxy] = useState('')
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)

  const suffix = HOSTS.find((h) => h.value === host)?.suffix ?? '@icloud.com'
  const previewEmail = prefix.trim() ? `${prefix.trim()}${suffix}` : ''

  function reset() {
    setName('')
    setPrefix('')
    setHost('icloud.com')
    setCookies(emptyCookieValue())
    setAppPassword('')
    setProxy('')
    setError('')
    setSubmitting(false)
  }

  function handleClose() {
    reset()
    onClose()
  }

  async function handleSubmit() {
    if (submitting) return
    if (!prefix.trim()) {
      setError('請輸入 iCloud 信箱的 Prefix')
      return
    }
    setSubmitting(true)
    setError('')
    try {
      if (editing) {
        await request(`/api/accounts/${editing.id}`, {
          method: 'PATCH',
          body: JSON.stringify({
            name: name.trim() || undefined,
            // 只送 Prefix，後端會用（可能已更新的）區域補全
            icloud_email: prefix.trim(),
            host,
          }),
        })
        reset()
        onSaved({ created: null, warning: '' })
      } else {
        const cookiePayload = toCookiePayload(cookies)
        const { data, warning } = await addAccount({
          // 名稱留空時由後端以 Prefix 自動推導
          name: name.trim(),
          icloud_email: prefix.trim(),
          host,
          proxy: proxy.trim(),
          cookies: Object.keys(cookiePayload).length > 0 ? JSON.stringify(cookiePayload) : '',
          app_password: appPassword.trim(),
        })
        reset()
        onSaved({ created: data, warning })
      }
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '網路連線失敗，請檢查服務狀態')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog
      title={isEdit ? '編輯帳號' : '新增 iCloud 帳號'}
      description={
        isEdit
          ? '修改顯示名稱、信箱 Prefix 或區域。'
          : '只需要 iCloud 信箱的 Prefix，名稱留空會自動帶入；Cookie 與 App 專用密碼都是選填。'
      }
      open={open}
      onClose={handleClose}
      wide
    >
      {error && (
        <div className="alert alert-error" role="alert">
          {error}
        </div>
      )}

      <div className="form-row">
        <div className="form-field">
          <label htmlFor="acc-prefix">iCloud 信箱 Prefix</label>
          <div className="input-group">
            <input
              id="acc-prefix"
              value={prefix}
              onChange={(e) => setPrefix(toPrefix(e.target.value))}
              placeholder="yourname"
              autoComplete="off"
              spellCheck={false}
            />
            <span className="input-suffix">{suffix}</span>
          </div>
          <p className="hint">
            只需填寫 @ 前面的部分
            {previewEmail ? `，完整信箱為 ${previewEmail}` : ''}。
          </p>
        </div>

        <div className="form-field">
          <label htmlFor="acc-host">區域</label>
          <select
            id="acc-host"
            value={host}
            onChange={(e) => setHost(e.target.value)}
            disabled={isEdit}
          >
            {HOSTS.map((h) => (
              <option key={h.value} value={h.value}>
                {h.label}（{h.suffix}）
              </option>
            ))}
          </select>
          <p className="hint">決定信箱後綴與 iCloud 服務端點。</p>
        </div>
      </div>

      <div className="form-field">
        <label htmlFor="acc-name">
          顯示名稱 <span className="hint" style={{ fontWeight: 400 }}>（選填）</span>
        </label>
        <input
          id="acc-name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          maxLength={64}
          placeholder={prefix.trim() ? `留空將使用「${prefix.trim()}」` : '留空將使用 Prefix'}
        />
      </div>

      {!isEdit && (
        <>
          <CookieFields value={cookies} onChange={setCookies} />

          <div className="form-row">
            <div className="form-field">
              <label htmlFor="acc-app-password">
                App 專用密碼{' '}
                <span className="hint" style={{ fontWeight: 400 }}>
                  （選填）
                </span>
              </label>
              <input
                id="acc-app-password"
                type="password"
                value={appPassword}
                onChange={(e) => setAppPassword(e.target.value)}
                placeholder="xxxx-xxxx-xxxx-xxxx"
                autoComplete="off"
                spellCheck={false}
              />
              <p className="hint">
                可於 appleid.apple.com → 登入與安全性 → App 專用密碼
                產生。填寫後會立即以 IMAP 驗證，通過才儲存；信箱沿用上面的
                {previewEmail ? ` ${previewEmail}` : ' Prefix'}，不必再打一次。
              </p>
            </div>

            <div className="form-field">
              <label htmlFor="acc-proxy">
                代理{' '}
                <span className="hint" style={{ fontWeight: 400 }}>
                  （選填）
                </span>
              </label>
              <input
                id="acc-proxy"
                type="text"
                value={proxy}
                onChange={(e) => setProxy(e.target.value)}
                placeholder="http://user:pass@host:port"
                autoComplete="off"
                spellCheck={false}
              />
            </div>
          </div>
        </>
      )}

      {isEdit && (
        <div className="alert alert-info">
          <span>Cookie 與代理不在這裡修改，請用帳號列表上的「更新 Cookie」與「設定代理」。</span>
        </div>
      )}

      <div className="form-actions">
        <button onClick={handleClose}>取消</button>
        <button className="primary" onClick={() => void handleSubmit()} disabled={submitting}>
          {submitting ? '儲存中…' : isEdit ? '儲存變更' : '新增帳號'}
        </button>
      </div>
    </Dialog>
  )
}
