import { useEffect, useMemo, useRef, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { request, ApiError } from '../api/client'
import type {
  AccountSummary,
  Alias,
  FullMessage,
  InboxMessage,
  InboxResult,
} from '../api/types'
import AsyncState from '../components/AsyncState'
import Dialog from '../components/Dialog'
import ConfirmDialog from '../components/ConfirmDialog'
import { useToast } from '../components/ToastProvider'
import { IconInbox, IconMail, IconRefresh, IconTrash } from '../components/icons'

function formatDate(raw: string): string {
  const d = new Date(raw)
  if (Number.isNaN(d.getTime())) return raw
  return new Intl.DateTimeFormat('zh-TW', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(d)
}

export default function InboxPage() {
  const [accounts, setAccounts] = useState<AccountSummary[]>([])
  const [aliases, setAliases] = useState<Alias[]>([])
  const [accountId, setAccountId] = useState('')
  const [alias, setAlias] = useState('')
  const [limit, setLimit] = useState(20)
  const [days, setDays] = useState(7)

  const [result, setResult] = useState<InboxResult | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [retryKey, setRetryKey] = useState(0)
  const [detail, setDetail] = useState<FullMessage | null>(null)
  const [detailLoading, setDetailLoading] = useState(false)
  const [deleteFor, setDeleteFor] = useState<InboxMessage | null>(null)
  const [deleting, setDeleting] = useState(false)

  const [searchParams, setSearchParams] = useSearchParams()
  const abortRef = useRef<AbortController | null>(null)
  const { show } = useToast()

  async function openMessage(message: InboxMessage) {
    setDetailLoading(true)
    try {
      const data = await request<FullMessage>(
        `/api/inbox/${encodeURIComponent(message.id)}?account_id=${encodeURIComponent(accountId)}`,
      )
      setDetail(data)
    } catch (err) {
      show(err instanceof ApiError ? err.message : '讀取郵件內容失敗')
    } finally {
      setDetailLoading(false)
    }
  }

  async function deleteMessage() {
    if (!deleteFor) return
    setDeleting(true)
    try {
      await request(
        `/api/inbox/${encodeURIComponent(deleteFor.id)}?account_id=${encodeURIComponent(accountId)}`,
        { method: 'DELETE' },
      )
      setDeleteFor(null)
      setDetail(null)
      show('郵件已刪除')
      setRetryKey((key) => key + 1)
    } catch (err) {
      show(err instanceof ApiError ? err.message : '刪除郵件失敗')
    } finally {
      setDeleting(false)
    }
  }

  // 載入帳號列表並初始化篩選狀態
  useEffect(() => {
    let cancelled = false
    request<AccountSummary[]>('/api/accounts')
      .then((data) => {
        if (cancelled) return
        setAccounts(data)
        const queryId = searchParams.get('account_id')
        const valid = data.find((a) => a.id === queryId)
        const target = valid ? valid.id : (data[0]?.id ?? '')
        setAccountId(target)
        if (target) {
          const next: Record<string, string> = { account_id: target }
          const qAlias = searchParams.get('alias')
          if (qAlias) {
            setAlias(qAlias)
            next.alias = qAlias
          }
          const qLimit = searchParams.get('limit')
          if (qLimit) next.limit = qLimit
          const qDays = searchParams.get('days')
          if (qDays) next.days = qDays
          setSearchParams(next, { replace: true })
        }
      })
      .catch((err) => {
        if (cancelled) return
        setError(err instanceof ApiError ? err.message : '網路連線失敗，請檢查服務狀態')
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // 帳號變更時載入別名列表（供篩選）
  useEffect(() => {
    if (!accountId) return
    let cancelled = false
    request<{ account_id: string; count: number; aliases: Alias[] }>(
      `/api/aliases?account_id=${encodeURIComponent(accountId)}`,
    )
      .then((data) => {
        if (cancelled) return
        setAliases(data.aliases ?? [])
      })
      .catch(() => {
        if (cancelled) return
        setAliases([])
      })
    return () => {
      cancelled = true
    }
  }, [accountId])

  // 查詢收件匣；帳號變更時清空舊郵件並中止舊請求
  useEffect(() => {
    if (!accountId) return
    abortRef.current?.abort()
    const controller = new AbortController()
    abortRef.current = controller
    let cancelled = false
    const params = new URLSearchParams({ account_id: accountId })
    if (alias) params.set('alias', alias)
    params.set('limit', String(limit))
    params.set('days', String(days))
    request<InboxResult>(`/api/inbox?${params.toString()}`, {
      signal: controller.signal,
    })
      .then((data) => {
        if (cancelled) return
        setResult(data)
        setError('')
      })
      .catch((err) => {
        if (cancelled || (err instanceof ApiError && err.status === 0)) return
        setError(err instanceof ApiError ? err.message : '網路連線失敗，請檢查服務狀態')
        setResult(null)
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
      controller.abort()
    }
  }, [accountId, alias, limit, days, retryKey])

  const messages = useMemo(() => result?.messages ?? [], [result])

  function handleSearch() {
    const next: Record<string, string> = { account_id: accountId }
    if (alias) next.alias = alias
    next.limit = String(limit)
    next.days = String(days)
    setSearchParams(next, { replace: true })
    setLoading(true)
    setRetryKey((k) => k + 1)
  }

  function handleAccountChange(id: string) {
    setAccountId(id)
    setAlias('')
    setResult(null)
    setSearchParams({ account_id: id }, { replace: true })
  }

  const methodText = result?.method === 'imap' ? 'IMAP' : 'Web API'

  if (accounts.length === 0 && !loading && !error) {
    return (
      <div className="page">
        <header className="page-head">
          <div>
            <h2 className="page-title">收件匣</h2>
            <p className="page-sub">讀取寄到隱藏別名的郵件</p>
          </div>
        </header>
        <div className="card">
          <p className="empty-state">
            還沒有任何帳號，請先到 <Link to="/accounts">帳號管理</Link> 新增。
          </p>
        </div>
      </div>
    )
  }

  return (
    <div className="page">
      <header className="page-head">
        <div>
          <h2 className="page-title">收件匣</h2>
          <p className="page-sub">
            讀取寄到隱藏別名的郵件（只顯示純文字摘要）
            {result && ` · 目前透過 ${methodText} 讀取`}
          </p>
        </div>
        <div className="page-actions">
          <button onClick={handleSearch} disabled={loading || !accountId}>
            <IconRefresh size={16} />
            重新整理
          </button>
        </div>
      </header>

      <div className="card">
        <div className="toolbar">
          <div className="form-field">
            <label htmlFor="inbox-account">帳號</label>
            <select
              id="inbox-account"
              value={accountId}
              onChange={(e) => handleAccountChange(e.target.value)}
            >
              {accounts.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name}
                </option>
              ))}
            </select>
          </div>
          <div className="form-field">
            <label htmlFor="inbox-alias">別名</label>
            <select id="inbox-alias" value={alias} onChange={(e) => setAlias(e.target.value)}>
              <option value="">全部別名</option>
              {aliases.map((a) => (
                <option key={a.anonymousId} value={a.email}>
                  {a.email}
                </option>
              ))}
            </select>
          </div>
          <div className="form-field" style={{ flex: '0 0 130px' }}>
            <label htmlFor="inbox-limit">筆數上限</label>
            <input
              id="inbox-limit"
              type="number"
              min="1"
              max="100"
              value={limit}
              onChange={(e) => setLimit(Number(e.target.value) || 20)}
            />
          </div>
          <div className="form-field" style={{ flex: '0 0 130px' }}>
            <label htmlFor="inbox-days">最近天數</label>
            <input
              id="inbox-days"
              type="number"
              min="1"
              max="365"
              value={days}
              onChange={(e) => setDays(Number(e.target.value) || 7)}
            />
          </div>
          <div className="toolbar-action">
            <button className="primary" onClick={handleSearch} disabled={loading || !accountId}>
              套用條件
            </button>
          </div>
        </div>
      </div>

      <AsyncState
        loading={loading}
        error={error}
        empty={messages.length === 0}
        emptyText="這段期間沒有收到郵件。"
        onRetry={handleSearch}
      >
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>寄件人</th>
                <th>主旨</th>
                <th>收件別名</th>
                <th>日期</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              {messages.map((message) => (
                <tr key={message.id}>
                  <td>
                    <span className="cell-strong">{message.from || '—'}</span>
                  </td>
                  <td>
                    <div className="preview-cell">
                      <span className="cell-strong">{message.subject || '(無主旨)'}</span>
                    </div>
                    {message.preview && <span className="cell-secondary">{message.preview}</span>}
                  </td>
                  <td className="cell-mono">{message.to || '—'}</td>
                  <td>{formatDate(message.date)}</td>
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
                        onClick={() => setDeleteFor(message)}
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

      <Dialog
        title={detail?.subject || '郵件內容'}
        description={`透過 ${methodText} 讀取`}
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
              <p className="hint">日期：{formatDate(detail.date)}</p>
            </div>
            <pre className="mail-body">{detail.body || '（無內文）'}</pre>
            <div className="form-actions">
              <button className="danger" onClick={() => setDeleteFor(detail)}>
                <IconTrash size={14} />
                刪除郵件
              </button>
              <button onClick={() => setDetail(null)}>關閉</button>
            </div>
          </>
        )}
      </Dialog>

      {deleteFor && (
        <ConfirmDialog
          title="刪除郵件"
          message="這封郵件將從收件匣中永久刪除。"
          open
          busy={deleting}
          onClose={() => setDeleteFor(null)}
          onConfirm={() => void deleteMessage()}
        />
      )}

      {messages.length > 0 && (
        <p className="hint" style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          <IconInbox size={14} />
          共 {result?.count ?? messages.length} 封郵件
        </p>
      )}
    </div>
  )
}
