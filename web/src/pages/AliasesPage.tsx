import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { request, ApiError } from '../api/client'
import type { AccountSummary, Alias } from '../api/types'
import AsyncState from '../components/AsyncState'
import CreateAliasDialog from '../components/CreateAliasDialog'
import ConfirmDialog from '../components/ConfirmDialog'
import { useToast } from '../components/ToastProvider'
import { copyText } from '../utils/clipboard'
import {
  IconAliases,
  IconCheck,
  IconChevronDown,
  IconChevronUp,
  IconClock,
  IconInbox,
  IconPlus,
  IconSearch,
  IconTrash,
} from '../components/icons'

type SortDirection = 'asc' | 'desc'
type Filter = 'all' | 'active' | 'inactive'

function parseAliasDate(raw?: string): Date | null {
  const value = raw?.trim()
  if (!value) return null

  // iCloud 可能回傳 ISO 字串，也可能是秒、毫秒或微秒時間戳。
  if (/^[+-]?\d+(?:\.\d+)?$/.test(value)) {
    const numeric = Number(value)
    if (Number.isFinite(numeric)) {
      const magnitude = Math.abs(numeric)
      const milliseconds =
        magnitude < 1e11
          ? numeric * 1000
          : magnitude < 1e14
            ? numeric
            : magnitude < 1e17
              ? numeric / 1000
              : numeric / 1e6
      const timestampDate = new Date(milliseconds)
      if (!Number.isNaN(timestampDate.getTime())) return timestampDate
    }
  }

  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? null : date
}

function formatDate(raw?: string): string {
  const date = parseAliasDate(raw)
  if (!date) return raw?.trim() || '—'
  const pad = (value: number) => String(value).padStart(2, '0')
  return `${date.getFullYear()}/${pad(date.getMonth() + 1)}/${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`
}

function dateTimestamp(raw?: string): number | null {
  return parseAliasDate(raw)?.getTime() ?? null
}

export default function AliasesPage() {
  const [accounts, setAccounts] = useState<AccountSummary[]>([])
  const [accountId, setAccountId] = useState('')
  const [aliases, setAliases] = useState<Alias[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [retryKey, setRetryKey] = useState(0)
  const [search, setSearch] = useState('')
  const [filter, setFilter] = useState<Filter>('all')
  const [sortDirection, setSortDirection] = useState<SortDirection>('desc')
  const [createOpen, setCreateOpen] = useState(false)
  const [confirm, setConfirm] = useState<{
    type: 'deactivate' | 'reactivate' | 'delete'
    alias: Alias
  } | null>(null)
  const [busy, setBusy] = useState(false)
  const [actionError, setActionError] = useState('')

  const [searchParams, setSearchParams] = useSearchParams()
  const { show, showCopyable } = useToast()

  // 載入帳號列表
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
        if (target && (!queryId || !valid)) {
          setSearchParams({ account_id: target }, { replace: true })
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

  // 載入別名列表
  useEffect(() => {
    if (!accountId) return
    let cancelled = false
    request<{ account_id: string; count: number; aliases: Alias[] }>(
      `/api/aliases?account_id=${encodeURIComponent(accountId)}`,
    )
      .then((data) => {
        if (cancelled) return
        setAliases(data.aliases ?? [])
        setError('')
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
  }, [accountId, retryKey])

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase()
    return aliases
      .map((alias, index) => ({ alias, index }))
      .filter(({ alias }) => {
        if (filter === 'active' && !alias.active) return false
        if (filter === 'inactive' && alias.active) return false
        if (!q) return true
        return alias.email.toLowerCase().includes(q) || alias.label.toLowerCase().includes(q)
      })
      .sort((left, right) => {
        const leftTime = dateTimestamp(left.alias.createdAt)
        const rightTime = dateTimestamp(right.alias.createdAt)

        // 沒有有效時間的記錄一律排到最後，避免倒序時跳到最前面。
        if (leftTime === null || rightTime === null) {
          if (leftTime === rightTime) return left.index - right.index
          return leftTime === null ? 1 : -1
        }
        if (leftTime === rightTime) return left.index - right.index
        return sortDirection === 'asc' ? leftTime - rightTime : rightTime - leftTime
      })
      .map(({ alias }) => alias)
  }, [aliases, search, filter, sortDirection])

  const activeCount = aliases.filter((a) => a.active).length

  function handleRetry() {
    setLoading(true)
    setRetryKey((k) => k + 1)
  }

  const copyEmail = useCallback(
    async (email: string) => {
      if (await copyText(email)) {
        show('信箱已複製')
      } else {
        showCopyable(email, '複製失敗，請手動複製')
      }
    },
    [show, showCopyable],
  )

  async function runAction(type: 'deactivate' | 'reactivate' | 'delete') {
    if (!confirm) return
    setBusy(true)
    setActionError('')
    const { alias } = confirm
    try {
      if (type === 'delete') {
        await request(`/api/aliases/${encodeURIComponent(alias.anonymousId)}`, {
          method: 'DELETE',
          body: JSON.stringify({ account_id: accountId }),
        })
        show('別名已刪除')
      } else {
        await request(
          `/api/aliases/${encodeURIComponent(alias.anonymousId)}/${type === 'deactivate' ? 'deactivate' : 'reactivate'}`,
          { method: 'POST', body: JSON.stringify({ account_id: accountId }) },
        )
        show(type === 'deactivate' ? '別名已停用' : '別名已啟用')
      }
      setConfirm(null)
      setRetryKey((k) => k + 1)
    } catch (err) {
      setActionError(err instanceof ApiError ? err.message : '網路連線失敗，請檢查服務狀態')
    } finally {
      setBusy(false)
    }
  }

  function handleCreated(email: string) {
    setCreateOpen(false)
    showCopyable(email)
    setRetryKey((k) => k + 1)
  }

  if (accounts.length === 0 && !loading && !error) {
    return (
      <div className="page">
        <header className="page-head">
          <div>
            <h2 className="page-title">別名管理</h2>
            <p className="page-sub">建立、停用、啟用或刪除 Hide My Email 別名</p>
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

  const confirmTitle =
    confirm?.type === 'delete'
      ? '刪除別名'
      : confirm?.type === 'deactivate'
        ? '停用別名'
        : '啟用別名'

  const confirmLabel =
    confirm?.type === 'delete'
      ? '確認刪除'
      : confirm?.type === 'deactivate'
        ? '確認停用'
        : '確認啟用'

  return (
    <div className="page">
      <header className="page-head">
        <div>
          <h2 className="page-title">別名管理</h2>
          <p className="page-sub">建立、停用、啟用或刪除 Hide My Email 別名</p>
        </div>
        <div className="page-actions">
          <label htmlFor="alias-account" className="visually-hidden">
            選擇帳號
          </label>
          <select
            id="alias-account"
            value={accountId}
            onChange={(e) => {
              setAccountId(e.target.value)
              setSearchParams({ account_id: e.target.value }, { replace: true })
            }}
            style={{ width: 'auto', minWidth: 180 }}
          >
            {accounts.map((a) => (
              <option key={a.id} value={a.id}>
                {a.name}（{a.icloud_email || a.id}）
              </option>
            ))}
          </select>
          <button className="primary" onClick={() => setCreateOpen(true)} disabled={!accountId}>
            <IconPlus size={16} />
            建立別名
          </button>
        </div>
      </header>

      <div className="stat-grid">
        <div className="stat">
          <span className="stat-icon">
            <IconAliases size={20} />
          </span>
          <div className="stat-body">
            <div className="stat-value">{aliases.length}</div>
            <div className="stat-label">別名總數</div>
          </div>
        </div>
        <div className="stat">
          <span className="stat-icon is-success">
            <IconCheck size={20} />
          </span>
          <div className="stat-body">
            <div className="stat-value">{activeCount}</div>
            <div className="stat-label">已啟用</div>
          </div>
        </div>
        <div className="stat">
          <span className="stat-icon is-warning">
            <IconClock size={20} />
          </span>
          <div className="stat-body">
            <div className="stat-value">{aliases.length - activeCount}</div>
            <div className="stat-label">已停用</div>
          </div>
        </div>
      </div>

      <div className="card">
        <div className="toolbar">
          <div className="search-wrap">
            <label htmlFor="alias-search" className="visually-hidden">
              搜尋別名
            </label>
            <input
              id="alias-search"
              type="search"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="以信箱或標籤搜尋"
            />
            <span className="search-icon" aria-hidden="true">
              <IconSearch size={16} />
            </span>
          </div>
          <div className="form-field" style={{ flex: '0 0 160px' }}>
            <label htmlFor="alias-filter">狀態</label>
            <select
              id="alias-filter"
              value={filter}
              onChange={(e) => setFilter(e.target.value as Filter)}
            >
              <option value="all">全部</option>
              <option value="active">已啟用</option>
              <option value="inactive">已停用</option>
            </select>
          </div>
          <div className="toolbar-action">
            <button onClick={handleRetry}>重新整理</button>
          </div>
        </div>
      </div>

      {actionError && (
        <div className="alert alert-error" role="alert">
          <span>{actionError}</span>
        </div>
      )}

      <AsyncState
        loading={loading}
        error={error}
        empty={filtered.length === 0}
        emptyText={aliases.length === 0 ? '這個帳號還沒有任何別名。' : '沒有符合條件的別名。'}
        onRetry={handleRetry}
      >
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>信箱</th>
                <th>標籤</th>
                <th>狀態</th>
                <th aria-sort={sortDirection === 'asc' ? 'ascending' : 'descending'}>
                  <button
                    type="button"
                    className="table-sort-button"
                    onClick={() =>
                      setSortDirection((direction) => (direction === 'asc' ? 'desc' : 'asc'))
                    }
                    aria-label={`建立時間排序：目前${sortDirection === 'asc' ? '正序' : '倒序'}，點擊切換為${sortDirection === 'asc' ? '倒序' : '正序'}`}
                    title="點擊切換建立時間排序"
                  >
                    <span>建立時間</span>
                    {sortDirection === 'asc' ? (
                      <IconChevronUp size={14} />
                    ) : (
                      <IconChevronDown size={14} />
                    )}
                  </button>
                </th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((alias) => (
                <tr key={alias.anonymousId}>
                  <td>
                    <button
                      type="button"
                      className="link-button"
                      onClick={() => void copyEmail(alias.email)}
                      title="點擊複製信箱"
                    >
                      {alias.email}
                    </button>
                  </td>
                  <td>{alias.label || '—'}</td>
                  <td>
                    <span className={alias.active ? 'badge badge-active' : 'badge badge-neutral'}>
                      {alias.active ? <IconCheck size={12} /> : <IconClock size={12} />}
                      {alias.active ? '已啟用' : '已停用'}
                    </span>
                  </td>
                  <td>{formatDate(alias.createdAt)}</td>
                  <td>
                    <div className="row-actions">
                      {alias.active ? (
                        <button
                          className="small"
                          disabled={busy}
                          onClick={() => setConfirm({ type: 'deactivate', alias })}
                        >
                          停用
                        </button>
                      ) : (
                        <button
                          className="small"
                          disabled={busy}
                          onClick={() => setConfirm({ type: 'reactivate', alias })}
                        >
                          啟用
                        </button>
                      )}
                      <Link
                        className="btn small"
                        to={`/inbox?account_id=${encodeURIComponent(accountId)}&alias=${encodeURIComponent(alias.email)}`}
                        title="查看此別名的收件匣"
                      >
                        <IconInbox size={13} />
                        收件匣
                      </Link>
                      <button
                        className="icon-button danger"
                        aria-label={`刪除別名 ${alias.email}`}
                        title="刪除別名"
                        disabled={busy}
                        onClick={() => setConfirm({ type: 'delete', alias })}
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

      <CreateAliasDialog
        accountId={accountId}
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        onCreated={handleCreated}
      />

      {confirm && (
        <ConfirmDialog
          title={confirmTitle}
          message={
            confirm.type === 'delete'
              ? `將刪除別名 ${confirm.alias.email}。此操作無法復原，也不會影響 Apple 帳號本身。`
              : confirm.type === 'deactivate'
                ? `將停用別名 ${confirm.alias.email}，之後該信箱將不再收信。`
                : `將重新啟用別名 ${confirm.alias.email}。`
          }
          confirmLabel={confirmLabel}
          requireText={confirm.type === 'delete' ? confirm.alias.email : undefined}
          requireLabel={confirm.type === 'delete' ? '輸入完整信箱以確認' : undefined}
          open
          busy={busy}
          onClose={() => setConfirm(null)}
          onConfirm={() => void runAction(confirm.type)}
        />
      )}
    </div>
  )
}
