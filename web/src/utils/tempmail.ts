/**
 * 隨機信箱頁面的純函式與常數。
 *
 * 與畫面分開放置，一方面是為了讓它們能單獨受測，另一方面是避免
 * TempMailPage 匯出非元件而觸發 react-refresh 警告。
 */

import type { AccountSummary, TempMailbox } from '../api/types'

/** 收件匣自動重新整理間隔（毫秒）。 */
export const INBOX_REFRESH_MS = 15_000

/** 收件匣一次抓幾封。 */
export const INBOX_LIMIT = 20

/** 收件匣回看天數。 */
export const INBOX_DAYS = 30

/** 後端 keep=true 時回傳的零值時間前綴。 */
export const ZERO_TIME_PREFIX = '0001-01-01'

/** 剩餘時間快於這個門檻就用警示色（毫秒）。 */
export const URGENT_THRESHOLD_MS = 60 * 60 * 1000

/**
 * 只有具備 Cookie 的帳號才能建立隨機信箱。
 *
 * 建立 HME 別名一定要有有效的 Cookie：App 專用密碼只能拿來登入換 Cookie，
 * 本身不足以呼叫 HME 介面。所以這裡不能用 has_app_password 放行，否則會把
 * 一個註定失敗的帳號列出來，還可能蓋掉另一個其實可用的帳號。
 */
export function isUsableAccount(acc: AccountSummary): boolean {
  return acc.has_cookies
}

/**
 * 解析到期時間。
 *
 * 回傳 null 代表「不會自動刪除」：keep=true，或後端給了零值時間
 * （0001-01-01T00:00:00Z）。零值**不能**當成已過期。
 */
export function expiresAtMs(mailbox: TempMailbox): number | null {
  if (mailbox.keep) return null
  if (mailbox.expires_at.startsWith(ZERO_TIME_PREFIX)) return null
  const ms = new Date(mailbox.expires_at).getTime()
  return Number.isNaN(ms) ? null : ms
}

/**
 * 把剩餘毫秒格式化成「23 小時 58 分」。
 *
 * 不足 1 小時時附上秒數，這樣畫面每秒都會有變化，也讓使用者對最後一小時
 * 更有感覺。零的單位會被省略，免得剛建立好的 24 小時信箱顯示成
 * 「1 天 0 小時 0 分」。
 */
export function formatRemaining(ms: number): string {
  if (ms <= 0) return '不到 1 秒'
  const totalSeconds = Math.floor(ms / 1000)
  const days = Math.floor(totalSeconds / 86400)
  const hours = Math.floor((totalSeconds % 86400) / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const seconds = totalSeconds % 60
  if (days > 0) return hours > 0 ? `${days} 天 ${hours} 小時` : `${days} 天`
  if (hours > 0) return `${hours} 小時 ${minutes} 分`
  if (minutes > 0) return `${minutes} 分 ${seconds} 秒`
  return `${seconds} 秒`
}

/** 絕對時間（列出其他隨機信箱的到期時間用）。 */
export function formatDateTime(raw: string): string {
  const d = new Date(raw)
  if (Number.isNaN(d.getTime())) return raw
  return new Intl.DateTimeFormat('zh-TW', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(d)
}

/** 時分秒（「最後更新」用）。 */
export function formatClock(date: Date): string {
  return new Intl.DateTimeFormat('zh-TW', {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).format(date)
}

/** 從追蹤清單取出最新建立的一筆。 */
export function latestMailbox(list: TempMailbox[]): TempMailbox | null {
  let best: TempMailbox | null = null
  for (const item of list) {
    if (best === null || Date.parse(item.created_at) > Date.parse(best.created_at)) {
      best = item
    }
  }
  return best
}
