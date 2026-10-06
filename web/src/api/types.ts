/**
 * API 型別定義 — 與 internal/server 的凍結契約保持一致。
 */

/** 統一回應包裹 */
export interface ApiResponse<T> {
  success: boolean
  data?: T
  code?: string
  message?: string
  /**
   * 成功但有非致命問題時的提示（例如新增帳號時 App 專用密碼沒通過驗證，
   * 因此沒有儲存）。失敗一律走 code/message。
   */
  warning?: string
}

/** 帳號安全摘要（不含任何祕密欄位） */
export interface AccountSummary {
  id: string
  name: string
  real_email: string
  icloud_email: string
  host: string
  status: 'active' | 'pending' | 'error' | string
  alias_total: number
  alias_active: number
  has_cookies: boolean
  has_app_password: boolean
  has_proxy: boolean
  mailbox?: MailboxSummary
  last_validated: string
  status_message?: string
  created_at: string
}

export interface MailboxSummary {
  provider: string
  email: string
  imap_host: string
  imap_port: number
}

/** HME 別名（iCloud 回傳欄位為 camelCase） */
export interface Alias {
  email: string
  anonymousId: string
  label: string
  active: boolean
  createdAt?: string
}

/** 郵件摘要 */
export interface InboxMessage {
  id: string
  from: string
  to: string
  subject: string
  date: string
  preview: string
}

export interface FullMessage extends InboxMessage {
  body: string
  content_type: string
}

/** 收件匣查詢結果 */
export interface InboxResult {
  account_id: string
  alias?: string
  count: number
  messages: InboxMessage[]
  method: 'imap' | 'web_api'
}

/** 登入回應 */
export interface LoginResult {
  csrf_token: string
  expires_at: string
}

/**
 * 隨機信箱（後端 /api/temp 的公開表示）。
 *
 * `keep === true` 時後端不會自動刪除，且 `expires_at` 是零值
 * （`0001-01-01T00:00:00Z`）；UI 不可把它當成「已過期」。
 */
export interface TempMailbox {
  /** 別名識別碼 */
  id: string
  /** 隨機信箱位址 */
  email: string
  account_id: string
  /** 後端產生的可讀標籤，例如 temp-jade-reef-4821 */
  label: string
  /** RFC3339 */
  created_at: string
  /** RFC3339；`keep` 為 true 時是零值 */
  expires_at: string
  /** true = 不自動刪除 */
  keep: boolean
  attempts?: number
  last_error?: string
}

/** GET /api/temp 的回應 */
export interface TempMailboxList {
  count: number
  mailboxes: TempMailbox[]
  /** 自動刪除的存活秒數（後端目前為 24 小時） */
  ttl_seconds: number
}

/** DELETE /api/temp/:id 的回應 */
export interface TempMailboxRemoval {
  id: string
  email: string
  removed: boolean
  /** 本機已停止追蹤，但 iCloud 端刪除失敗時的原因 */
  upstream_warning?: string
}
