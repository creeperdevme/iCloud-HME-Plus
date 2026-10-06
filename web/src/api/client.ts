import type { ApiResponse, TempMailbox, TempMailboxList, TempMailboxRemoval } from './types'

/** CSRF token，僅存於 React 記憶體狀態 */
let csrfToken: string | null = null

/** 全域 401 回呼（由 AuthProvider 註冊，避免循環 import） */
let unauthorizedHandler: (() => void) | null = null

/** 註冊全域 401 回呼 */
export function registerUnauthorizedHandler(handler: (() => void) | null): void {
  unauthorizedHandler = handler
}

/** 設定記憶體中的 CSRF token（由 AuthProvider 管理） */
export function setCSRFToken(token: string | null): void {
  csrfToken = token
}

/** ApiError 攜帶 HTTP 狀態與穩定錯誤碼 */
export class ApiError extends Error {
  readonly status: number
  readonly code: string

  constructor(status: number, code: string, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }
}

interface RequestOptions extends Omit<RequestInit, 'body'> {
  body?: unknown
  signal?: AbortSignal
}

/** 管理員工作階段失效時，伺服器端回傳的穩定錯誤碼（見 internal/server/auth.go） */
const AUTH_REQUIRED = 'AUTH_REQUIRED'

/**
 * 判斷一次 401 回應是否代表「管理員工作階段失效」。
 *
 * 只有伺服器明確回傳 AUTH_REQUIRED 才算工作階段過期。上游 iCloud 的業務錯誤
 * （UPSTREAM_UNAUTHORIZED / OTP_INVALID 等）不屬於管理員工作階段問題，一旦把
 * 它們也當成過期，任何 iCloud Cookie 失效都會把管理員踢回登入頁。
 *
 * 回應內容不是本服務的 JSON 時（例如反向代理直接攔截）視為工作階段失效。
 */
function isSessionExpired(payload: ApiResponse<unknown> | null): boolean {
  if (payload === null) return true
  return payload.code === AUTH_REQUIRED
}

/**
 * 唯一的 fetch 入口。
 *
 * 統一設定 Accept、JSON Content-Type 與 credentials: same-origin；
 * 非 GET/HEAD/OPTIONS 自動攜帶 X-CSRF-Token；
 * 僅當伺服器明確回傳 401／AUTH_REQUIRED 時觸發 onUnauthorized 回呼。
 */
export async function request<T>(
  path: string,
  init?: RequestOptions,
  onUnauthorized?: () => void,
): Promise<T> {
  const headers = new Headers(init?.headers)
  headers.set('Accept', 'application/json')
  headers.set('Content-Type', 'application/json')

  const method = (init?.method ?? 'GET').toUpperCase()
  if (method !== 'GET' && method !== 'HEAD' && method !== 'OPTIONS' && csrfToken) {
    headers.set('X-CSRF-Token', csrfToken)
  }

  let body: BodyInit | undefined
  if (init?.body !== undefined) {
    body = typeof init.body === 'string' ? init.body : JSON.stringify(init.body)
  }

  let resp: Response
  try {
    resp = await fetch(path, {
      ...init,
      method,
      headers,
      body,
      credentials: 'same-origin',
    })
  } catch {
    throw new ApiError(0, 'NETWORK_ERROR', '網路連線失敗，請檢查服務狀態')
  }

  let payload: ApiResponse<T> | null
  try {
    payload = (await resp.json()) as ApiResponse<T>
  } catch {
    payload = null
  }

  if (resp.status === 401 && isSessionExpired(payload)) {
    onUnauthorized?.()
    unauthorizedHandler?.()
  }

  if (payload === null) {
    throw new ApiError(resp.status, 'INVALID_RESPONSE', '網路連線失敗，請檢查服務狀態')
  }

  if (!resp.ok || payload.success === false) {
    throw new ApiError(
      resp.status,
      payload.code ?? 'INTERNAL_ERROR',
      payload.message ?? '請求失敗',
    )
  }
  return payload.data as T
}

/* ------------------------------------------------------------------ *
 * 隨機信箱（/api/temp）
 *
 * 都是 request() 的薄封裝：統一的 ApiError（status/code/message）與
 * 自動攜帶 X-CSRF-Token 都由 request() 處理，這裡只固定路徑與方法。
 * ------------------------------------------------------------------ */

/** 列出目前追蹤中的隨機信箱（含剩餘 TTL）。 */
export function listTempMailboxes(): Promise<TempMailboxList> {
  return request<TempMailboxList>('/api/temp')
}

/**
 * 建立一個隨機信箱。
 *
 * 未指定 accountId 時由後端挑選可用帳號；指定時使用該帳號。
 * 可能回 VALIDATION_ERROR（沒有可用帳號）或 502（上游 iCloud 失敗）。
 */
export function createTempMailbox(accountId?: string): Promise<TempMailbox> {
  return request<TempMailbox>('/api/temp', {
    method: 'POST',
    body: accountId ? { account_id: accountId } : {},
  })
}

/**
 * 切換「不自動刪除」。
 *
 * 關閉（keep=false）時後端會重新起算一輪 TTL。
 */
export function setTempMailboxKeep(id: string, keep: boolean): Promise<TempMailbox> {
  return request<TempMailbox>(`/api/temp/${encodeURIComponent(id)}/keep`, {
    method: 'POST',
    body: { keep },
  })
}

/** 停止追蹤並刪除隨機信箱（回應可能帶 upstream_warning）。 */
export function deleteTempMailbox(id: string): Promise<TempMailboxRemoval> {
  return request<TempMailboxRemoval>(`/api/temp/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
}
