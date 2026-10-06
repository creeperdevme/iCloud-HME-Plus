import { http, HttpResponse } from 'msw'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AccountsPage from './AccountsPage'
import { server } from '../test/server'
import { setCSRFToken } from '../api/client'
import { ToastProvider } from '../components/ToastProvider'
import CookieDialog from '../components/CookieDialog'
import type { AccountSummary } from '../api/types'

const accounts: AccountSummary[] = [
  {
    id: 'acc_active',
    name: '活躍號',
    real_email: 'active@example.com',
    icloud_email: 'active@icloud.com',
    host: 'icloud.com',
    status: 'active',
    alias_total: 15,
    alias_active: 12,
    has_cookies: true,
    has_app_password: true,
    has_proxy: false,
    last_validated: '2026-08-04T09:00:00+08:00',
    created_at: '2026-08-01T09:00:00+08:00',
  },
  {
    id: 'acc_pending',
    name: '等待號',
    real_email: 'pending@example.com',
    icloud_email: 'pending@icloud.com',
    host: 'icloud.com',
    status: 'pending',
    alias_total: 0,
    alias_active: 0,
    has_cookies: false,
    has_app_password: false,
    has_proxy: false,
    last_validated: '',
    created_at: '2026-08-02T09:00:00+08:00',
  },
  {
    id: 'acc_error',
    name: '錯誤號',
    real_email: 'error@example.com',
    icloud_email: 'error@icloud.com',
    host: 'icloud.com.cn',
    status: 'error',
    alias_total: 0,
    alias_active: 0,
    has_cookies: true,
    has_app_password: false,
    has_proxy: true,
    mailbox: {
      provider: 'imap',
      email: 'box@example.com',
      imap_host: 'imap.example.com',
      imap_port: 993,
    },
    last_validated: '',
    created_at: '2026-08-03T09:00:00+08:00',
  },
]

/** 帳號列表的 GET；預設回傳上面的固定清單，可傳入自訂清單覆寫。 */
const accountsHandler = (list: AccountSummary[] = accounts) =>
  http.get('/api/accounts', () => HttpResponse.json({ success: true, data: list }))

/** 模擬「Get cookies.txt LOCALLY」匯出的內容（含註解與 #HttpOnly_ 行）。 */
const netscapeBlob = [
  '# Netscape HTTP Cookie File',
  '# 這是註解，解析時必須忽略',
  '#HttpOnly_.icloud.com\tTRUE\t/\tTRUE\t1893456000\tX-APPLE-WEBAUTH-TOKEN\ttoken-value',
  '.icloud.com\tTRUE\t/\tTRUE\t1893456000\tX-APPLE-WEBAUTH-USER\tuser-value',
].join('\n')

/** 四個關鍵 Cookie 欄位（逐項填寫模式）。 */
function cookieInputs(scope: HTMLElement) {
  return scope.querySelectorAll<HTMLInputElement>('input[id^="cookie-X-APPLE-"]')
}

function renderPage() {
  return render(
    <MemoryRouter>
      <ToastProvider>
        <AccountsPage />
      </ToastProvider>
    </MemoryRouter>,
  )
}

/** 開啟「新增 iCloud 帳號」對話框並回傳該對話框。 */
async function openCreateDialog() {
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: '新增帳號' }))
  return { user, dialog: screen.getByRole('dialog', { name: '新增 iCloud 帳號' }) }
}

describe('AccountsPage', () => {
  beforeEach(() => {
    setCSRFToken('csrf-test')
    server.resetHandlers()
  })

  it('渲染帳號列表:狀態文字、憑證標誌、信箱與別名計數可見,無秘密', async () => {
    server.use(accountsHandler())
    renderPage()

    expect(await screen.findByRole('heading', { name: '帳號管理' })).toBeInTheDocument()
    expect(screen.getByText('活躍號')).toBeInTheDocument()
    expect(screen.getByText('等待號')).toBeInTheDocument()
    expect(screen.getByText('錯誤號')).toBeInTheDocument()
    expect(screen.getByText('active@icloud.com')).toBeInTheDocument()
    expect(screen.getByText('12 / 15')).toBeInTheDocument()

    // 狀態徽章
    expect(screen.getByText('正常')).toBeInTheDocument()
    expect(screen.getByText('待設定')).toBeInTheDocument()
    expect(screen.getByText('異常')).toBeInTheDocument()

    // 憑證標誌:有 Cookie / App 密碼 / 代理 / 收件信箱,沒有的顯示「尚未設定」
    expect(screen.getAllByText('Cookie').length).toBeGreaterThan(0)
    expect(screen.getAllByText('App 密碼').length).toBeGreaterThan(0)
    expect(screen.getAllByText('代理').length).toBeGreaterThan(0)
    expect(screen.getByText('收件匣 box@example.com')).toBeInTheDocument()
    expect(screen.getByText('尚未設定')).toBeInTheDocument()

    // 統計卡片
    expect(screen.getByText('帳號總數')).toBeInTheDocument()
    expect(screen.getByText('狀態正常')).toBeInTheDocument()
    expect(screen.getByText('待設定 / 異常')).toBeInTheDocument()
    expect(screen.getByText('別名總數')).toBeInTheDocument()

    // 秘密欄位不可見
    expect(screen.queryByText(/cookie-secret|app-secret|proxy-secret/)).toBeNull()
  })

  it('新增帳號:名稱可留空、送出期間禁用、成功後重新整理列表', async () => {
    const posts: Array<Record<string, unknown>> = []
    let listData = accounts
    let getCalls = 0
    let releasePost: (() => void) | undefined

    server.use(
      http.get('/api/accounts', () => {
        getCalls++
        return HttpResponse.json({ success: true, data: listData })
      }),
      http.post('/api/accounts', async ({ request }) => {
        posts.push((await request.json()) as Record<string, unknown>)
        await new Promise<void>((resolve) => {
          releasePost = resolve
        })
        listData = [
          { ...accounts[0], id: 'acc_new', name: 'owner', icloud_email: 'owner@icloud.com' },
          ...accounts,
        ]
        return HttpResponse.json({ success: true, data: listData[0] }, { status: 201 })
      }),
    )

    renderPage()
    await screen.findByText('活躍號')
    const { user, dialog } = await openCreateDialog()

    // 只填 Prefix:名稱留空是允許的
    fireEvent.change(within(dialog).getByLabelText('iCloud 信箱 Prefix'), {
      target: { value: 'owner' },
    })
    await user.click(within(dialog).getByRole('button', { name: '新增帳號' }))

    await waitFor(() => expect(posts).toHaveLength(1))
    expect(posts[0].name).toBe('')
    // 只送 Prefix,不是完整信箱;後綴由 host 決定
    expect(posts[0].icloud_email).toBe('owner')
    expect(posts[0].host).toBe('icloud.com')
    expect(posts[0].proxy).toBe('')
    expect(posts[0].cookies).toBe('')
    // App 專用密碼欄位未填時送空字串,後端據此不設定密碼
    expect(posts[0].app_password).toBe('')

    // 請求期間按鈕禁用
    await waitFor(() =>
      expect(within(dialog).getByRole('button', { name: '儲存中…' })).toBeDisabled(),
    )

    releasePost?.()
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    // 成功後重新載入列表
    expect(await screen.findByText('owner@icloud.com')).toBeInTheDocument()
    expect(getCalls).toBeGreaterThanOrEqual(2)
  })

  it('新增帳號:對話框內可直接填 App 專用密碼並一起送出', async () => {
    const posts: Array<Record<string, unknown>> = []
    server.use(
      accountsHandler(),
      http.post('/api/accounts', async ({ request }) => {
        posts.push((await request.json()) as Record<string, unknown>)
        return HttpResponse.json(
          { success: true, data: { ...accounts[0], id: 'acc_new', has_app_password: true } },
          { status: 201 },
        )
      }),
    )

    renderPage()
    await screen.findByText('活躍號')
    const { user, dialog } = await openCreateDialog()

    // 欄位就在新增帳號對話框底下,不必先建帳號再另外設定
    fireEvent.change(within(dialog).getByLabelText('iCloud 信箱 Prefix'), {
      target: { value: 'owner' },
    })
    fireEvent.change(within(dialog).getByLabelText(/App 專用密碼/), {
      target: { value: 'abcd-efgh-ijkl-mnop' },
    })
    await user.click(within(dialog).getByRole('button', { name: '新增帳號' }))

    await waitFor(() => expect(posts).toHaveLength(1))
    expect(posts[0].app_password).toBe('abcd-efgh-ijkl-mnop')
    // 信箱只送 Prefix,不必填完整位址
    expect(posts[0].icloud_email).toBe('owner')

    // 密碼有存進去時只顯示一般的成功訊息
    const toast = await screen.findByRole('status')
    expect(toast).toHaveTextContent('帳號已儲存')
    expect(toast.textContent).not.toContain('未通過')
  })

  it('新增帳號:App 專用密碼沒通過驗證時明白告知未設定', async () => {
    server.use(
      accountsHandler(),
      http.post('/api/accounts', () =>
        HttpResponse.json(
          {
            success: true,
            data: { ...accounts[0], id: 'acc_new', has_app_password: false },
            warning: '帳號已建立，但 App 專用密碼未通過 IMAP 驗證，因此尚未設定；請確認密碼後用「App 密碼」重新設定。',
          },
          { status: 201 },
        ),
      ),
    )

    renderPage()
    await screen.findByText('活躍號')
    const { user, dialog } = await openCreateDialog()

    fireEvent.change(within(dialog).getByLabelText('iCloud 信箱 Prefix'), {
      target: { value: 'owner' },
    })
    fireEvent.change(within(dialog).getByLabelText(/App 專用密碼/), {
      target: { value: 'wrong-password' },
    })
    await user.click(within(dialog).getByRole('button', { name: '新增帳號' }))

    // 帳號有建立,但 Toast 必須說清楚密碼沒設定,不能只說「已儲存」
    const toast = await screen.findByRole('status')
    expect(toast).toHaveTextContent('未通過 IMAP 驗證')
    expect(toast.textContent).not.toBe('帳號已儲存')
    // 對話框仍然關閉(帳號確實建好了)
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  })

  it('新增帳號:Prefix 空白時前端阻擋且不發送請求', async () => {
    let posted = false
    server.use(
      accountsHandler(),
      http.post('/api/accounts', () => {
        posted = true
        return HttpResponse.json({ success: true, data: accounts[0] }, { status: 201 })
      }),
    )

    renderPage()
    await screen.findByText('活躍號')
    const { user, dialog } = await openCreateDialog()

    await user.click(within(dialog).getByRole('button', { name: '新增帳號' }))

    expect(await within(dialog).findByRole('alert')).toHaveTextContent(
      '請輸入 iCloud 信箱的 Prefix',
    )
    expect(posted).toBe(false)
    expect(screen.getByRole('dialog')).toBeInTheDocument()
  })

  it('新增帳號:區域決定後綴,貼上完整信箱只保留 Prefix', async () => {
    server.use(accountsHandler())
    renderPage()
    await screen.findByText('活躍號')
    const { user, dialog } = await openCreateDialog()

    const prefix = within(dialog).getByLabelText('iCloud 信箱 Prefix') as HTMLInputElement
    const hostSelect = within(dialog).getByLabelText('區域')

    // host 只能選全球區或中國區
    expect(within(hostSelect).getByText('全球區（@icloud.com）')).toBeInTheDocument()
    expect(within(hostSelect).getByText('中國區（@icloud.com.cn）')).toBeInTheDocument()
    expect(within(dialog).getByText('@icloud.com')).toBeInTheDocument()

    fireEvent.change(prefix, { target: { value: 'owner' } })
    expect(within(dialog).getByText(/完整信箱為 owner@icloud\.com/)).toBeInTheDocument()

    // 貼上完整信箱:自動去掉 @ 之後的部分
    fireEvent.change(prefix, { target: { value: 'owner@icloud.com' } })
    expect(prefix.value).toBe('owner')

    // 切換區域會改變顯示的後綴
    await user.selectOptions(hostSelect, 'icloud.com.cn')
    expect(within(dialog).getByText('@icloud.com.cn')).toBeInTheDocument()
    expect(within(dialog).getByText(/完整信箱為 owner@icloud\.com\.cn/)).toBeInTheDocument()

    fireEvent.change(prefix, { target: { value: 'owner@icloud.com.cn' } })
    expect(prefix.value).toBe('owner')
  })

  it('新增帳號:逐項填寫 Cookie 只送出非空的關鍵欄位', async () => {
    const posts: Array<Record<string, unknown>> = []
    server.use(
      accountsHandler(),
      http.post('/api/accounts', async ({ request }) => {
        posts.push((await request.json()) as Record<string, unknown>)
        return HttpResponse.json({ success: true, data: accounts[0] }, { status: 201 })
      }),
    )

    renderPage()
    await screen.findByText('活躍號')
    const { user, dialog } = await openCreateDialog()

    // 逐項填寫模式:四個關鍵 Cookie 各一個輸入框
    const inputs = cookieInputs(dialog)
    expect(inputs).toHaveLength(4)
    expect(Array.from(inputs).map((input) => input.id)).toEqual([
      'cookie-X-APPLE-WEBAUTH-TOKEN',
      'cookie-X-APPLE-WEBAUTH-USER',
      'cookie-X-APPLE-WEBAUTH-HSA-TRUST',
      'cookie-X-APPLE-DS-WEB-SESSION-TOKEN',
    ])

    fireEvent.change(within(dialog).getByLabelText('iCloud 信箱 Prefix'), {
      target: { value: 'owner' },
    })
    fireEvent.change(within(dialog).getByLabelText(/X-APPLE-WEBAUTH-TOKEN/), {
      target: { value: '  token-value  ' },
    })
    fireEvent.change(within(dialog).getByLabelText(/X-APPLE-WEBAUTH-HSA-TRUST/), {
      target: { value: 'trust-value' },
    })

    await user.click(within(dialog).getByRole('button', { name: '新增帳號' }))
    await waitFor(() => expect(posts).toHaveLength(1))

    expect(JSON.parse(posts[0].cookies as string)).toEqual({
      'X-APPLE-WEBAUTH-TOKEN': 'token-value',
      'X-APPLE-WEBAUTH-HSA-TRUST': 'trust-value',
    })
  })

  it('新增帳號:可切換「貼上匯入」並自動識別 Netscape/JSON,無法識別時顯示錯誤', async () => {
    server.use(accountsHandler())
    renderPage()
    await screen.findByText('活躍號')
    const { user, dialog } = await openCreateDialog()

    const manualButton = within(dialog).getByRole('button', { name: '逐項填寫' })
    const pasteButton = within(dialog).getByRole('button', { name: '貼上匯入' })
    expect(manualButton).toHaveAttribute('aria-pressed', 'true')
    expect(pasteButton).toHaveAttribute('aria-pressed', 'false')

    await user.click(pasteButton)
    expect(pasteButton).toHaveAttribute('aria-pressed', 'true')
    expect(manualButton).toHaveAttribute('aria-pressed', 'false')

    const textarea = within(dialog).getByLabelText('貼上驗證資訊')
    expect(textarea).toHaveAttribute('id', 'cookie-paste')

    // Netscape cookies.txt:自動填入關鍵欄位
    fireEvent.change(textarea, { target: { value: netscapeBlob } })
    expect(await within(dialog).findByRole('status')).toHaveTextContent(
      'cookies.txt（Netscape）',
    )
    expect(within(dialog).getByLabelText(/X-APPLE-WEBAUTH-TOKEN/)).toHaveValue('token-value')
    expect(within(dialog).getByLabelText(/X-APPLE-WEBAUTH-USER/)).toHaveValue('user-value')
    expect(within(dialog).getByLabelText(/X-APPLE-WEBAUTH-HSA-TRUST/)).toHaveValue('')

    // JSON:同樣會自動識別
    fireEvent.change(textarea, {
      target: { value: '{"X-APPLE-WEBAUTH-HSA-TRUST":"trust-json"}' },
    })
    expect(await within(dialog).findByRole('status')).toHaveTextContent('JSON')
    expect(within(dialog).getByLabelText(/X-APPLE-WEBAUTH-HSA-TRUST/)).toHaveValue('trust-json')

    // 無法識別:顯示錯誤提示且不清空已填欄位
    fireEvent.change(textarea, { target: { value: '完全不是 Cookie 的內容' } })
    expect(await within(dialog).findByRole('alert')).toHaveTextContent('無法識別')
    expect(within(dialog).getByLabelText(/X-APPLE-WEBAUTH-HSA-TRUST/)).toHaveValue('trust-json')
  })

  it('更新 Cookie:貼上匯入後送出、對話框關閉且列表重新整理', async () => {
    let putUrl = ''
    let putBody: Record<string, unknown> | null = null
    server.use(
      accountsHandler(),
      http.put('/api/accounts/:id/cookies', async ({ request }) => {
        putUrl = request.url
        putBody = (await request.json()) as Record<string, unknown>
        return HttpResponse.json({ success: true, data: accounts[0] })
      }),
    )

    renderPage()
    await screen.findByText('活躍號')
    const user = userEvent.setup()
    await user.click(screen.getAllByRole('button', { name: '更新 Cookie' })[0])

    const dialog = screen.getByRole('dialog', { name: '更新 Cookie' })
    await user.click(within(dialog).getByRole('button', { name: '貼上匯入' }))
    const textarea = within(dialog).getByLabelText('貼上驗證資訊')
    fireEvent.change(textarea, { target: { value: 'X-APPLE-WEBAUTH-TOKEN=token-x' } })
    expect(await within(dialog).findByRole('status')).toHaveTextContent('Cookie Header')

    await user.click(within(dialog).getByRole('button', { name: '儲存 Cookie' }))

    await waitFor(() => expect(putBody).not.toBeNull())
    expect(putUrl).toContain('/api/accounts/acc_active/cookies')
    expect(JSON.parse((putBody as unknown as Record<string, string>).cookies)).toEqual({
      'X-APPLE-WEBAUTH-TOKEN': 'token-x',
    })
    // 送出後對話框關閉:textarea 不再留在畫面上
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(screen.queryByLabelText('貼上驗證資訊')).toBeNull()
  })

  it('CookieDialog:未填寫時阻擋送出,提交後清空欄位', async () => {
    const onSaved = vi.fn()
    let puts = 0
    let lastBody: Record<string, unknown> | null = null
    server.use(
      http.put('/api/accounts/:id/cookies', async ({ request }) => {
        puts++
        lastBody = (await request.json()) as Record<string, unknown>
        return HttpResponse.json({ success: true, data: accounts[0] })
      }),
    )

    render(
      <CookieDialog accountId="acc_active" open onClose={() => {}} onSaved={onSaved} />,
    )
    const user = userEvent.setup()

    // 全部留空:只顯示錯誤,不送請求
    await user.click(screen.getByRole('button', { name: '儲存 Cookie' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('請至少填入一個 Cookie')
    expect(puts).toBe(0)

    const tokenInput = screen.getByLabelText(/X-APPLE-WEBAUTH-TOKEN/) as HTMLInputElement
    fireEvent.change(tokenInput, { target: { value: 'token-y' } })
    await user.click(screen.getByRole('button', { name: '儲存 Cookie' }))

    await waitFor(() => expect(onSaved).toHaveBeenCalledTimes(1))
    expect(JSON.parse((lastBody as unknown as Record<string, string>).cookies)).toEqual({
      'X-APPLE-WEBAUTH-TOKEN': 'token-y',
    })
    // 提交後欄位清空
    expect((screen.getByLabelText(/X-APPLE-WEBAUTH-TOKEN/) as HTMLInputElement).value).toBe('')
    // 已清空,再次提交只會回到驗證錯誤,不會送出空 payload
    await user.click(screen.getByRole('button', { name: '儲存 Cookie' }))
    expect(puts).toBe(1)
    expect(screen.getByRole('alert')).toHaveTextContent('請至少填入一個 Cookie')
  })

  it('iCloud 登入收到 OTP_REQUIRED 後只顯示 OTP 輸入並可重試', async () => {
    let calls = 0
    const bodies: Array<Record<string, unknown>> = []
    server.use(
      accountsHandler(),
      http.post('/api/accounts/:id/login', async ({ request }) => {
        calls++
        bodies.push((await request.json()) as Record<string, unknown>)
        if (calls === 1) {
          return HttpResponse.json(
            { success: false, code: 'OTP_REQUIRED', message: '需要提供 OTP 驗證碼' },
            { status: 409 },
          )
        }
        return HttpResponse.json({ success: true, data: accounts[0] })
      }),
    )

    renderPage()
    await screen.findByText('活躍號')
    const user = userEvent.setup()
    await user.click(screen.getAllByRole('button', { name: /iCloud 登入/ })[0])
    // 注意:Dialog 目前每次 render 都會把焦點移回第一個可聚焦元素,
    // 逐字輸入會被中斷,因此這裡以單次 change 事件填入(見最終報告的來源缺陷)。
    fireEvent.change(screen.getByLabelText(/Apple ID 密碼/), { target: { value: 'p@ssw0rd' } })
    let dialog = screen.getByRole('dialog')
    await user.click(within(dialog).getByRole('button', { name: '登入' }))

    // 出現 OTP 輸入,密碼欄位收起來
    const otpInput = await screen.findByLabelText(/驗證碼/)
    expect(otpInput).toHaveAttribute('inputmode', 'numeric')
    expect(screen.queryByLabelText(/Apple ID 密碼/)).toBeNull()

    fireEvent.change(otpInput, { target: { value: '123456' } })
    dialog = screen.getByRole('dialog')
    await user.click(within(dialog).getByRole('button', { name: '驗證' }))

    await waitFor(() => expect(calls).toBe(2))
    expect(bodies[0].password).toBe('p@ssw0rd')
    expect(bodies[1].otp_code).toBe('123456')
    // 成功後對話框關閉
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  })

  it('App Password:既有帳號不必再填完整 iCloud 信箱，只送密碼', async () => {
    let pwdBody = ''
    server.use(
      accountsHandler(),
      http.post('/api/accounts/:id/password', async ({ request }) => {
        pwdBody = await request.text()
        return HttpResponse.json({ success: true, data: accounts[0] })
      }),
    )

    renderPage()
    await screen.findByText('活躍號')
    const user = userEvent.setup()
    await user.click(screen.getAllByRole('button', { name: 'App 密碼' })[0])

    // 不再出現「完整 iCloud 信箱」輸入框,改為顯示帳號已存的位址
    const dialog = screen.getByRole('dialog')
    expect(screen.queryByLabelText(/完整 iCloud 信箱/)).toBeNull()
    expect(within(dialog).getByText('active@icloud.com')).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText('App 專用密碼'), {
      target: { value: 'xxxx-xxxx-xxxx-xxxx' },
    })
    await user.click(screen.getByRole('button', { name: '驗證並儲存' }))

    await waitFor(() => expect(pwdBody).not.toBe(''))
    const body = JSON.parse(pwdBody) as { icloud_email: string; app_password: string }
    // 帶空字串讓後端沿用帳號上的信箱（前端不重送一份可能過期的位址）
    expect(body.icloud_email).toBe('')
    expect(body.app_password).toBe('xxxx-xxxx-xxxx-xxxx')
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  })

  it('App Password:帳號沒有信箱時仍需填寫，且未填不送出請求', async () => {
    let posted = false
    server.use(
      accountsHandler([{ ...accounts[0], icloud_email: '' }]),
      http.post('/api/accounts/:id/password', () => {
        posted = true
        return HttpResponse.json({ success: true, data: accounts[0] })
      }),
    )

    renderPage()
    await screen.findByText('活躍號')
    const user = userEvent.setup()
    await user.click(screen.getAllByRole('button', { name: 'App 密碼' })[0])

    // 這種情況下才需要自己填
    fireEvent.change(screen.getByLabelText(/完整 iCloud 信箱/), {
      target: { value: 'typed@icloud.com' },
    })
    fireEvent.change(screen.getByLabelText('App 專用密碼'), { target: { value: '' } })
    await user.click(screen.getByRole('button', { name: '驗證並儲存' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('請輸入 App 專用密碼')
    expect(posted).toBe(false)
  })

  it('App Password 提交後不保留已送出的密碼', async () => {
    let pwdBody = ''
    server.use(
      accountsHandler(),
      http.post('/api/accounts/:id/password', async ({ request }) => {
        pwdBody = await request.text()
        return HttpResponse.json({ success: true, data: accounts[0] })
      }),
    )

    renderPage()
    await screen.findByText('活躍號')
    const user = userEvent.setup()
    await user.click(screen.getAllByRole('button', { name: 'App 密碼' })[0])
    fireEvent.change(screen.getByLabelText('App 專用密碼'), {
      target: { value: 'xxxx-xxxx-xxxx-xxxx' },
    })
    await user.click(screen.getByRole('button', { name: '驗證並儲存' }))

    await waitFor(() => expect(pwdBody).toContain('xxxx-xxxx-xxxx-xxxx'))
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())

    // 重新開啟:欄位為空,不保留已送出的密碼
    await user.click(screen.getAllByRole('button', { name: 'App 密碼' })[0])
    expect((screen.getByLabelText('App 專用密碼') as HTMLInputElement).value).toBe('')
  })

  it('代理從不回顯:儲存後對話框關閉,重新開啟仍為空', async () => {
    let proxyValue = ''
    server.use(
      accountsHandler(),
      http.put('/api/accounts/:id/proxy', async ({ request }) => {
        const body = (await request.json()) as { proxy: string }
        proxyValue = body.proxy
        return HttpResponse.json({
          success: true,
          data: { ...accounts[0], has_proxy: body.proxy !== '' },
        })
      }),
    )

    renderPage()
    await screen.findByText('活躍號')
    const user = userEvent.setup()
    await user.click(screen.getAllByRole('button', { name: '代理' })[0])

    const input = screen.getByLabelText(/代理位址/) as HTMLInputElement
    expect(input.value).toBe('')
    fireEvent.change(input, { target: { value: 'http://u:p@proxy.example.com:8080' } })
    await user.click(screen.getByRole('button', { name: '儲存' }))

    await waitFor(() => expect(proxyValue).toBe('http://u:p@proxy.example.com:8080'))
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    // 畫面上不得回顯代理位址
    expect(screen.queryByText('http://u:p@proxy.example.com:8080')).toBeNull()

    // 重新開啟仍為空(從不回顯)
    await user.click(screen.getAllByRole('button', { name: '代理' })[0])
    expect((screen.getByLabelText(/代理位址/) as HTMLInputElement).value).toBe('')
  })

  it('刪除要求輸入帳號名稱精確匹配,取消不發請求', async () => {
    let deleted = false
    server.use(
      accountsHandler(),
      http.delete('/api/accounts/:id', () => {
        deleted = true
        return HttpResponse.json({ success: true, data: { id: 'acc_active' } })
      }),
    )

    renderPage()
    await screen.findByText('活躍號')
    const user = userEvent.setup()
    await user.click(screen.getAllByRole('button', { name: /刪除帳號/ })[0])

    const dialog = screen.getByRole('dialog', { name: '刪除帳號' })
    expect(dialog).toHaveTextContent('活躍號')
    const confirmButton = within(dialog).getByRole('button', { name: '確認刪除' })
    const confirmInput = within(dialog).getByLabelText(/輸入帳號名稱以確認/)

    // 名稱不匹配時按鈕禁用
    fireEvent.change(confirmInput, { target: { value: '錯誤名稱' } })
    expect(confirmButton).toBeDisabled()

    // 精確匹配才啟用
    fireEvent.change(confirmInput, { target: { value: '活躍號' } })
    expect(confirmButton).toBeEnabled()

    // 取消不發請求
    await user.click(within(dialog).getByRole('button', { name: '取消' }))
    expect(deleted).toBe(false)
    expect(screen.queryByRole('dialog')).toBeNull()

    // 精確匹配後確認才刪除
    await user.click(screen.getAllByRole('button', { name: /刪除帳號/ })[0])
    const secondDialog = screen.getByRole('dialog', { name: '刪除帳號' })
    fireEvent.change(within(secondDialog).getByLabelText(/輸入帳號名稱以確認/), {
      target: { value: '活躍號' },
    })
    await user.click(within(secondDialog).getByRole('button', { name: '確認刪除' }))
    await waitFor(() => expect(deleted).toBe(true))
  })
})
