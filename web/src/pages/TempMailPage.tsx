import { useCallback, useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  ApiError,
  createTempMailbox,
  deleteTempMailbox,
  listTempMailboxes,
  request,
  setTempMailboxKeep,
} from '../api/client'
import type {
  AccountSummary,
  FullMessage,
  InboxMessage,
  InboxResult,
  TempMailbox,
} from '../api/types'
import AsyncState from '../components/AsyncState'
import ConfirmDialog from '../components/ConfirmDialog'
import Dialog from '../components/Dialog'
import { useToast } from '../components/ToastProvider'
import {
  IconCheck,
  IconClock,
  IconCopy,
  IconMail,
  IconPlus,
  IconRefresh,
  IconSparkles,
  IconTempMail,
  IconTrash,
} from '../components/icons'
import { copyText } from '../utils/clipboard'
import {
  INBOX_DAYS,
  INBOX_LIMIT,
  INBOX_REFRESH_MS,
  URGENT_THRESHOLD_MS,
  ZERO_TIME_PREFIX,
  expiresAtMs,
  formatClock,
  formatDateTime,
  formatRemaining,
  isUsableAccount,
  latestMailbox,
} from '../utils/tempmail'

/** 收件匣狀態：mailboxId 用來判斷資料是否屬於目前顯示的信箱。 */
interface InboxState {
  mailboxId: string
  messages: InboxMessage[]
  updatedAt: Date | null
  error: string
}

const EMPTY_INBOX: InboxState = {
  mailboxId: '',
  messages: [],
  updatedAt: null,
  error: '',
}

export default function TempMailPage() {
  const { show } = useToast()

  const [accounts, setAccounts] = useState<AccountSummary[]>([])
  const [accountId, setAccountId] = useState('')
  const [mailboxes, setMailboxes] = useState<TempMailbox[]>([])
  const [ttlSeconds, setTtlSeconds] = useState(24 * 60 * 60)
  const [current, setCurrent] = useState<TempMailbox | null>(null)

  const [booting, setBooting] = useState(true)
  const [bootError, setBootError] = useState('')
  const [creating, setCreating] = useState(false)
  const [keepBusy, setKeepBusy] = useState(false)

  const [deleteTarget, setDeleteTarget] = useState<TempMailbox | null>(null)
  const [deleting, setDeleting] = useState(false)

  const [now, setNow] = useState(() => Date.now())

  const [inbox, setInbox] = useState<InboxState>(EMPTY_INBOX)
  const [inboxRefreshKey, setInboxRefreshKey] = useState(0)
  const [detail, setDetail] = useState<FullMessage | null>(null)
  const [detailLoading, setDetailLoading] = useState(false)
  const [deleteMessageFor, setDeleteMessageFor] = useState<InboxMessage | null>(null)
  const [deletingMessage, setDeletingMessage] = useState(false)

  /** 目前選定的帳號（用 ref 讀取，避免 bootstrap 依賴 accountId 而重跑）。 */
  const accountIdRef = useRef('')
  /** 防止 StrictMode 的第二次 effect 重複建立。 */
  const createGuardRef = useRef(false)
  /** 使用者主動刪除後就不再自動建立，避免嚇到人。 */
  const autoCreateRef = useRef(true)

  const usableAccounts = accounts.filter(isUsableAccount)
  const ttlHours = Math.max(1, Math.round(ttlSeconds / 3600))
  const currentAccount = accounts.find((a) => a.id === current?.account_id) ?? null

  /** 取得目前信箱所屬的收件匣狀態（不屬於目前信箱就當成還沒載入）。 */
  const inboxForCurrent = current && inbox.mailboxId === current.id ? inbox : null
  const inboxMessages = inboxForCurrent?.messages ?? []
  const inboxError = inboxForCurrent?.error ?? ''
  const inboxUpdatedAt = inboxForCurrent?.updatedAt ?? null
  const inboxLoading = current !== null && inboxForCurrent === null

  /**
   * 首次載入：帳號 → 追蹤清單 →（必要時）自動建立一個。
   *
   * 全部以 await 之後才 setState，避免同步 setState 造成的額外 render。
   */
  const bootstrap = useCallback(async () => {
    try {
      const all = await request<AccountSummary[]>('/api/accounts')
      setAccounts(all)

      const usable = all.filter(isUsableAccount)
      if (usable.length === 0) {
        setBootError('')
        return
      }

      const keptSelection = usable.some((a) => a.id === accountIdRef.current)
      const picked = keptSelection ? accountIdRef.current : usable[0].id
      accountIdRef.current = picked
      setAccountId(picked)

      const list = await listTempMailboxes()
      setMailboxes(list.mailboxes)
      setTtlSeconds(list.ttl_seconds)

      const latest = latestMailbox(list.mailboxes)
      if (latest) {
        setCurrent(latest)
        setBootError('')
        return
      }

      // 沒有追蹤中的隨機信箱 → 自動建立一個（只做一次）。
      if (!autoCreateRef.current || createGuardRef.current) {
        setBootError('')
        return
      }
      createGuardRef.current = true
      try {
        const created = await createTempMailbox(picked)
        setCurrent(created)
        setMailboxes([created])
        setBootError('')
      } finally {
        createGuardRef.current = false
      }
    } catch (err) {
      setBootError(err instanceof ApiError ? err.message : '網路連線失敗，請檢查服務狀態')
    } finally {
      setBooting(false)
    }
  }, [])

  useEffect(() => {
    // 初始載入是完整非同步流程（帳號 → 追蹤清單 → 必要時自動建立），
    // 所有 setState 都發生在 await 之後；這裡用 microtask 讓它離開 effect
    // 的同步階段，避免同一輪 render 內連續 setState。
    void Promise.resolve().then(bootstrap)
  }, [bootstrap])

  /** 倒數計時：只有需要倒數時才啟動計時器。 */
  const deadline = current ? expiresAtMs(current) : null
  useEffect(() => {
    if (deadline === null) return
    const timer = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [deadline])

  /** 收件匣：進入即有資料，每 15 秒重抓，分頁重新可見時立刻重抓。 */
  const mailboxId = current?.id ?? ''
  const mailboxAccountId = current?.account_id ?? ''
  const mailboxEmail = current?.email ?? ''
  useEffect(() => {
    if (!mailboxId) return
    let cancelled = false
    const controller = new AbortController()
    const params = new URLSearchParams({
      account_id: mailboxAccountId,
      alias: mailboxEmail,
      limit: String(INBOX_LIMIT),
      days: String(INBOX_DAYS),
    })

    const run = async () => {
      try {
        const data = await request<InboxResult>(`/api/inbox?${params.toString()}`, {
          signal: controller.signal,
        })
        if (cancelled) return
        setInbox({
          mailboxId,
          messages: data.messages ?? [],
          updatedAt: new Date(),
          error: '',
        })
      } catch (err) {
        if (cancelled || (err instanceof ApiError && err.status === 0)) return
        setInbox({
          mailboxId,
          messages: [],
          updatedAt: new Date(),
          error: err instanceof ApiError ? err.message : '讀取收件匣失敗',
        })
      }
    }

    void run()
    const timer = window.setInterval(() => void run(), INBOX_REFRESH_MS)
    const handleVisibility = () => {
      if (document.visibilityState === 'visible') void run()
    }
    document.addEventListener('visibilitychange', handleVisibility)

    return () => {
      cancelled = true
      controller.abort()
      window.clearInterval(timer)
      document.removeEventListener('visibilitychange', handleVisibility)
    }
  }, [mailboxId, mailboxAccountId, mailboxEmail, inboxRefreshKey])

  function handleRetry() {
    setBooting(true)
    setBootError('')
    void bootstrap()
  }

  function handleAccountChange(id: string) {
    accountIdRef.current = id
    setAccountId(id)
  }

  async function handleCreate() {
    if (creating || usableAccounts.length === 0) return
    setCreating(true)
    try {
      const created = await createTempMailbox(accountIdRef.current || undefined)
      setCurrent(created)
      setMailboxes((prev) => [created, ...prev.filter((m) => m.id !== created.id)])
      show('已建立新的隨機信箱')
    } catch (err) {
      show(err instanceof ApiError ? err.message : '建立隨機信箱失敗')
    } finally {
      setCreating(false)
    }
  }

  async function handleToggleKeep(next: boolean) {
    if (!current || keepBusy) return
    setKeepBusy(true)
    try {
      const updated = await setTempMailboxKeep(current.id, next)
      setCurrent(updated)
      setMailboxes((prev) => prev.map((m) => (m.id === updated.id ? updated : m)))
      show(next ? '已設為不自動刪除' : `已恢復自動刪除，重新起算 ${ttlHours} 小時`)
    } catch (err) {
      show(err instanceof ApiError ? err.message : '更新設定失敗')
    } finally {
      setKeepBusy(false)
    }
  }

  async function handleDelete() {
    if (!deleteTarget) return
    const target = deleteTarget
    setDeleting(true)
    try {
      const result = await deleteTempMailbox(target.id)
      const remaining = mailboxes.filter((m) => m.id !== target.id)
      setMailboxes(remaining)

      if (current?.id === target.id) {
        // 不自動再建一個：只有在還有其他追蹤中的信箱時才切換過去。
        autoCreateRef.current = false
        setCurrent(latestMailbox(remaining))
      }

      setDeleteTarget(null)
      if (result.upstream_warning) {
        show(`已停止追蹤，但 iCloud 端刪除失敗：${result.upstream_warning}`)
      } else {
        show('隨機信箱已刪除')
      }
    } catch (err) {
      show(err instanceof ApiError ? err.message : '刪除隨機信箱失敗')
    } finally {
      setDeleting(false)
    }
  }

  function handleRefreshInbox() {
    setInboxRefreshKey((key) => key + 1)
  }

  async function handleCopy(email: string) {
    if (await copyText(email)) {
      show('已複製信箱位址')
    } else {
      show('複製失敗，請手動選取')
    }
  }

  async function openMessage(message: InboxMessage) {
    setDetailLoading(true)
    try {
      const data = await request<FullMessage>(
        `/api/inbox/${encodeURIComponent(message.id)}?account_id=${encodeURIComponent(mailboxAccountId)}`,
      )
      setDetail(data)
    } catch (err) {
      show(err instanceof ApiError ? err.message : '讀取郵件內容失敗')
    } finally {
      setDetailLoading(false)
    }
  }

  async function deleteMessage() {
    if (!deleteMessageFor) return
    setDeletingMessage(true)
    try {
      await request(
        `/api/inbox/${encodeURIComponent(deleteMessageFor.id)}?account_id=${encodeURIComponent(mailboxAccountId)}`,
        { method: 'DELETE' },
      )
      setDeleteMessageFor(null)
      setDetail(null)
      show('郵件已刪除')
      handleRefreshInbox()
    } catch (err) {
      show(err instanceof ApiError ? err.message : '刪除郵件失敗')
    } finally {
      setDeletingMessage(false)
    }
  }

  const keepCount = mailboxes.filter((m) => m.keep).length
  const otherMailboxes = current ? mailboxes.filter((m) => m.id !== current.id) : mailboxes

  // 倒數文字：keep=true 或零值時間都不算「已過期」。
  let countdownText = ''
  let countdownUrgent = false
  if (current) {
    if (deadline === null) {
      countdownText = '已設為不自動刪除，這個信箱不會被自動刪除'
    } else {
      const remaining = deadline - now
      countdownUrgent = remaining < URGENT_THRESHOLD_MS
      countdownText =
        remaining <= 0 ? '這個信箱即將被刪除' : `將於 ${formatRemaining(remaining)}後自動刪除`
    }
  }

  const head = (
    <header className="page-head">
      <div>
        <h2 className="page-title">隨機信箱</h2>
        <p className="page-sub">建立用完即丟的 iCloud 信箱，到期自動刪除</p>
      </div>
      <div className="page-actions">
        <button onClick={handleRetry} disabled={booting} title="重新載入隨機信箱狀態">
          <IconRefresh size={16} />
          重新載入
        </button>
      </div>
    </header>
  )

  if (usableAccounts.length === 0 && !booting && !bootError) {
    return (
      <div className="page">
        {head}
        <div className="card">
          <p className="empty-state">
            <IconTempMail size={40} className="empty-icon" />
            還沒有可用的帳號。隨機信箱需要一個已登入（具備 Cookie）的 iCloud 帳號，請先到{' '}
            <Link to="/accounts">帳號管理</Link> 新增並登入。
          </p>
        </div>
      </div>
    )
  }

  return (
    <div className="page">
      {head}

      <AsyncState
        loading={booting}
        error={bootError}
        empty={false}
        onRetry={handleRetry}
      >
        {usableAccounts.length > 1 && (
          <div className="card">
            <div className="toolbar">
              <div className="form-field">
                <label htmlFor="temp-account">用來建立隨機信箱的帳號</label>
                <select
                  id="temp-account"
                  value={accountId}
                  onChange={(e) => handleAccountChange(e.target.value)}
                >
                  {usableAccounts.map((a) => (
                    <option key={a.id} value={a.id}>
                      {a.name}（{a.icloud_email || a.real_email}）
                    </option>
                  ))}
                </select>
              </div>
            </div>
          </div>
        )}

        <div className="stat-grid">
          <div className="stat">
            <span className="stat-icon">
              <IconTempMail size={20} />
            </span>
            <div className="stat-body">
              <div className="stat-value">{mailboxes.length}</div>
              <div className="stat-label">追蹤中的信箱</div>
            </div>
          </div>
          <div className="stat">
            <span className={keepCount > 0 ? 'stat-icon is-success' : 'stat-icon'}>
              <IconCheck size={20} />
            </span>
            <div className="stat-body">
              <div className="stat-value">{keepCount}</div>
              <div className="stat-label">不自動刪除</div>
            </div>
          </div>
          <div className="stat">
            <span className="stat-icon">
              <IconMail size={20} />
            </span>
            <div className="stat-body">
              <div className="stat-value">{inboxMessages.length}</div>
              <div className="stat-label">本信箱郵件</div>
            </div>
          </div>
          <div className="stat">
            <span className="stat-icon">
              <IconClock size={20} />
            </span>
            <div className="stat-body">
              <div className="stat-value">{ttlHours} 小時</div>
              <div className="stat-label">自動刪除倒數</div>
            </div>
          </div>
        </div>

        {current === null ? (
          <div className="card">
            <p className="empty-state">
              <IconTempMail size={40} className="empty-icon" />
              目前沒有追蹤中的隨機信箱。
            </p>
            <div className="form-actions">
              <button className="primary" onClick={() => void handleCreate()} disabled={creating}>
                <IconPlus size={16} />
                {creating ? '建立中…' : '建立隨機信箱'}
              </button>
            </div>
          </div>
        ) : (
          <div className="card">
            <div className="card-head">
              <div>
                <h3 className="card-title">目前追蹤的隨機信箱</h3>
                <p className="card-sub">
                  {current.label}
                  {currentAccount ? ` · ${currentAccount.name}` : ''}
                </p>
              </div>
              <span className={current.keep ? 'badge badge-info' : 'badge badge-neutral'}>
                {current.keep ? '不自動刪除' : '自動刪除'}
              </span>
            </div>

            <div className="temp-address">
              <code className="temp-address-value">{current.email}</code>
              <button
                className="icon-button"
                aria-label="複製信箱位址"
                title="複製信箱位址"
                onClick={() => void handleCopy(current.email)}
              >
                <IconCopy size={15} />
              </button>
            </div>

            <p
              className={
                current.keep
                  ? 'temp-countdown is-kept'
                  : countdownUrgent
                    ? 'temp-countdown is-urgent'
                    : 'temp-countdown'
              }
              data-testid="temp-countdown"
            >
              <IconClock size={14} />
              {countdownText}
            </p>

            <label className="temp-switch">
              <input
                type="checkbox"
                checked={current.keep}
                disabled={keepBusy}
                onChange={(e) => void handleToggleKeep(e.target.checked)}
              />
              <span>不自動刪除</span>
            </label>
            <p className="hint">
              開啟後這個信箱不會被自動刪除；關閉後會重新起算 {ttlHours} 小時。
            </p>

            <div className="form-actions">
              <button onClick={() => void handleCreate()} disabled={creating}>
                <IconSparkles size={15} />
                {creating ? '建立中…' : '換個郵箱'}
              </button>
              <button className="danger" onClick={() => setDeleteTarget(current)}>
                <IconTrash size={15} />
                立即刪除
              </button>
            </div>
          </div>
        )}

        {current && (
          <>
            <div className="card">
              <div className="card-head">
                <div>
                  <h3 className="card-title">這個信箱收到的郵件</h3>
                  <p className="card-sub">
                    每 {INBOX_REFRESH_MS / 1000} 秒自動重新整理
                    {inboxUpdatedAt ? ` · 最後更新 ${formatClock(inboxUpdatedAt)}` : ''}
                  </p>
                </div>
                <button onClick={handleRefreshInbox} disabled={inboxLoading}>
                  <IconRefresh size={16} />
                  重新整理
                </button>
              </div>
            </div>

            <AsyncState
              loading={inboxLoading}
              error={inboxError}
              empty={inboxMessages.length === 0}
              emptyText="還沒有收到郵件，把這個信箱拿去註冊服務吧"
              onRetry={handleRefreshInbox}
            >
              <div className="table-wrap">
                <table>
                  <thead>
                    <tr>
                      <th>寄件人</th>
                      <th>主旨</th>
                      <th>時間</th>
                      <th>操作</th>
                    </tr>
                  </thead>
                  <tbody>
                    {inboxMessages.map((message) => (
                      <tr key={message.id}>
                        <td>
                          <span className="cell-strong">{message.from || '—'}</span>
                        </td>
                        <td>
                          <div className="preview-cell">
                            <span className="cell-strong">{message.subject || '(無主旨)'}</span>
                          </div>
                          {message.preview && (
                            <span className="cell-secondary">{message.preview}</span>
                          )}
                        </td>
                        <td>{formatDateTime(message.date)}</td>
                        <td>
                          <div className="row-actions">
                            <button className="small" onClick={() => void openMessage(message)}>
                              <IconMail size={13} />
                              讀取
                            </button>
                            <button
                              className="icon-button danger"
                              aria-label="刪除郵件"
                              title="刪除郵件"
                              onClick={() => setDeleteMessageFor(message)}
                            >
                              <IconTrash size={14} />
                            </button>
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </AsyncState>
          </>
        )}

        {otherMailboxes.length > 0 && (
          <div className="card">
            <div className="card-head">
              <div>
                <h3 className="card-title">其他追蹤中的隨機信箱</h3>
                <p className="card-sub">切換只改變顯示，不會影響各自的到期時間。</p>
              </div>
            </div>
            <div className="table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>信箱</th>
                    <th>狀態</th>
                    <th>到期</th>
                    <th>操作</th>
                  </tr>
                </thead>
                <tbody>
                  {otherMailboxes.map((mailbox) => (
                    <tr key={mailbox.id}>
                      <td className="cell-mono">{mailbox.email}</td>
                      <td>
                        <span className={mailbox.keep ? 'badge badge-info' : 'badge badge-neutral'}>
                          {mailbox.keep ? '不自動刪除' : '自動刪除'}
                        </span>
                      </td>
                      <td>
                        {mailbox.keep || mailbox.expires_at.startsWith(ZERO_TIME_PREFIX)
                          ? '不會自動刪除'
                          : formatDateTime(mailbox.expires_at)}
                      </td>
                      <td>
                        <div className="row-actions">
                          <button className="small" onClick={() => setCurrent(mailbox)}>
                            切換
                          </button>
                          <button
                            className="icon-button danger"
                            aria-label="刪除隨機信箱"
                            title="刪除隨機信箱"
                            onClick={() => setDeleteTarget(mailbox)}
                          >
                            <IconTrash size={14} />
                          </button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        )}
      </AsyncState>

      <Dialog
        title={detail?.subject || '郵件內容'}
        description={mailboxEmail ? `收件信箱：${mailboxEmail}` : undefined}
        open={detail !== null || detailLoading}
        onClose={() => setDetail(null)}
        wide
      >
        {detailLoading && <p className="hint">讀取中…</p>}
        {detail && (
          <>
            <div className="form-row">
              <p className="hint">寄件人：{detail.from || '—'}</p>
              <p className="hint">收件人：{detail.to || '—'}</p>
              <p className="hint">日期：{formatDateTime(detail.date)}</p>
            </div>
            <pre className="mail-body">{detail.body || '（無內文）'}</pre>
            <div className="form-actions">
              <button className="danger" onClick={() => setDeleteMessageFor(detail)}>
                <IconTrash size={14} />
                刪除郵件
              </button>
              <button onClick={() => setDetail(null)}>關閉</button>
            </div>
          </>
        )}
      </Dialog>

      {deleteMessageFor && (
        <ConfirmDialog
          title="刪除郵件"
          message="這封郵件將從收件匣中永久刪除。"
          open
          busy={deletingMessage}
          onClose={() => setDeleteMessageFor(null)}
          onConfirm={() => void deleteMessage()}
        />
      )}

      {deleteTarget && (
        <ConfirmDialog
          title="刪除隨機信箱"
          message={`將停止追蹤 ${deleteTarget.email}，並嘗試在 iCloud 端刪除這個別名。`}
          confirmLabel="立即刪除"
          open
          busy={deleting}
          onClose={() => setDeleteTarget(null)}
          onConfirm={() => void handleDelete()}
        />
      )}
    </div>
  )
}
