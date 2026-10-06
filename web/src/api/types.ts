/**
 * API 型別定義 — 與 internal/server 的凍結契約保持一致。
 */

/** 統一回應包裹 */
export interface ApiResponse<T> {
  success: boolean
  data?: T
  code?: string
  message?: string
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
