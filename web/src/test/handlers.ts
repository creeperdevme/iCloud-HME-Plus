import { http, HttpResponse } from 'msw'
import type { InboxMessage, TempMailbox } from '../api/types'

/* ------------------------------------------------------------------ *
 * 隨機信箱測試資料
 *
 * 後端會產生隨機標籤，這裡改用可預期的固定序列，讓測試能穩定斷言，
 * 也讓收件匣 fixture 能精準對上某一個隨機信箱位址。
 * ------------------------------------------------------------------ */

/** 後端 TTL：24 小時。 */
export const TEMP_TTL_SECONDS = 24 * 60 * 60

/** keep=true 時後端回傳的零值時間（不可當成「已過期」）。 */
export const TEMP_ZERO_TIME = '0001-01-01T00:00:00Z'

/** 依序產生的隨機信箱標籤。 */
export const TEMP_MAILBOX_LABELS: string[] = [
  'temp-jade-reef-4821',
  'temp-amber-tide-7702',
  'temp-coral-mist-3390',
]

/** 第 index 個隨機信箱的完整位址。 */
export function tempMailboxEmail(index: number): string {
  const label = TEMP_MAILBOX_LABELS[index] ?? `temp-extra-${index}`
  return `${label}@icloud.com`
}

/** 預設收件匣：只有第一個隨機信箱收得到信，其餘為空（用來驗證空狀態）。 */
export const inboxFixture: InboxMessage[] = [
  {
    id: 'msg-jade-1',
    from: 'no-reply@shop.example',
    to: tempMailboxEmail(0),
    subject: '歡迎加入，請驗證你的信箱',
    date: '2026-10-06T02:30:00Z',
    preview: '點擊下方連結完成驗證，連結 24 小時內有效。',
  },
  {
    id: 'msg-jade-2',
    from: 'newsletter@example.org',
    to: tempMailboxEmail(0),
    subject: '本週電子報',
    date: '2026-10-06T01:00:00Z',
    preview: '這是測試用的第二封信。',
  },
]

/** 讀信對話框用的完整內文。 */
export const fullMessageBodies: Record<string, string> = {
  'msg-jade-1': '您好：\n\n請點擊以下連結完成驗證。\n\n謝謝！',
  'msg-jade-2': '本週電子報內容（測試）。',
}

/* ------------------------------------------------------------------ *
 * 可變狀態（每個 test 的 beforeEach 請呼叫 resetTempStore()）
 * ------------------------------------------------------------------ */

let tempSeq = 0
let tempStore: TempMailbox[] = []
const deletedMessageIds = new Set<string>()

/** 清空隨機信箱與收件匣的測試狀態。 */
export function resetTempStore(): void {
  tempSeq = 0
  tempStore = []
  deletedMessageIds.clear()
}

function liveMessages(): InboxMessage[] {
  return inboxFixture.filter((m) => !deletedMessageIds.has(m.id))
}

function nextTempMailbox(accountId: string): TempMailbox {
  const index = tempSeq
  tempSeq += 1
  const label = TEMP_MAILBOX_LABELS[index] ?? `temp-extra-${index}`
  const now = Date.now()
  return {
    id: `temp_${index + 1}`,
    email: `${label}@icloud.com`,
    account_id: accountId,
    label,
    created_at: new Date(now).toISOString(),
    expires_at: new Date(now + TEMP_TTL_SECONDS * 1000).toISOString(),
    keep: false,
  }
}

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
  http.get('/api/accounts', () => HttpResponse.json({ success: true, data: [] })),

  /* 隨機信箱 */
  http.get('/api/temp', () =>
    HttpResponse.json({
      success: true,
      data: {
        count: tempStore.length,
        mailboxes: tempStore,
        ttl_seconds: TEMP_TTL_SECONDS,
      },
    }),
  ),
  http.post('/api/temp', async ({ request }) => {
    const body = (await request.json().catch(() => ({}))) as { account_id?: string }
    const mailbox = nextTempMailbox(body.account_id ?? 'acc_temp')
    tempStore = [mailbox, ...tempStore]
    return HttpResponse.json({ success: true, data: mailbox })
  }),
  http.post('/api/temp/:id/keep', async ({ params, request }) => {
    const body = (await request.json().catch(() => ({}))) as { keep?: boolean }
    const keep = body.keep === true
    const found = tempStore.find((m) => m.id === params.id)
    if (!found) {
      return HttpResponse.json(
        { success: false, code: 'NOT_FOUND', message: '找不到這個隨機信箱' },
        { status: 404 },
      )
    }
    // 關閉 keep 時後端會重新起算一輪 TTL。
    const updated: TempMailbox = {
      ...found,
      keep,
      expires_at: keep
        ? TEMP_ZERO_TIME
        : new Date(Date.now() + TEMP_TTL_SECONDS * 1000).toISOString(),
    }
    tempStore = tempStore.map((m) => (m.id === updated.id ? updated : m))
    return HttpResponse.json({ success: true, data: updated })
  }),
  http.delete('/api/temp/:id', ({ params }) => {
    const found = tempStore.find((m) => m.id === params.id)
    tempStore = tempStore.filter((m) => m.id !== params.id)
    return HttpResponse.json({
      success: true,
      data: { id: String(params.id), email: found?.email ?? '', removed: true },
    })
  }),

  /* 收件匣（依 alias 過濾） */
  http.get('/api/inbox', ({ request }) => {
    const alias = new URL(request.url).searchParams.get('alias')
    const all = liveMessages()
    const messages = alias ? all.filter((m) => m.to === alias) : all
    return HttpResponse.json({
      success: true,
      data: {
        account_id: 'acc_temp',
        ...(alias ? { alias } : {}),
        count: messages.length,
        messages,
        method: 'imap',
      },
    })
  }),
  http.get('/api/inbox/:id', ({ params }) => {
    const message = liveMessages().find((m) => m.id === params.id)
    if (!message) {
      return HttpResponse.json(
        { success: false, code: 'NOT_FOUND', message: '找不到這封郵件' },
        { status: 404 },
      )
    }
    return HttpResponse.json({
      success: true,
      data: {
        ...message,
        body: fullMessageBodies[message.id] ?? '（無內文）',
        content_type: 'text/plain',
      },
    })
  }),
  http.delete('/api/inbox/:id', ({ params }) => {
    deletedMessageIds.add(String(params.id))
    return HttpResponse.json({ success: true, data: { deleted: true, id: params.id } })
  }),
]
