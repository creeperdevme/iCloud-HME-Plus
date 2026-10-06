import { http, HttpResponse } from 'msw'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import {
  createTempMailbox,
  deleteTempMailbox,
  listTempMailboxes,
  request,
  registerUnauthorizedHandler,
  setCSRFToken,
  setTempMailboxKeep,
} from './client'
import { server } from '../test/server'

/** 通用的網路失敗訊息（client.ts 統一使用繁體中文） */
const NETWORK_ERROR = '網路連線失敗，請檢查服務狀態'

describe('api client', () => {
  beforeEach(() => {
    setCSRFToken(null)
    registerUnauthorizedHandler(null)
    server.resetHandlers()
  })

  it('成功解包 data', async () => {
    server.use(
      http.get('/api/accounts', () =>
        HttpResponse.json({ success: true, data: { id: 'acc_1' } }),
      ),
    )
    const data = await request<{ id: string }>('/api/accounts')
    expect(data.id).toBe('acc_1')
  })

  it('非 2xx 拋出 ApiError 並攜帶 code/message/status', async () => {
    server.use(
      http.get('/api/accounts', () =>
        HttpResponse.json(
          { success: false, code: 'VALIDATION_ERROR', message: '參數錯誤' },
          { status: 400 },
        ),
      ),
    )
    await expect(request('/api/accounts')).rejects.toMatchObject({
      status: 400,
      code: 'VALIDATION_ERROR',
      message: '參數錯誤',
    })
  })

  it('非 JSON 回應拋出網路錯誤', async () => {
    server.use(
      http.get('/api/accounts', () =>
        new HttpResponse('<html>bad</html>', { status: 502 }),
      ),
    )
    await expect(request('/api/accounts')).rejects.toThrow(NETWORK_ERROR)
  })

  it('401 觸發全域回呼', async () => {
    const onUnauthorized = vi.fn()
    server.use(
      http.get('/api/accounts', () =>
        HttpResponse.json(
          { success: false, code: 'AUTH_REQUIRED', message: '請先登入' },
          { status: 401 },
        ),
      ),
    )
    request('/api/accounts', undefined, onUnauthorized).catch(() => {})
    await vi.waitFor(() => expect(onUnauthorized).toHaveBeenCalled())
  })

  // 回歸測試:面板「一直被登出」。
  // 上游 iCloud 的鑑權失敗帶的是自己的錯誤碼,不是管理員工作階段失效;
  // 若把它們也當成工作階段過期,任何 iCloud Cookie 過期都會把管理員踢回登入頁。
  it('401 但錯誤碼不是 AUTH_REQUIRED 時不觸發登出回呼', async () => {
    const onUnauthorized = vi.fn()
    const globalHandler = vi.fn()
    registerUnauthorizedHandler(globalHandler)
    server.use(
      http.get('/api/aliases', () =>
        HttpResponse.json(
          {
            success: false,
            code: 'UPSTREAM_UNAUTHORIZED',
            message: 'iCloud 工作階段失效,請更新 Cookie',
          },
          { status: 401 },
        ),
      ),
    )

    await expect(
      request('/api/aliases', undefined, onUnauthorized),
    ).rejects.toMatchObject({ status: 401, code: 'UPSTREAM_UNAUTHORIZED' })

    expect(onUnauthorized).not.toHaveBeenCalled()
    expect(globalHandler).not.toHaveBeenCalled()
  })

  it('401 + AUTH_REQUIRED 同時觸發局部與全域登出回呼', async () => {
    const onUnauthorized = vi.fn()
    const globalHandler = vi.fn()
    registerUnauthorizedHandler(globalHandler)
    server.use(
      http.get('/api/accounts', () =>
        HttpResponse.json(
          { success: false, code: 'AUTH_REQUIRED', message: '工作階段已失效,請重新登入' },
          { status: 401 },
        ),
      ),
    )

    await expect(
      request('/api/accounts', undefined, onUnauthorized),
    ).rejects.toMatchObject({ status: 401, code: 'AUTH_REQUIRED' })

    expect(onUnauthorized).toHaveBeenCalledTimes(1)
    expect(globalHandler).toHaveBeenCalledTimes(1)
  })

  it('登入接口 401 INVALID_CREDENTIALS 不觸發全域登出回呼', async () => {
    const globalHandler = vi.fn()
    registerUnauthorizedHandler(globalHandler)
    server.use(
      http.post('/api/auth/login', () =>
        HttpResponse.json(
          { success: false, code: 'INVALID_CREDENTIALS', message: '管理員密碼錯誤' },
          { status: 401 },
        ),
      ),
    )

    await expect(
      request('/api/auth/login', { method: 'POST', body: JSON.stringify({ password: 'x' }) }),
    ).rejects.toMatchObject({ status: 401, code: 'INVALID_CREDENTIALS' })

    expect(globalHandler).not.toHaveBeenCalled()
  })

  it('非 JSON 的 401(反向代理攔截)仍按工作階段失效處理', async () => {
    const globalHandler = vi.fn()
    registerUnauthorizedHandler(globalHandler)
    server.use(
      http.get('/api/accounts', () =>
        new HttpResponse('<html>401 Unauthorized</html>', { status: 401 }),
      ),
    )

    await expect(request('/api/accounts')).rejects.toThrow(NETWORK_ERROR)
    expect(globalHandler).toHaveBeenCalledTimes(1)
  })

  it('GET 不帶 CSRF,POST 自動帶 CSRF', async () => {
    setCSRFToken('csrf-token-123')
    let getHeaders: Headers | undefined
    let postHeaders: Headers | undefined
    server.use(
      http.get('/api/auth/session', ({ request }) => {
        getHeaders = request.headers
        return HttpResponse.json({ success: true, data: {} })
      }),
      http.post('/api/auth/logout', ({ request }) => {
        postHeaders = request.headers
        return HttpResponse.json({ success: true, data: { logged_out: true } })
      }),
    )
    await request('/api/auth/session')
    await request('/api/auth/logout', { method: 'POST' })
    expect(getHeaders?.get('X-CSRF-Token')).toBeNull()
    expect(postHeaders?.get('X-CSRF-Token')).toBe('csrf-token-123')
  })

  it('沒有 CSRF token 時 POST 不帶該標頭', async () => {
    let postHeaders: Headers | undefined
    server.use(
      http.post('/api/auth/logout', ({ request }) => {
        postHeaders = request.headers
        return HttpResponse.json({ success: true, data: {} })
      }),
    )
    await request('/api/auth/logout', { method: 'POST' })
    expect(postHeaders?.get('X-CSRF-Token')).toBeNull()
  })

  it('使用 credentials same-origin', async () => {
    let seen: RequestInit | undefined
    server.use(
      http.get('/api/accounts', ({ request }) => {
        seen = request as unknown as RequestInit
        return HttpResponse.json({ success: true, data: [] })
      }),
    )
    await request('/api/accounts')
    expect(seen?.credentials).toBe('same-origin')
  })

  it('支援 AbortSignal', async () => {
    const controller = new AbortController()
    server.use(
      http.get('/api/accounts', () =>
        HttpResponse.json({ success: true, data: [] }),
      ),
    )
    controller.abort()
    await expect(
      request('/api/accounts', { signal: controller.signal }),
    ).rejects.toThrow()
  })

  describe('隨機信箱 API', () => {
    it('listTempMailboxes 以 GET 取得追蹤清單', async () => {
      let method = ''
      server.use(
        http.get('/api/temp', ({ request }) => {
          method = request.method
          return HttpResponse.json({
            success: true,
            data: { count: 0, mailboxes: [], ttl_seconds: 86400 },
          })
        }),
      )
      const data = await listTempMailboxes()
      expect(method).toBe('GET')
      expect(data).toEqual({ count: 0, mailboxes: [], ttl_seconds: 86400 })
    })

    it('createTempMailbox 以 POST 帶上 account_id 與 CSRF 標頭', async () => {
      setCSRFToken('csrf-token-123')
      let method = ''
      let csrf: string | null = null
      let body: unknown = null
      server.use(
        http.post('/api/temp', async ({ request: req }) => {
          method = req.method
          csrf = req.headers.get('X-CSRF-Token')
          body = await req.json()
          return HttpResponse.json({ success: true, data: { id: 'temp_1' } })
        }),
      )
      const created = await createTempMailbox('acc_1')
      expect(method).toBe('POST')
      expect(csrf).toBe('csrf-token-123')
      expect(body).toEqual({ account_id: 'acc_1' })
      expect(created.id).toBe('temp_1')
    })

    it('createTempMailbox 未指定帳號時送空物件，由後端挑選', async () => {
      let body: unknown = null
      server.use(
        http.post('/api/temp', async ({ request: req }) => {
          body = await req.json()
          return HttpResponse.json({ success: true, data: { id: 'temp_1' } })
        }),
      )
      await createTempMailbox()
      expect(body).toEqual({})
    })

    it('setTempMailboxKeep 對 id 做 URL 編碼並送出 keep', async () => {
      let decodedId = ''
      let body: unknown = null
      server.use(
        http.post('/api/temp/:id/keep', async ({ params, request: req }) => {
          decodedId = String(params.id)
          body = await req.json()
          return HttpResponse.json({
            success: true,
            data: { id: decodedId, keep: true },
          })
        }),
      )
      const updated = await setTempMailboxKeep('temp/a b', true)
      expect(decodedId).toBe('temp/a b')
      expect(body).toEqual({ keep: true })
      expect(updated.keep).toBe(true)
    })

    it('deleteTempMailbox 保留 upstream_warning', async () => {
      let method = ''
      server.use(
        http.delete('/api/temp/:id', ({ params, request: req }) => {
          method = req.method
          return HttpResponse.json({
            success: true,
            data: {
              id: String(params.id),
              email: 'a@icloud.com',
              removed: true,
              upstream_warning: '別名不存在',
            },
          })
        }),
      )
      const result = await deleteTempMailbox('temp_1')
      expect(method).toBe('DELETE')
      expect(result.removed).toBe(true)
      expect(result.upstream_warning).toBe('別名不存在')
    })

    it('建立失敗時拋出 ApiError 並原樣保留後端訊息', async () => {
      server.use(
        http.post('/api/temp', () =>
          HttpResponse.json(
            { success: false, code: 'VALIDATION_ERROR', message: '沒有可用的帳號可建立隨機信箱' },
            { status: 400 },
          ),
        ),
      )
      await expect(createTempMailbox()).rejects.toMatchObject({
        status: 400,
        code: 'VALIDATION_ERROR',
        message: '沒有可用的帳號可建立隨機信箱',
      })
    })
  })
})
