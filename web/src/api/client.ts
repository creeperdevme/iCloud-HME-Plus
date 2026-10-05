import type { ApiResponse } from './types'

/** CSRF token,仅存 React 内存状态 */
let csrfToken: string | null = null

/** 全局 401 回调(由 AuthProvider 注册,避免循环 import) */
let unauthorizedHandler: (() => void) | null = null

/** 注册全局 401 回调 */
export function registerUnauthorizedHandler(handler: (() => void) | null): void {
  unauthorizedHandler = handler
}

/** 设置内存 CSRF token(由 AuthProvider 管理) */
export function setCSRFToken(token: string | null): void {
  csrfToken = token
}

/** ApiError 携带 HTTP 状态与稳定错误码 */
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

/** 管理员会话失效时服务端返回的稳定错误码(见 internal/server/auth.go) */
const AUTH_REQUIRED = 'AUTH_REQUIRED'

/**
 * 判断一次 401 响应是否代表"管理员会话失效"。
 *
 * 只有服务端明确返回 AUTH_REQUIRED 才算会话过期。上游 iCloud 的业务错误
 * (UPSTREAM_UNAUTHORIZED / OTP_INVALID 等)不属于管理员会话问题,一旦把它们
 * 也当成会话过期,任何 iCloud Cookie 过期都会把管理员踢回登录页。
 *
 * 响应体不是本服务的 JSON 时(例如反向代理直接拦截)按会话失效处理。
 */
function isSessionExpired(payload: ApiResponse<unknown> | null): boolean {
  if (payload === null) return true
  return payload.code === AUTH_REQUIRED
}

/**
 * 唯一的 fetch 入口。
 *
 * 统一设置 Accept、JSON Content-Type 与 credentials: same-origin;
 * 非 GET/HEAD/OPTIONS 自动携带 X-CSRF-Token;
 * 仅当服务端明确返回 401/AUTH_REQUIRED 时触发 onUnauthorized 回调。
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
    throw new ApiError(0, 'NETWORK_ERROR', '网络连接失败，请检查服务状态')
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
    throw new ApiError(resp.status, 'INVALID_RESPONSE', '网络连接失败，请检查服务状态')
  }

  if (!resp.ok || payload.success === false) {
    throw new ApiError(
      resp.status,
      payload.code ?? 'INTERNAL_ERROR',
      payload.message ?? '请求失败',
    )
  }
  return payload.data as T
}
