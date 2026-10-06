import { http, HttpResponse } from 'msw'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { beforeEach, describe, expect, it } from 'vitest'
import type { ReactNode } from 'react'
import { useState } from 'react'
import { AuthProvider, useAuth } from './AuthProvider'
import { server } from '../test/server'
import LoginPage from '../pages/LoginPage'
import { ApiError, request, registerUnauthorizedHandler, setCSRFToken } from '../api/client'

/** 登入頁標題（LoginPage 的 <h1>）。 */
const APP_TITLE = 'iCloud HME Plus'

function ProtectedProbe() {
  const { status } = useAuth()
  if (status !== 'authenticated') return <p>loading…</p>
  return <p data-testid="protected">已登入頁面</p>
}

/** 模擬 App.tsx 的認證分支邏輯 */
function TestApp({ children }: { children?: ReactNode }) {
  const { status } = useAuth()
  if (status === 'checking') return <p>loading…</p>
  if (status === 'anonymous') return <LoginPage />
  return <>{children ?? <ProtectedProbe />}</>
}

function renderApp(initialPath = '/accounts') {
  return render(
    <MemoryRouter initialEntries={[initialPath]}>
      <AuthProvider>
        <Routes>
          <Route path="/login" element={<TestApp />} />
          <Route path="/accounts" element={<TestApp />} />
        </Routes>
      </AuthProvider>
    </MemoryRouter>,
  )
}

/** 未登入時 /api/auth/session 的 401 回應（與 internal/server 契約一致）。 */
const sessionExpired = () =>
  HttpResponse.json(
    { success: false, code: 'AUTH_REQUIRED', message: '請先登入' },
    { status: 401 },
  )

describe('AuthProvider + LoginPage', () => {
  beforeEach(() => {
    setCSRFToken(null)
    registerUnauthorizedHandler(null)
    server.resetHandlers()
  })

  it('首次訪問顯示 loading,工作階段校驗通過後進入受保護頁', async () => {
    server.use(
      http.get('/api/auth/session', () =>
        HttpResponse.json({
          success: true,
          data: {
            csrf_token: 'csrf-abc',
            expires_at: '2026-08-05T22:00:00+08:00',
          },
        }),
      ),
    )
    renderApp()
    expect(screen.getByText('loading…')).toBeInTheDocument()
    expect(await screen.findByTestId('protected')).toBeInTheDocument()
  })

  it('工作階段無效(401)顯示登入頁', async () => {
    server.use(http.get('/api/auth/session', sessionExpired))
    renderApp()
    expect(
      await screen.findByRole('heading', { name: APP_TITLE }),
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '登入' })).toBeInTheDocument()
  })

  it('登入成功進入 /accounts', async () => {
    server.use(
      http.get('/api/auth/session', sessionExpired),
      http.post('/api/auth/login', () =>
        HttpResponse.json({
          success: true,
          data: {
            csrf_token: 'csrf-new',
            expires_at: '2026-08-05T22:00:00+08:00',
          },
        }),
      ),
    )
    renderApp()
    const user = userEvent.setup()
    await screen.findByRole('heading', { name: APP_TITLE })
    await user.type(screen.getByLabelText(/管理員密碼/), 'admin-pass-2026')
    await user.click(screen.getByRole('button', { name: '登入' }))
    expect(await screen.findByTestId('protected')).toBeInTheDocument()
  })

  it('錯誤密碼顯示伺服器訊息且不回顯密碼', async () => {
    server.use(
      http.get('/api/auth/session', sessionExpired),
      http.post('/api/auth/login', () =>
        HttpResponse.json(
          { success: false, code: 'INVALID_CREDENTIALS', message: '管理員密碼錯誤' },
          { status: 401 },
        ),
      ),
    )
    renderApp()
    const user = userEvent.setup()
    await screen.findByRole('heading', { name: APP_TITLE })
    await user.type(screen.getByLabelText(/管理員密碼/), 'wrong-password')
    await user.click(screen.getByRole('button', { name: '登入' }))
    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent('管理員密碼錯誤')
    expect(alert).not.toHaveTextContent('wrong-password')
  })

  it('登入後重新整理頁面可恢復工作階段', async () => {
    server.use(
      http.get('/api/auth/session', () =>
        HttpResponse.json({
          success: true,
          data: {
            csrf_token: 'csrf-abc',
            expires_at: '2026-08-05T22:00:00+08:00',
          },
        }),
      ),
    )
    const { unmount } = renderApp()
    expect(await screen.findByTestId('protected')).toBeInTheDocument()
    unmount()
    renderApp()
    expect(await screen.findByTestId('protected')).toBeInTheDocument()
  })

  it('登出呼叫 logout 並回到登入頁', async () => {
    server.use(
      http.get('/api/auth/session', () =>
        HttpResponse.json({
          success: true,
          data: {
            csrf_token: 'csrf-abc',
            expires_at: '2026-08-05T22:00:00+08:00',
          },
        }),
      ),
      http.post('/api/auth/logout', () =>
        HttpResponse.json({ success: true, data: { logged_out: true } }),
      ),
    )
    render(
      <MemoryRouter initialEntries={['/accounts']}>
        <AuthProvider>
          <Routes>
            <Route path="/login" element={<TestApp />} />
            <Route path="/accounts" element={<LogoutProbe />} />
          </Routes>
        </AuthProvider>
      </MemoryRouter>,
    )
    const user = userEvent.setup()
    await screen.findByTestId('protected')
    await user.click(screen.getByRole('button', { name: '登出' }))
    await waitFor(() =>
      expect(screen.getByRole('heading', { name: APP_TITLE })).toBeInTheDocument(),
    )
  })

  // 回歸測試:面板「一直被登出」。
  // 上游 iCloud 的鑑權失敗屬於業務錯誤,不得影響管理員工作階段。
  // 兩種狀態碼都要覆蓋:502 是新後端的行為,401 是舊後端/防禦性場景。
  it.each([502, 401])(
    '業務接口回傳上游失效(HTTP %i)時管理員工作階段保持有效',
    async (status) => {
      server.use(
        http.get('/api/auth/session', () =>
          HttpResponse.json({
            success: true,
            data: {
              csrf_token: 'csrf-abc',
              expires_at: '2026-08-05T22:00:00+08:00',
            },
          }),
        ),
        http.get('/api/aliases', () =>
          HttpResponse.json(
            {
              success: false,
              code: 'UPSTREAM_UNAUTHORIZED',
              message: 'iCloud 工作階段失效，請更新 Cookie',
            },
            { status },
          ),
        ),
      )
      renderBusinessApp()
      await screen.findByTestId('protected')

      const user = userEvent.setup()
      await user.click(screen.getByRole('button', { name: '重新整理別名' }))

      expect(
        await screen.findByText('iCloud 工作階段失效，請更新 Cookie'),
      ).toBeInTheDocument()
      // 關鍵:仍然停留在受保護頁面,沒有被登出
      expect(screen.getByTestId('protected')).toBeInTheDocument()
    },
  )

  it('業務接口回傳 AUTH_REQUIRED 時確實登出', async () => {
    server.use(
      http.get('/api/auth/session', () =>
        HttpResponse.json({
          success: true,
          data: {
            csrf_token: 'csrf-abc',
            expires_at: '2026-08-05T22:00:00+08:00',
          },
        }),
      ),
      http.get('/api/aliases', () =>
        HttpResponse.json(
          { success: false, code: 'AUTH_REQUIRED', message: '工作階段已失效，請重新登入' },
          { status: 401 },
        ),
      ),
    )
    renderBusinessApp()
    await screen.findByTestId('protected')

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: '重新整理別名' }))

    expect(
      await screen.findByRole('heading', { name: APP_TITLE }),
    ).toBeInTheDocument()
  })
})

/** 繪製一個可以主動發起業務請求的受保護頁面 */
function renderBusinessApp() {
  return render(
    <MemoryRouter initialEntries={['/accounts']}>
      <AuthProvider>
        <Routes>
          <Route path="/login" element={<TestApp />} />
          <Route path="/accounts" element={<BusinessCallProbe />} />
        </Routes>
      </AuthProvider>
    </MemoryRouter>,
  )
}

function BusinessCallProbe() {
  const { status } = useAuth()
  const [error, setError] = useState('')

  if (status === 'checking') return <p>loading…</p>
  if (status === 'anonymous') return <LoginPage />

  return (
    <div>
      <p data-testid="protected">已登入頁面</p>
      <button
        onClick={() => {
          void request('/api/aliases?account_id=acc_1').catch((err: unknown) => {
            setError(err instanceof ApiError ? err.message : '網路連線失敗，請檢查服務狀態')
          })
        }}
      >
        重新整理別名
      </button>
      {error && <p>{error}</p>}
    </div>
  )
}

function LogoutProbe() {
  const { status, logout } = useAuth()
  if (status === 'checking') return <p>loading…</p>
  if (status === 'anonymous') return <LoginPage />
  return (
    <div>
      <p data-testid="protected">已登入頁面</p>
      <button onClick={() => void logout()}>登出</button>
    </div>
  )
}
