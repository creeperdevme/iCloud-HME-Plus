import { http, HttpResponse } from 'msw'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it } from 'vitest'
import InboxPage from './InboxPage'
import { server } from '../test/server'
import { setCSRFToken } from '../api/client'
import { ToastProvider } from '../components/ToastProvider'
import type { AccountSummary, Alias, InboxResult } from '../api/types'

const accounts: AccountSummary[] = [
  {
    id: 'acc_1',
    name: '主號',
    real_email: 'a@example.com',
    icloud_email: 'a@icloud.com',
    host: 'icloud.com',
    status: 'active',
    alias_total: 2,
    alias_active: 2,
    has_cookies: true,
    has_app_password: true,
    has_proxy: false,
    last_validated: '2026-08-04T09:00:00+08:00',
    created_at: '2026-08-01T09:00:00+08:00',
  },
]

const aliases: Alias[] = [
  {
    email: 'alpha@icloud.com',
    anonymousId: 'anon_alpha',
    label: 'Alpha',
    active: true,
    createdAt: '2026-07-01T00:00:00+08:00',
  },
]

const inboxResult: InboxResult = {
  account_id: 'acc_1',
  alias: 'alpha@icloud.com',
  count: 1,
  method: 'imap',
  messages: [
    {
      id: '1',
      from: 'sender@example.com',
      to: 'alpha@icloud.com',
      subject: '主題一',
      date: '2026-08-04T10:00:00+08:00',
      preview: '預覽內容',
    },
  ],
}

const accountsHandler = () =>
  http.get('/api/accounts', () => HttpResponse.json({ success: true, data: accounts }))

const aliasesHandler = () =>
  http.get('/api/aliases', () =>
    HttpResponse.json({
      success: true,
      data: { account_id: 'acc_1', count: aliases.length, aliases },
    }),
  )

function renderPage(initialPath = '/inbox') {
  return render(
    <MemoryRouter initialEntries={[initialPath]}>
      <Routes>
        <Route
          path="/inbox"
          element={
            <ToastProvider>
              <InboxPage />
            </ToastProvider>
          }
        />
      </Routes>
    </MemoryRouter>,
  )
}

describe('InboxPage', () => {
  beforeEach(() => {
    setCSRFToken('csrf-test')
    server.resetHandlers()
  })

  it('帳號必選;別名可空;limit/days 生效;query 經 URLSearchParams', async () => {
    let lastUrl = ''
    server.use(
      accountsHandler(),
      aliasesHandler(),
      http.get('/api/inbox', ({ request }) => {
        lastUrl = request.url
        return HttpResponse.json({ success: true, data: inboxResult })
      }),
    )
    renderPage()
    await screen.findByText('主題一')

    // 初次載入的 query 參數
    const url = new URL(lastUrl)
    expect(url.searchParams.get('account_id')).toBe('acc_1')
    expect(url.searchParams.get('limit')).toBe('20')
    expect(url.searchParams.get('days')).toBe('7')

    // 修改筆數上限與最近天數後重新查詢
    const user = userEvent.setup()
    fireEvent.change(screen.getByLabelText('筆數上限'), { target: { value: '100' } })
    fireEvent.change(screen.getByLabelText('最近天數'), { target: { value: '30' } })
    await user.click(screen.getByRole('button', { name: '套用條件' }))

    await waitFor(() => {
      const updated = new URL(lastUrl)
      expect(updated.searchParams.get('limit')).toBe('100')
      expect(updated.searchParams.get('days')).toBe('30')
    })
  })

  it('從 URL 的 alias 參數初始化篩選,支援別名頁直達收件匣', async () => {
    const inboxUrls: string[] = []
    server.use(
      accountsHandler(),
      aliasesHandler(),
      http.get('/api/inbox', ({ request }) => {
        inboxUrls.push(request.url)
        return HttpResponse.json({ success: true, data: inboxResult })
      }),
    )
    renderPage('/inbox?account_id=acc_1&alias=alpha%40icloud.com')
    await screen.findByText('主題一')

    await waitFor(() => {
      expect(
        inboxUrls.some(
          (url) => new URL(url).searchParams.get('alias') === 'alpha@icloud.com',
        ),
      ).toBe(true)
    })
    expect(screen.getByLabelText('別名')).toHaveValue('alpha@icloud.com')
  })

  it('顯示目前使用的讀取方式(method=imap 或 web_api)', async () => {
    server.use(
      accountsHandler(),
      aliasesHandler(),
      http.get('/api/inbox', () =>
        HttpResponse.json({
          success: true,
          data: { ...inboxResult, method: 'web_api' },
        }),
      ),
    )
    renderPage()
    await screen.findByText('主題一')
    expect(screen.getByText(/目前透過 Web API 讀取/)).toBeInTheDocument()
  })

  it('載入中顯示骨架且查詢按鈕停用', () => {
    server.use(
      accountsHandler(),
      aliasesHandler(),
      http.get('/api/inbox', () => new Promise<Response>(() => {})),
    )
    renderPage()

    // 首次渲染即為載入中,因此同步斷言(不需等待)
    expect(screen.getByText('載入中')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '套用條件' })).toBeDisabled()
    expect(screen.queryByText(/這段期間沒有收到郵件/)).toBeNull()
  })

  it('空列表顯示空狀態且按鈕可用', async () => {
    server.use(
      accountsHandler(),
      aliasesHandler(),
      http.get('/api/inbox', () =>
        HttpResponse.json({
          success: true,
          data: { account_id: 'acc_1', count: 0, messages: [], method: 'imap' },
        }),
      ),
    )
    renderPage()

    expect(await screen.findByText(/這段期間沒有收到郵件/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '套用條件' })).toBeEnabled()
  })

  it('查詢失敗顯示錯誤訊息並可重試', async () => {
    let calls = 0
    server.use(
      accountsHandler(),
      aliasesHandler(),
      http.get('/api/inbox', () => {
        calls++
        if (calls === 1) {
          return HttpResponse.json(
            { success: false, code: 'UPSTREAM_FAILURE', message: '讀取收件匣失敗' },
            { status: 502 },
          )
        }
        return HttpResponse.json({ success: true, data: inboxResult })
      }),
    )
    renderPage()

    expect(await screen.findByRole('alert')).toHaveTextContent('讀取收件匣失敗')
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: '重試' }))
    expect(await screen.findByText('主題一')).toBeInTheDocument()
  })

  it('工作階段失效(401)時顯示伺服器訊息且不顯示郵件', async () => {
    server.use(
      accountsHandler(),
      aliasesHandler(),
      http.get('/api/inbox', () =>
        HttpResponse.json(
          { success: false, code: 'AUTH_REQUIRED', message: '請先登入' },
          { status: 401 },
        ),
      ),
    )
    renderPage()

    expect(await screen.findByRole('alert')).toHaveTextContent('請先登入')
    expect(screen.queryByText('主題一')).toBeNull()
    expect(screen.queryByRole('table')).toBeNull()
  })

  it('惡意 HTML 只作為文字顯示,不產生 img/script 節點', async () => {
    const evil = {
      ...inboxResult,
      messages: [
        {
          id: '2',
          from: 'evil@example.com',
          to: 'alpha@icloud.com',
          subject: '<img src=x onerror=alert(1)>',
          date: '2026-08-04T10:00:00+08:00',
          preview: '<script>alert(2)</script>預覽',
        },
      ],
    }
    server.use(
      accountsHandler(),
      aliasesHandler(),
      http.get('/api/inbox', () => HttpResponse.json({ success: true, data: evil })),
    )
    renderPage()
    await screen.findByText(/<img src=x onerror=alert\(1\)>/)
    expect(document.querySelector('img')).toBeNull()
    expect(document.querySelector('script')).toBeNull()
  })

  it('快速切換篩選:第一次請求晚返回不覆蓋第二次請求', async () => {
    let release: (() => void) | undefined
    let calls = 0
    server.use(
      accountsHandler(),
      aliasesHandler(),
      http.get('/api/inbox', () => {
        calls++
        if (calls === 1) {
          // 第一次請求掛起,稍後才回傳舊資料
          return new Promise<Response>((resolve) => {
            release = () =>
              resolve(
                HttpResponse.json({
                  success: true,
                  data: {
                    ...inboxResult,
                    messages: [{ ...inboxResult.messages[0], subject: '舊主題' }],
                  },
                }),
              )
          })
        }
        return HttpResponse.json({
          success: true,
          data: {
            ...inboxResult,
            alias: 'second',
            messages: [
              {
                id: '9',
                from: 's2@example.com',
                to: 'alpha@icloud.com',
                subject: '第二請求主題',
                date: '2026-08-04T11:00:00+08:00',
                preview: '第二請求',
              },
            ],
          },
        })
      }),
    )
    renderPage()
    await screen.findByText('載入中')

    // 等第一次請求真的送出後再切換條件(第二次請求會中止第一次)
    await waitFor(() => expect(calls).toBe(1))
    fireEvent.change(screen.getByLabelText('筆數上限'), { target: { value: '100' } })
    expect(await screen.findByText('第二請求主題')).toBeInTheDocument()

    // 第一次請求此時才返回,舊資料不得覆蓋新資料
    release?.()
    await new Promise((resolve) => setTimeout(resolve, 100))
    expect(screen.getByText('第二請求主題')).toBeInTheDocument()
    expect(screen.queryByText('舊主題')).toBeNull()
  })

  it('空主旨顯示(無主旨)', async () => {
    server.use(
      accountsHandler(),
      aliasesHandler(),
      http.get('/api/inbox', () =>
        HttpResponse.json({
          success: true,
          data: {
            ...inboxResult,
            messages: [{ ...inboxResult.messages[0], subject: '' }],
          },
        }),
      ),
    )
    renderPage()
    expect(await screen.findByText('(無主旨)')).toBeInTheDocument()
  })

  it('缺少寄件人/收件人顯示破折號佔位', async () => {
    server.use(
      accountsHandler(),
      aliasesHandler(),
      http.get('/api/inbox', () =>
        HttpResponse.json({
          success: true,
          data: {
            ...inboxResult,
            messages: [{ ...inboxResult.messages[0], from: '', to: '', preview: '' }],
          },
        }),
      ),
    )
    renderPage()
    await screen.findByText('主題一')

    // 寄件人與收件人各自的佔位符
    expect(screen.getAllByText('—')).toHaveLength(2)
    // 新版不為空摘要產生節點
    expect(document.querySelector('.cell-secondary')).toBeNull()
    expect(within(screen.getByRole('table')).queryByText('預覽內容')).toBeNull()
  })

  it('顯示郵件總數', async () => {
    server.use(
      accountsHandler(),
      aliasesHandler(),
      http.get('/api/inbox', () => HttpResponse.json({ success: true, data: inboxResult })),
    )
    renderPage()
    await screen.findByText('主題一')
    expect(screen.getByText(/共 1 封郵件/)).toBeInTheDocument()
  })
})
