import { useState } from 'react'
import {
  KEY_COOKIES,
  formatLabel,
  parseCookies,
  splitKeyCookies,
  type CookieFormat,
} from '../utils/cookies'
import { IconCookie, IconInfo, IconCheck, IconAlert } from './icons'

export interface CookieValue {
  /** 四個關鍵 Cookie（key 為固定名稱） */
  fields: Record<string, string>
  /** 貼上匯入時識別到的其他 Cookie，會一併儲存 */
  extras: Record<string, string>
}

export function emptyCookieValue(): CookieValue {
  return { fields: {}, extras: {} }
}

/** 把欄位與額外 Cookie 合成要送給 API 的物件（略過空值）。 */
export function toCookiePayload(value: CookieValue): Record<string, string> {
  const out: Record<string, string> = {}
  for (const item of KEY_COOKIES) {
    const raw = (value.fields[item.key] ?? '').trim()
    if (raw) out[item.key] = raw
  }
  for (const [key, raw] of Object.entries(value.extras)) {
    const trimmed = (raw ?? '').trim()
    if (trimmed && !(key in out)) out[key] = trimmed
  }
  return out
}

type ParseStatus =
  | { kind: 'idle' }
  | { kind: 'ok'; format: CookieFormat; total: number; found: number; missing: string[] }
  | { kind: 'unknown' }

interface CookieFieldsProps {
  value: CookieValue
  onChange: (next: CookieValue) => void
  /** 已存在的 Cookie 不會回顯，因此編輯既有帳號時只當作「更新」用 */
  disabled?: boolean
}

/**
 * Cookie 輸入區：可切換「逐項填寫」與「貼上匯入」。
 *
 * 貼上匯入會自動識別 cookies.txt（Netscape）、JSON 與 Cookie Header 三種格式，
 * 並把四個關鍵 Cookie 自動填入對應欄位。
 */
export default function CookieFields({ value, onChange, disabled = false }: CookieFieldsProps) {
  const [mode, setMode] = useState<'manual' | 'paste'>('manual')
  const [pasted, setPasted] = useState('')
  const [status, setStatus] = useState<ParseStatus>({ kind: 'idle' })

  function handlePasted(next: string) {
    setPasted(next)
    if (!next.trim()) {
      setStatus({ kind: 'idle' })
      return
    }
    const result = parseCookies(next)
    if (result.format === 'unknown') {
      setStatus({ kind: 'unknown' })
      return
    }
    const { fields, extras } = splitKeyCookies(result.cookies)
    onChange({ fields, extras })
    setStatus({
      kind: 'ok',
      format: result.format,
      total: Object.keys(result.cookies).length,
      found: Object.keys(fields).length,
      missing: KEY_COOKIES.filter((item) => !(item.key in fields)).map((item) => item.key),
    })
  }

  return (
    <div className="cookie-section">
      <div className="card-head" style={{ marginBottom: 12 }}>
        <div>
          <span className="card-title" style={{ display: 'flex', alignItems: 'center', gap: 7 }}>
            <IconCookie size={16} />
            Cookie
          </span>
          <p className="card-sub">至少填入金鑰 Cookie，或直接貼上匯出檔自動識別。</p>
        </div>
        <div className="segmented" role="group" aria-label="Cookie 輸入方式">
          <button
            type="button"
            aria-pressed={mode === 'manual'}
            onClick={() => setMode('manual')}
            disabled={disabled}
          >
            逐項填寫
          </button>
          <button
            type="button"
            aria-pressed={mode === 'paste'}
            onClick={() => setMode('paste')}
            disabled={disabled}
          >
            貼上匯入
          </button>
        </div>
      </div>

      {mode === 'manual' ? (
        <div className="cookie-grid">
          {KEY_COOKIES.map((item) => (
            <div className="form-field" key={item.key}>
              <label htmlFor={`cookie-${item.key}`}>
                {item.key}
                <span className="hint" style={{ fontWeight: 400, marginLeft: 6 }}>
                  {item.hint}
                </span>
              </label>
              <input
                id={`cookie-${item.key}`}
                className="cell-mono"
                type="text"
                value={value.fields[item.key] ?? ''}
                onChange={(e) =>
                  onChange({ ...value, fields: { ...value.fields, [item.key]: e.target.value } })
                }
                placeholder="貼上此 Cookie 的值"
                autoComplete="off"
                spellCheck={false}
                disabled={disabled}
              />
            </div>
          ))}
        </div>
      ) : (
        <>
          <div className="form-field">
            <label htmlFor="cookie-paste">貼上驗證資訊</label>
            <textarea
              id="cookie-paste"
              value={pasted}
              onChange={(e) => handlePasted(e.target.value)}
              placeholder={
                '# Netscape HTTP Cookie File\n.icloud.com\tTRUE\t/\tTRUE\t0\tX-APPLE-WEBAUTH-TOKEN\tabc…\n\n或 {"X-APPLE-WEBAUTH-TOKEN":"abc…"}'
              }
              spellCheck={false}
              disabled={disabled}
              style={{ minHeight: 132 }}
            />
            <p className="hint">
              支援「Get cookies.txt LOCALLY」匯出的 cookies.txt、JSON 物件／陣列，以及 Cookie Header
              字串；貼上後會自動識別並填入下方四個欄位。
            </p>
          </div>

          {status.kind === 'ok' && (
            <div className="alert alert-success" role="status">
              <IconCheck size={16} />
              <span>
                已識別 <strong>{formatLabel(status.format)}</strong> 格式，共 {status.total} 個
                Cookie，其中 {status.found}／{KEY_COOKIES.length} 個關鍵欄位已自動填入。
                {status.missing.length > 0 && `（缺少：${status.missing.join('、')}）`}
              </span>
            </div>
          )}
          {status.kind === 'unknown' && (
            <div className="alert alert-error" role="alert">
              <IconAlert size={16} />
              <span>無法識別這段內容，請確認已複製完整的 cookies.txt 或 JSON。</span>
            </div>
          )}

          <div className="cookie-grid">
            {KEY_COOKIES.map((item) => {
              const filled = (value.fields[item.key] ?? '').trim() !== ''
              return (
                <div className="form-field" key={item.key}>
                  <label htmlFor={`cookie-paste-${item.key}`}>
                    {item.key}
                  </label>
                  <input
                    id={`cookie-paste-${item.key}`}
                    className="cell-mono"
                    type="text"
                    value={value.fields[item.key] ?? ''}
                    onChange={(e) =>
                      onChange({
                        ...value,
                        fields: { ...value.fields, [item.key]: e.target.value },
                      })
                    }
                    placeholder="（未識別到，可手動填入）"
                    autoComplete="off"
                    spellCheck={false}
                    disabled={disabled}
                    data-filled={filled ? 'true' : 'false'}
                  />
                </div>
              )
            })}
          </div>

          {Object.keys(value.extras).length > 0 && (
            <div className="alert alert-info" role="status">
              <IconInfo size={16} />
              <span>
                另有 {Object.keys(value.extras).length} 個非關鍵 Cookie（
                {Object.keys(value.extras).slice(0, 4).join('、')}
                {Object.keys(value.extras).length > 4 ? '…' : ''}
                ）將一併儲存。
              </span>
            </div>
          )}
        </>
      )}
    </div>
  )
}
