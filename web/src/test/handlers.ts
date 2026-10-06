import { http, HttpResponse } from 'msw'

export const handlers = [
  http.post('/api/auth/login', () =>
    HttpResponse.json(
      { success: false, code: 'INVALID_CREDENTIALS', message: '管理員密碼錯誤' },
      { status: 401 },
    ),
  ),
  http.get('/api/auth/session', () =>
    HttpResponse.json(
      { success: false, code: 'AUTH_REQUIRED', message: '請先登入' },
      { status: 401 },
    ),
  ),
  http.get('/api/accounts', () =>
    HttpResponse.json({ success: true, data: [] }),
  ),
]
