import { http, HttpResponse } from 'msw'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AliasesPage from './AliasesPage'
import { server } from '../test/server'
import { setCSRFToken } from '../api/client'
import { ToastProvider } from '../components/ToastProvider'
import type { AccountSummary, Alias } from '../api/types'

const accounts: AccountSummary[] = [
  {
    id: 'acc_1',
    name: '主號',
    real_email: 'a@example.com',
    icloud_email: 'a@icloud.com',
    host: 'icloud.com',
    status: 'active',
    alias_total: 2,
    alias_active: 1,
    has_cookies: true,
    has_app_password: false,
    has_proxy: false,
    last_validated: '2026-08-04T09:00:00+08:00',
    created_at: '2026-08-01T09:00:00+08:00',
  },
  {
    id: 'acc_2',
    name: '備用號',
    real_email: 'b@example.com',
    icloud_email: 'b@icloud.com',
    host: 'icloud.com',
    status: 'active',
    alias_total: 1,
    alias_active: 1,
    has_cookies: true,
    has_app_password: false,
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
  {
    email: 'beta@icloud.com',
    anonymousId: 'anon_beta',
    label: 'Beta',
    active: false,
    createdAt: '2026-07-02T00:00:00+08:00',
  },
]

const accountsHandler = () =>
  http.get('/api/accounts', () => HttpResponse.json({ success: true, data: accounts }))

/** 別名列表只回傳指定的資料。 */
const aliasesHandler = (list: Alias[], accountId = 'acc_1') =>
  http.get('/api/aliases', () =>
    HttpResponse.json({
      success: true,
      data: { account_id: accountId, count: list.length, aliases: list },
    }),
  )

const table = () => screen.getByRole('table')
/** 表格內的信箱複製按鈕（排除「刪除別名 <email>」icon 按鈕）。 */
const emailButtons = () =>
  within(table()).getAllByRole('button', { name: /^\S+@icloud\.com$/ })

function renderPage(initialPath = '/aliases') {
  return render(
    <MemoryRouter initialEntries={[initialPath]}>
      <Routes>
        <Route
          path="/aliases"
          element={
            <ToastProvider>
              <AliasesPage />
            </ToastProvider>
          }
        />
      </Routes>
    </MemoryRouter>,
  )
}

describe('AliasesPage', () => {
  beforeEach(() => {
    setCSRFToken('csrf-test')
    server.resetHandlers()
  })

  it('無帳號時顯示引導', async () => {
    server.use(
      http.get('/api/accounts', () => HttpResponse.json({ success: true, data: [] })),
    )
    renderPage()
    expect(await screen.findByText(/還沒有任何帳號/)).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: '別名管理' })).toBeInTheDocument()
  })

  it('帳號切換:URL query 優先,無效時回退到第一個帳號', async () => {
    server.use(
      accountsHandler(),
      http.get('/api/aliases', ({ request }) => {
        const id = new URL(request.url).searchParams.get('account_id')
        return HttpResponse.json({
          success: true,
          data: {
            account_id: id,
            count: id === 'acc_2' ? 1 : 2,
            aliases: id === 'acc_2' ? [aliases[0]] : aliases,
          },
        })
      }),
    )
    const { unmount } = renderPage('/aliases?account_id=acc_2')
    expect(await screen.findByText('alpha@icloud.com')).toBeInTheDocument()
    expect(screen.queryByText('beta@icloud.com')).toBeNull()
    unmount()
    renderPage('/aliases?account_id=bad_id')
    // 回退到第一個帳號 acc_1,顯示 2 個別名
    expect(await screen.findByText('beta@icloud.com')).toBeInTheDocument()
  })

  it('loading/empty/error/retry 狀態', async () => {
    server.use(
      accountsHandler(),
      http.get('/api/aliases', () =>
        HttpResponse.json(
          { success: false, code: 'UPSTREAM_FAILURE', message: '取得別名列表失敗' },
          { status: 502 },
        ),
      ),
    )
    renderPage()
    const retry = await screen.findByRole('button', { name: '重試' })
    expect(screen.getByRole('alert')).toHaveTextContent('取得別名列表失敗')

    server.use(accountsHandler(), aliasesHandler([]))
    await userEvent.click(retry)
    expect(await screen.findByText(/還沒有任何別名/)).toBeInTheDocument()
  })

  it('依 email/label 大小寫不敏感搜尋與啟用狀態篩選', async () => {
    server.use(accountsHandler(), aliasesHandler(aliases))
    renderPage()
    expect(await screen.findByText('alpha@icloud.com')).toBeInTheDocument()

    const user = userEvent.setup()
    await user.type(screen.getByLabelText(/搜尋別名/), 'ALPHA')
    expect(screen.getByText('alpha@icloud.com')).toBeInTheDocument()
    expect(screen.queryByText('beta@icloud.com')).toBeNull()

    await user.clear(screen.getByLabelText(/搜尋別名/))
    await user.selectOptions(screen.getByLabelText('狀態'), 'active')
    expect(screen.getByText('alpha@icloud.com')).toBeInTheDocument()
    expect(screen.queryByText('beta@icloud.com')).toBeNull()

    await user.selectOptions(screen.getByLabelText('狀態'), 'inactive')
    expect(screen.getByText('beta@icloud.com')).toBeInTheDocument()
    expect(screen.queryByText('alpha@icloud.com')).toBeNull()
  })

  it('建立時間預設倒序且可切換正序,相容時間戳並以收件匣格式顯示', async () => {
    const timestampAliases: Alias[] = [
      { ...aliases[0], createdAt: '1787406420000' },
      { ...aliases[1], createdAt: '1787406360000' },
    ]
    server.use(accountsHandler(), aliasesHandler(timestampAliases))
    renderPage()
    await screen.findByText('alpha@icloud.com')

    expect(emailButtons().map((button) => button.textContent)).toEqual([
      'alpha@icloud.com',
      'beta@icloud.com',
    ])
    expect(screen.getAllByText(/^\d{4}\/\d{2}\/\d{2} \d{2}:\d{2}$/)).toHaveLength(2)

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /建立時間排序/ }))
    expect(emailButtons().map((button) => button.textContent)).toEqual([
      'beta@icloud.com',
      'alpha@icloud.com',
    ])

    const alphaRow = within(table()).getByRole('row', { name: /alpha@icloud\.com/ })
    expect(within(alphaRow).getByRole('link', { name: /收件匣/ })).toHaveAttribute(
      'href',
      '/inbox?account_id=acc_1&alias=alpha%40icloud.com',
    )
  })

  it('建立別名:空標籤被阻擋、200 字元邊界、成功後刷新並可複製信箱', async () => {
    let created = false
    server.use(
      accountsHandler(),
      http.get('/api/aliases', () =>
        HttpResponse.json({
          success: true,
          data: {
            account_id: 'acc_1',
            count: created ? 3 : 2,
            aliases: created
              ? [
                  ...aliases,
                  {
                    email: 'gamma@icloud.com',
                    anonymousId: 'anon_gamma',
                    label: 'Gamma',
                    active: true,
                  },
                ]
              : aliases,
          },
        }),
      ),
      http.post('/api/create', async ({ request }) => {
        const body = (await request.json()) as { account_id: string; label: string }
        created = true
        return HttpResponse.json({
          success: true,
          data: {
            email: 'gamma@icloud.com',
            label: body.label,
            created_at: '2026-08-05T09:00:00+08:00',
            account_id: body.account_id,
          },
        })
      }),
    )
    renderPage()
    await screen.findByText('alpha@icloud.com')
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: '建立別名' }))

    const dialog = screen.getByRole('dialog', { name: '建立別名' })
    const submit = within(dialog).getByRole('button', { name: '建立別名' })
    // 空標籤提交被阻止
    await user.click(submit)
    expect(await within(dialog).findByRole('alert')).toHaveTextContent('請輸入標籤')
    expect(created).toBe(false)

    // 200 字元邊界:輸入 201 字元被截斷到 200
    fireEvent.change(within(dialog).getByLabelText('標籤'), { target: { value: 'x'.repeat(201) } })
    expect((within(dialog).getByLabelText('標籤') as HTMLInputElement).value.length).toBe(200)

    await user.click(submit)
    // 成功後列表刷新,並以可複製的提示顯示新信箱
    expect(
      await within(table()).findByRole('button', { name: 'gamma@icloud.com' }),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('status', { name: '別名已建立：gamma@icloud.com' }),
    ).toBeInTheDocument()
  })

  it('停用別名:顯示目標信箱並二次確認', async () => {
    server.use(
      accountsHandler(),
      aliasesHandler(aliases),
      http.post('/api/aliases/:id/deactivate', () =>
        HttpResponse.json({ success: true, data: { anonymous_id: 'anon_alpha', success: true } }),
      ),
    )
    renderPage()
    await screen.findByText('alpha@icloud.com')
    const user = userEvent.setup()
    await user.click(screen.getAllByRole('button', { name: '停用' })[0])

    const dialog = screen.getByRole('dialog', { name: '停用別名' })
    expect(dialog).toHaveTextContent('alpha@icloud.com')
    await user.click(within(dialog).getByRole('button', { name: '確認停用' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  })

  it('啟用別名:顯示目標信箱並二次確認', async () => {
    server.use(
      accountsHandler(),
      aliasesHandler(aliases),
      http.post('/api/aliases/:id/reactivate', () =>
        HttpResponse.json({ success: true, data: { anonymous_id: 'anon_beta', success: true } }),
      ),
    )
    renderPage()
    await screen.findByText('beta@icloud.com')
    const user = userEvent.setup()
    await user.click(screen.getAllByRole('button', { name: '啟用' })[0])

    const dialog = screen.getByRole('dialog', { name: '啟用別名' })
    expect(dialog).toHaveTextContent('beta@icloud.com')
    await user.click(within(dialog).getByRole('button', { name: '確認啟用' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  })

  it('刪除別名:要求輸入完整信箱,用 anonymousId 構造 URL 並編碼', async () => {
    let deletedUrl = ''
    server.use(
      accountsHandler(),
      aliasesHandler(aliases),
      http.delete('/api/aliases/:id', ({ request }) => {
        deletedUrl = request.url
        return HttpResponse.json({ success: true, data: { anonymous_id: 'anon_alpha' } })
      }),
    )
    renderPage()
    await screen.findByText('alpha@icloud.com')
    const user = userEvent.setup()

    const alphaRow = within(table()).getByRole('row', { name: /alpha@icloud\.com/ })
    await user.click(within(alphaRow).getByRole('button', { name: '刪除別名 alpha@icloud.com' }))

    const dialog = screen.getByRole('dialog', { name: '刪除別名' })
    expect(dialog).toHaveTextContent('alpha@icloud.com')
    const confirm = within(dialog).getByRole('button', { name: '確認刪除' })
    const confirmInput = within(dialog).getByLabelText(/輸入完整信箱以確認/)

    // 輸入不完整信箱時按鈕禁用
    fireEvent.change(confirmInput, { target: { value: 'alpha@icloud' } })
    expect(confirm).toBeDisabled()
    fireEvent.change(confirmInput, { target: { value: 'alpha@icloud.com' } })
    await user.click(confirm)

    await waitFor(() => expect(deletedUrl).toContain('anon_alpha'))
    expect(deletedUrl).not.toContain('alpha%40icloud.com')
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  })

  it('點擊信箱可複製並顯示提示;複製失敗時提供手動複製', async () => {
    server.use(accountsHandler(), aliasesHandler(aliases))
    renderPage()
    await screen.findByText('alpha@icloud.com')
    // userEvent.setup() 會安裝自己的剪貼簿替身,必須在它之後再覆寫
    const user = userEvent.setup()
    const descriptor = Object.getOwnPropertyDescriptor(navigator, 'clipboard')
    const writeText = vi.fn()
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText },
    })

    try {
      // 成功複製
      writeText.mockResolvedValueOnce(undefined)
      await user.click(within(table()).getByRole('button', { name: 'alpha@icloud.com' }))
      await waitFor(() => expect(writeText).toHaveBeenCalledWith('alpha@icloud.com'))
      expect(await screen.findByText('信箱已複製')).toBeInTheDocument()

      // 複製被拒:顯示可手動複製的提示
      writeText.mockRejectedValueOnce(new Error('Permission denied'))
      await user.click(within(table()).getByRole('button', { name: 'beta@icloud.com' }))
      expect(
        await screen.findByRole('status', { name: '複製失敗，請手動複製：beta@icloud.com' }),
      ).toBeInTheDocument()
    } finally {
      if (descriptor) {
        Object.defineProperty(navigator, 'clipboard', descriptor)
      } else {
        Reflect.deleteProperty(navigator, 'clipboard')
      }
    }
  })

  it('操作失敗保留列表並顯示錯誤', async () => {
    server.use(
      accountsHandler(),
      aliasesHandler(aliases),
      http.post('/api/aliases/:id/deactivate', () =>
        HttpResponse.json(
          { success: false, code: 'UPSTREAM_FAILURE', message: '停用失敗' },
          { status: 502 },
        ),
      ),
    )
    renderPage()
    await screen.findByText('alpha@icloud.com')
    const user = userEvent.setup()
    await user.click(screen.getAllByRole('button', { name: '停用' })[0])
    await user.click(
      within(screen.getByRole('dialog', { name: '停用別名' })).getByRole('button', {
        name: '確認停用',
      }),
    )

    expect(await screen.findByRole('alert')).toHaveTextContent('停用失敗')
    expect(within(table()).getByRole('button', { name: 'alpha@icloud.com' })).toBeInTheDocument()
  })
})
