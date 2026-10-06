import { http, HttpResponse } from 'msw'
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it } from 'vitest'
import TempMailPage from './TempMailPage'
import { server } from '../test/server'
import { setCSRFToken } from '../api/client'
import { ToastProvider } from '../components/ToastProvider'
import {
  TEMP_TTL_SECONDS,
  TEMP_ZERO_TIME,
  resetTempStore,
  tempMailboxEmail,
} from '../test/handlers'
import type { AccountSummary, TempMailbox } from '../api/types'

const ACCOUNT_ID = 'acc_main'

/** 具備 Cookie 的帳號（可建立隨機信箱）。 */
const usableAccount: AccountSummary = {
  id: ACCOUNT_ID,
  name: '主號',
  real_email: 'main@example.com',
  icloud_email: 'main@icloud.com',
  host: 'icloud.com',
  status: 'active',
  alias_total: 0,
  alias_active: 0,
  has_cookies: true,
  has_app_password: false,
  has_proxy: false,
  last_validated: '2026-10-06T09:00:00Z',
  created_at: '2026-10-01T09:00:00Z',
}

/** 第二個有 Cookie 的可用帳號（用來驗證帳號下拉）。 */
const secondAccount: AccountSummary = {
  ...usableAccount,
  id: 'acc_second',
  name: '備援號',
  real_email: 'second@example.com',
  icloud_email: 'second@icloud.com',
}

/**
 * 只有 App 密碼、沒有 Cookie 的帳號：**不可**用來建立隨機信箱。
 *
 * 建立 HME 別名一定要有 Cookie，App 專用密碼只能拿來登入換 Cookie；後端也是
 * 據此挑帳號，前端若放行就會送出一個註定失敗的請求。
 */
const appPasswordAccount: AccountSummary = {
  ...usableAccount,
  id: 'acc_app_pwd',
  name: '只有密碼',
  real_email: 'pwd@example.com',
  icloud_email: 'pwd@icloud.com',
  has_cookies: false,
  has_app_password: true,
}

/** 兩種憑證都沒有的帳號（不可用）。 */
const bareAccount: AccountSummary = {
  ...usableAccount,
  id: 'acc_bare',
  name: '空號',
  real_email: 'bare@example.com',
  icloud_email: 'bare@icloud.com',
  status: 'pending',
  has_cookies: false,
  has_app_password: false,
}

function accountsHandler(list: AccountSummary[] = [usableAccount]) {
  return http.get('/api/accounts', () => HttpResponse.json({ success: true, data: list }))
}

function tempListHandler(mailboxes: TempMailbox[]) {
  return http.get('/api/temp', () =>
    HttpResponse.json({
      success: true,
      data: { count: mailboxes.length, mailboxes, ttl_seconds: TEMP_TTL_SECONDS },
    }),
  )
}

/** 建立一個隨機信箱 fixture（每次呼叫都拿到全新時間，避免測試互相污染）。 */
function mailbox(overrides: Partial<TempMailbox> = {}): TempMailbox {
  const now = Date.now()
  return {
    id: 'temp_1',
    email: tempMailboxEmail(0),
    account_id: ACCOUNT_ID,
    label: 'temp-jade-reef-4821',
    created_at: new Date(now).toISOString(),
    expires_at: new Date(now + TEMP_TTL_SECONDS * 1000).toISOString(),
    keep: false,
    ...overrides,
  }
}

/** 統計卡的值（依標籤找卡片）。 */
function statValue(label: string): string {
  const card = screen.getByText(label).closest('.stat')
  if (!card) throw new Error(`找不到統計卡：${label}`)
  return card.querySelector('.stat-value')?.textContent ?? ''
}

function renderPage() {
  return render(
    <MemoryRouter initialEntries={['/temp']}>
      <Routes>
        <Route
          path="/temp"
          element={
            <ToastProvider>
              <TempMailPage />
            </ToastProvider>
          }
        />
        <Route path="/accounts" element={<p>帳號管理頁</p>} />
      </Routes>
    </MemoryRouter>,
  )
}

/** 開啟「立即刪除」確認框並按下確認。 */
async function confirmDeleteMailbox(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole('button', { name: '立即刪除' }))
  const dialog = await screen.findByRole('dialog', { name: '刪除隨機信箱' })
  await user.click(within(dialog).getByRole('button', { name: '立即刪除' }))
}

describe('TempMailPage', () => {
  beforeEach(() => {
    setCSRFToken('csrf-test')
    resetTempStore()
    server.resetHandlers()
  })

  it('沒有追蹤中的隨機信箱時自動建立一個（僅一次）並顯示位址與倒數', async () => {
    server.use(accountsHandler())
    renderPage()

    expect(await screen.findByText(tempMailboxEmail(0))).toBeInTheDocument()
    expect(screen.getByText('temp-jade-reef-4821 · 主號')).toBeInTheDocument()
    // 只建立一個：統計卡顯示 1，且第二個種子沒出現
    expect(statValue('追蹤中的信箱')).toBe('1')
    expect(screen.queryByText(tempMailboxEmail(1))).not.toBeInTheDocument()

    // 24 小時 TTL → 有倒數且不是警示色
    const countdown = screen.getByTestId('temp-countdown')
    expect(countdown.textContent).toMatch(/^將於 .+後自動刪除$/)
    expect(countdown.className).not.toContain('is-urgent')
    expect(screen.getByText('自動刪除')).toBeInTheDocument()
  })

  it('倒數文字符合「將於 23 小時 58 分後自動刪除」的格式', async () => {
    // 多加 2 秒緩衝，避免因為 render 前的毫秒差被無條件捨去成 57 分。
    const almostADay = mailbox({
      expires_at: new Date(Date.now() + (23 * 3600 + 58 * 60 + 2) * 1000).toISOString(),
    })
    server.use(accountsHandler(), tempListHandler([almostADay]))
    renderPage()

    const countdown = await screen.findByTestId('temp-countdown')
    expect(countdown.textContent).toBe('將於 23 小時 58 分後自動刪除')
  })

  it('倒數計時每秒更新，剩餘不足 1 小時時使用警示色並顯示秒數', async () => {
    // 用真實計時器：剩餘 90 秒 → 顯示到秒，且每秒都會變，不必依賴假計時器。
    const soon = mailbox({
      expires_at: new Date(Date.now() + 90_000).toISOString(),
    })
    server.use(accountsHandler(), tempListHandler([soon]))
    renderPage()

    const countdown = await screen.findByTestId('temp-countdown')
    expect(countdown.textContent).toMatch(/將於 \d+ 分 \d+ 秒後自動刪除/)
    expect(countdown.className).toContain('is-urgent')

    const before = countdown.textContent
    await waitFor(() => expect(countdown.textContent).not.toBe(before), { timeout: 4000 })
  })

  it('「換個郵箱」建立新的並切換顯示，舊的仍留在追蹤清單', async () => {
    server.use(accountsHandler())
    renderPage()
    await screen.findByText(tempMailboxEmail(0))

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: '換個郵箱' }))

    expect(await screen.findByText(tempMailboxEmail(1))).toBeInTheDocument()
    expect(statValue('追蹤中的信箱')).toBe('2')

    // 舊的沒有被刪掉，而是出現在「其他追蹤中的隨機信箱」
    const others = screen.getByText('其他追蹤中的隨機信箱').closest('.card') as HTMLElement
    expect(within(others).getByText(tempMailboxEmail(0))).toBeInTheDocument()
    expect(within(others).queryByText(tempMailboxEmail(1))).not.toBeInTheDocument()
  })

  it('切換「不自動刪除」會呼叫 keep API，且改顯示不會被自動刪除', async () => {
    const keepCalls: Array<{ id: string; keep: boolean }> = []
    const base = mailbox()
    server.use(
      accountsHandler(),
      tempListHandler([base]),
      http.post('/api/temp/:id/keep', async ({ params, request }) => {
        const body = (await request.json()) as { keep?: boolean }
        const keep = body.keep === true
        keepCalls.push({ id: String(params.id), keep })
        return HttpResponse.json({
          success: true,
          data: {
            ...base,
            id: String(params.id),
            keep,
            expires_at: keep
              ? TEMP_ZERO_TIME
              : new Date(Date.now() + TEMP_TTL_SECONDS * 1000).toISOString(),
          },
        })
      }),
    )
    renderPage()
    await screen.findByText(tempMailboxEmail(0))

    const user = userEvent.setup()
    const toggle = screen.getByRole('checkbox', { name: '不自動刪除' })
    expect(toggle).not.toBeChecked()
    await user.click(toggle)

    await waitFor(() =>
      expect(screen.getByRole('checkbox', { name: '不自動刪除' })).toBeChecked(),
    )
    expect(keepCalls).toEqual([{ id: base.id, keep: true }])
    // 零值時間不能被當成「已過期」
    expect(screen.getByTestId('temp-countdown')).toHaveTextContent(
      '已設為不自動刪除，這個信箱不會被自動刪除',
    )
    expect(screen.getByTestId('temp-countdown').className).toContain('is-kept')
  })

  it('「立即刪除」需經確認，刪除後顯示空狀態且不會自動再建立', async () => {
    server.use(accountsHandler())
    renderPage()
    await screen.findByText(tempMailboxEmail(0))

    const user = userEvent.setup()
    await confirmDeleteMailbox(user)

    expect(await screen.findByText('目前沒有追蹤中的隨機信箱。')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '建立隨機信箱' })).toBeInTheDocument()
    expect(screen.queryByText(tempMailboxEmail(0))).not.toBeInTheDocument()
    expect(statValue('追蹤中的信箱')).toBe('0')

    // 重新載入也不會偷偷再建一個
    await user.click(screen.getByRole('button', { name: '重新載入' }))
    expect(await screen.findByText('目前沒有追蹤中的隨機信箱。')).toBeInTheDocument()
    expect(screen.queryByText(tempMailboxEmail(0))).not.toBeInTheDocument()
    expect(screen.queryByText(tempMailboxEmail(1))).not.toBeInTheDocument()
  })

  it('收件匣顯示該信箱收到的郵件；換到沒有信的信箱時顯示空狀態', async () => {
    server.use(accountsHandler())
    renderPage()
    await screen.findByText(tempMailboxEmail(0))

    // 第一個種子信箱在 fixture 裡有兩封信（handlers 依 alias 過濾）
    expect(await screen.findByText('歡迎加入，請驗證你的信箱')).toBeInTheDocument()
    expect(screen.getByText('no-reply@shop.example')).toBeInTheDocument()
    expect(screen.getByText('本週電子報')).toBeInTheDocument()
    expect(statValue('本信箱郵件')).toBe('2')

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: '換個郵箱' }))
    await screen.findByText(tempMailboxEmail(1))

    expect(
      await screen.findByText('還沒有收到郵件，把這個信箱拿去註冊服務吧'),
    ).toBeInTheDocument()
    expect(screen.queryByText('歡迎加入，請驗證你的信箱')).not.toBeInTheDocument()
  })

  it('點「讀取」會開啟對話框顯示完整內文', async () => {
    server.use(accountsHandler())
    renderPage()
    await screen.findByText('歡迎加入，請驗證你的信箱')

    const user = userEvent.setup()
    const [firstRead] = screen.getAllByRole('button', { name: '讀取' })
    await user.click(firstRead)

    const dialog = await screen.findByRole('dialog', { name: '歡迎加入，請驗證你的信箱' })
    expect(within(dialog).getByText(/請點擊以下連結完成驗證/)).toBeInTheDocument()
    expect(within(dialog).getByText(`收件信箱：${tempMailboxEmail(0)}`)).toBeInTheDocument()
  })

  it('其他隨機信箱可以切換顯示與刪除', async () => {
    const newer = mailbox({ created_at: '2026-10-06T10:00:00Z' })
    const older = mailbox({
      id: 'temp_2',
      email: tempMailboxEmail(1),
      label: 'temp-amber-tide-7702',
      created_at: '2026-10-06T09:00:00Z',
      keep: true,
      expires_at: TEMP_ZERO_TIME,
    })
    server.use(accountsHandler(), tempListHandler([newer, older]))
    renderPage()

    // 最新的一筆是主卡
    await screen.findByText(tempMailboxEmail(0))
    const others = screen.getByText('其他追蹤中的隨機信箱').closest('.card') as HTMLElement
    expect(within(others).getByText(tempMailboxEmail(1))).toBeInTheDocument()
    expect(within(others).getByText('不自動刪除')).toBeInTheDocument()
    expect(within(others).getByText('不會自動刪除')).toBeInTheDocument()

    const user = userEvent.setup()
    await user.click(within(others).getByRole('button', { name: '切換' }))

    // 切換後主卡換成原本的另一筆，且沿用它的 keep 狀態
    await waitFor(() =>
      expect(screen.getByTestId('temp-countdown')).toHaveTextContent(
        '已設為不自動刪除，這個信箱不會被自動刪除',
      ),
    )
    const stillOthers = screen.getByText('其他追蹤中的隨機信箱').closest('.card') as HTMLElement
    expect(within(stillOthers).getByText(tempMailboxEmail(0))).toBeInTheDocument()

    // 從清單刪掉它
    await user.click(
      within(stillOthers).getByRole('button', { name: '刪除隨機信箱' }),
    )
    const dialog = await screen.findByRole('dialog', { name: '刪除隨機信箱' })
    await user.click(within(dialog).getByRole('button', { name: '立即刪除' }))

    await waitFor(() =>
      expect(screen.queryByText('其他追蹤中的隨機信箱')).not.toBeInTheDocument(),
    )
    expect(statValue('追蹤中的信箱')).toBe('1')
  })

  it('沒有可用帳號時顯示引導訊息，且完全不發出建立或查詢請求', async () => {
    let listCalls = 0
    let createCalls = 0
    server.use(
      accountsHandler([bareAccount]),
      http.get('/api/temp', () => {
        listCalls += 1
        return HttpResponse.json({
          success: true,
          data: { count: 0, mailboxes: [], ttl_seconds: TEMP_TTL_SECONDS },
        })
      }),
      http.post('/api/temp', () => {
        createCalls += 1
        return HttpResponse.json({ success: true, data: mailbox() })
      }),
    )
    renderPage()

    expect(await screen.findByText(/還沒有可用的帳號/)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: '帳號管理' })).toBeInTheDocument()
    expect(listCalls).toBe(0)
    expect(createCalls).toBe(0)
    expect(screen.queryByRole('button', { name: '換個郵箱' })).not.toBeInTheDocument()
  })

  it('多個可用帳號時顯示帳號下拉，只有一個時不顯示', async () => {
    server.use(accountsHandler([usableAccount, secondAccount]))
    const { unmount } = renderPage()

    await screen.findByText(tempMailboxEmail(0))
    const select = screen.getByLabelText('用來建立隨機信箱的帳號')
    expect(select).toHaveValue(ACCOUNT_ID)
    unmount()

    server.use(accountsHandler([usableAccount]))
    renderPage()
    await screen.findByText(tempMailboxEmail(0))
    expect(screen.queryByLabelText('用來建立隨機信箱的帳號')).not.toBeInTheDocument()
  })

  // 與後端的挑帳號規則保持一致：只有 App 密碼的帳號不能拿來建立隨機信箱，
  // 因此不該出現在下拉，也不該在只有它時被當成可用帳號。
  it('只有 App 密碼的帳號不會被當成可用帳號', async () => {
    server.use(accountsHandler([appPasswordAccount]))
    renderPage()

    expect(await screen.findByText(/還沒有可用的帳號/)).toBeInTheDocument()
    expect(screen.queryByLabelText('用來建立隨機信箱的帳號')).not.toBeInTheDocument()
  })

  it('有 Cookie 的帳號與只有 App 密碼的帳號並存時，下拉只列出有 Cookie 的', async () => {
    server.use(accountsHandler([usableAccount, secondAccount, appPasswordAccount]))
    renderPage()

    await screen.findByText(tempMailboxEmail(0))
    const select = screen.getByLabelText('用來建立隨機信箱的帳號')
    const options = within(select).getAllByRole('option')
    expect(options).toHaveLength(2)
    expect(options.map((o) => o.getAttribute('value'))).toEqual([ACCOUNT_ID, 'acc_second'])
    expect(within(select).queryByText(/只有密碼/)).not.toBeInTheDocument()
  })

  it('建立失敗時原樣顯示後端訊息（含 502 上游失敗的 Toast）', async () => {
    let createCalls = 0
    server.use(
      accountsHandler(),
      http.post('/api/temp', () => {
        createCalls += 1
        if (createCalls === 1) {
          return HttpResponse.json({ success: true, data: mailbox() })
        }
        return HttpResponse.json(
          { success: false, code: 'UPSTREAM_FAILURE', message: 'iCloud 暫時無法連線' },
          { status: 502 },
        )
      }),
    )
    renderPage()
    await screen.findByText(tempMailboxEmail(0))

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: '換個郵箱' }))

    expect(await screen.findByText('iCloud 暫時無法連線')).toBeInTheDocument()
    // 失敗時保留原本的信箱，不會變成空狀態
    expect(screen.getByText(tempMailboxEmail(0))).toBeInTheDocument()
  })

  it('首次自動建立失敗時，把後端訊息顯示在錯誤狀態並可重試', async () => {
    let createCalls = 0
    server.use(
      accountsHandler(),
      http.post('/api/temp', () => {
        createCalls += 1
        if (createCalls === 1) {
          return HttpResponse.json(
            { success: false, code: 'VALIDATION_ERROR', message: '沒有可用的帳號可建立隨機信箱' },
            { status: 400 },
          )
        }
        return HttpResponse.json({ success: true, data: mailbox() })
      }),
    )
    renderPage()

    expect(await screen.findByText('沒有可用的帳號可建立隨機信箱')).toBeInTheDocument()

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: '重試' }))
    expect(await screen.findByText(tempMailboxEmail(0))).toBeInTheDocument()
  })

  it('分頁重新變為可見時立刻重新整理收件匣', async () => {
    let inboxCalls = 0
    server.use(
      accountsHandler(),
      http.get('/api/inbox', () => {
        inboxCalls += 1
        return HttpResponse.json({
          success: true,
          data: { account_id: ACCOUNT_ID, count: 0, messages: [], method: 'imap' },
        })
      }),
    )
    renderPage()

    await screen.findByText(tempMailboxEmail(0))
    expect(
      await screen.findByText('還沒有收到郵件，把這個信箱拿去註冊服務吧'),
    ).toBeInTheDocument()
    await waitFor(() => expect(inboxCalls).toBe(1))

    await act(async () => {
      document.dispatchEvent(new Event('visibilitychange'))
    })
    await waitFor(() => expect(inboxCalls).toBe(2))
  })

  it('刪除時若 iCloud 端失敗，提示已停止追蹤但上游刪除失敗', async () => {
    server.use(
      accountsHandler(),
      http.delete('/api/temp/:id', ({ params }) =>
        HttpResponse.json({
          success: true,
          data: {
            id: String(params.id),
            email: tempMailboxEmail(0),
            removed: true,
            upstream_warning: '別名不存在',
          },
        }),
      ),
    )
    renderPage()
    await screen.findByText(tempMailboxEmail(0))

    const user = userEvent.setup()
    await confirmDeleteMailbox(user)

    expect(
      await screen.findByText('已停止追蹤，但 iCloud 端刪除失敗：別名不存在'),
    ).toBeInTheDocument()
    expect(screen.getByText('目前沒有追蹤中的隨機信箱。')).toBeInTheDocument()
  })
})
