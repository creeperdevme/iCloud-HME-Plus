/**
 * Cookie 解析工具。
 *
 * 支援自動識別三種常見格式：
 *  1. Netscape cookies.txt（"Get cookies.txt LOCALLY" 等擴充功能匯出的格式）
 *  2. JSON 物件 {"name":"value"} 或 JSON 陣列 [{"name":"…","value":"…"}]
 *  3. Cookie Header String "name1=value1; name2=value2"
 */

/** 四個關鍵 Cookie，對應 UI 上的四個欄位。 */
export const KEY_COOKIES = [
  {
    key: 'X-APPLE-WEBAUTH-TOKEN',
    hint: '主要驗證 Token',
  },
  {
    key: 'X-APPLE-WEBAUTH-USER',
    hint: '含 dsid（v=1:s=1:d=…）',
  },
  {
    key: 'X-APPLE-WEBAUTH-HSA-TRUST',
    hint: '裝置信任 Token',
  },
  {
    key: 'X-APPLE-DS-WEB-SESSION-TOKEN',
    hint: 'Web 工作階段 Token',
  },
] as const

export type CookieFormat = 'netscape' | 'json' | 'header' | 'unknown'

export interface ParsedCookies {
  format: CookieFormat
  cookies: Record<string, string>
}

/** 解析 Netscape cookies.txt。 */
function parseNetscape(text: string): Record<string, string> {
  const out: Record<string, string> = {}
  for (const rawLine of text.split(/\r?\n/)) {
    let line = rawLine.trim()
    if (!line) continue
    // "#HttpOnly_" 開頭的是真實 Cookie 行，不是註解。
    if (line.startsWith('#HttpOnly_')) {
      line = line.slice('#HttpOnly_'.length)
    } else if (line.startsWith('#')) {
      continue
    }

    const tabbed = line.split('\t')
    if (tabbed.length >= 7) {
      const name = tabbed[5].trim()
      const value = tabbed.slice(6).join('\t').trim()
      if (name) out[name] = value
      continue
    }
    // 容忍以空白分隔的變體：domain flag path secure expiry name value
    const match = line.match(/^(\S+)\s+(\S+)\s+(\S+)\s+(\S+)\s+(\S+)\s+(\S+)\s+(.*)$/)
    if (match) {
      out[match[6]] = match[7] ?? ''
    }
  }
  return out
}

/** 解析 JSON 物件或陣列。 */
function parseJSON(text: string): Record<string, string> | null {
  let data: unknown
  try {
    data = JSON.parse(text)
  } catch {
    return null
  }

  const out: Record<string, string> = {}

  if (Array.isArray(data)) {
    for (const item of data) {
      if (!item || typeof item !== 'object') continue
      const rec = item as Record<string, unknown>
      const name = typeof rec.name === 'string' ? rec.name : ''
      const value = rec.value == null ? '' : String(rec.value)
      if (name) out[name] = value
    }
    return Object.keys(out).length > 0 ? out : null
  }

  if (data && typeof data === 'object') {
    for (const [key, value] of Object.entries(data as Record<string, unknown>)) {
      if (value == null) continue
      if (typeof value === 'object') continue
      out[key] = String(value)
    }
    return Object.keys(out).length > 0 ? out : null
  }

  return null
}

/** 解析 Cookie Header String。 */
function parseHeader(text: string): Record<string, string> {
  const out: Record<string, string> = {}
  for (const part of text.split(';')) {
    const trimmed = part.trim()
    const idx = trimmed.indexOf('=')
    if (idx <= 0) continue
    const name = trimmed.slice(0, idx).trim()
    const value = trimmed.slice(idx + 1).trim()
    if (name) out[name] = value
  }
  return out
}

/** 判斷文字是否長得像 Netscape cookies.txt。 */
function looksLikeNetscape(text: string): boolean {
  if (/^#\s*Netscape HTTP Cookie File/im.test(text)) return true
  if (/^#HttpOnly_/m.test(text)) return true
  return text.split(/\r?\n/).some((line) => {
    const trimmed = line.trim()
    if (!trimmed || trimmed.startsWith('#')) return false
    return trimmed.split('\t').length >= 7
  })
}

/**
 * 自動識別格式並解析 Cookie。
 *
 * 解析不出任何 Cookie 時回傳 format='unknown' 與空物件。
 */
export function parseCookies(raw: string): ParsedCookies {
  const text = raw.trim()
  if (!text) return { format: 'unknown', cookies: {} }

  if (text.startsWith('{') || text.startsWith('[')) {
    const cookies = parseJSON(text)
    if (cookies) return { format: 'json', cookies }
  }

  if (looksLikeNetscape(text)) {
    const cookies = parseNetscape(text)
    if (Object.keys(cookies).length > 0) return { format: 'netscape', cookies }
  }

  const header = parseHeader(text)
  if (Object.keys(header).length > 0) return { format: 'header', cookies: header }

  return { format: 'unknown', cookies: {} }
}

/** 格式的中文名稱，用於提示訊息。 */
export function formatLabel(format: CookieFormat): string {
  switch (format) {
    case 'netscape':
      return 'cookies.txt（Netscape）'
    case 'json':
      return 'JSON'
    case 'header':
      return 'Cookie Header'
    default:
      return '未知格式'
  }
}

/**
 * 從解析結果中取出四個關鍵欄位的值，其餘歸類為額外 Cookie。
 *
 * 名稱比對不分大小寫。
 */
export function splitKeyCookies(cookies: Record<string, string>): {
  fields: Record<string, string>
  extras: Record<string, string>
} {
  const byLower = new Map<string, string>()
  for (const [key, value] of Object.entries(cookies)) {
    byLower.set(key.toLowerCase(), value)
  }

  const fields: Record<string, string> = {}
  const keySet = new Set(KEY_COOKIES.map((item) => item.key.toLowerCase()))

  for (const item of KEY_COOKIES) {
    const hit = byLower.get(item.key.toLowerCase())
    if (hit !== undefined) fields[item.key] = hit
  }

  const extras: Record<string, string> = {}
  for (const [key, value] of Object.entries(cookies)) {
    if (!keySet.has(key.toLowerCase())) extras[key] = value
  }

  return { fields, extras }
}

/** 四個關鍵欄位是否至少填了一項。 */
export function hasAnyKeyCookie(fields: Record<string, string>): boolean {
  return KEY_COOKIES.some((item) => (fields[item.key] ?? '').trim() !== '')
}
