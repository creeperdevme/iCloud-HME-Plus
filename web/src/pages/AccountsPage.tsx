import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { request, ApiError } from '../api/client'
import type { AccountSummary } from '../api/types'
import AsyncState from '../components/AsyncState'
import AccountFormDialog from '../components/AccountFormDialog'
import CookieDialog from '../components/CookieDialog'
import ICloudLoginDialog from '../components/ICloudLoginDialog'
import AppPasswordDialog from '../components/AppPasswordDialog'
import ProxyDialog from '../components/ProxyDialog'
import MailboxDialog from '../components/MailboxDialog'
import ConfirmDialog from '../components/ConfirmDialog'
import { useToast } from '../components/ToastProvider'
import {
  IconAccounts,
  IconAliases,
  IconAlert,
  IconCheck,
  IconClock,
  IconEdit,
  IconInbox,
  IconKey,
  IconPlus,
  IconTrash,
} from '../components/icons'

const statusMeta: Record<string, { text: string; badge: string; icon: typeof IconCheck }> = {
  active: { text: '正常', badge: 'badge badge-active', icon: IconCheck },
  pending: { text: '待設定', badge: 'badge badge-pending', icon: IconClock },
  error: { text: '異常', badge: 'badge badge-error', icon: IconAlert },
}

function StatusBadge({ status }: { status: string }) {
  const meta = statusMeta[status] ?? {
    text: status,
    badge: 'badge badge-neutral',
    icon: IconClock,
  }
  const Icon = meta.icon
  return (
    <span className={meta.badge}>
      <Icon size={12} />
      {meta.text}
    </span>
  )
}

function credList(acc: AccountSummary): string[] {
  const parts: string[] = []
  if (acc.has_cookies) parts.push('Cookie')
  if (acc.has_app_password) parts.push('App 密碼')
  if (acc.mailbox) parts.push(`收件匣 ${acc.mailbox.email}`)
  if (acc.has_proxy) parts.push('代理')
  return parts
}

export default function AccountsPage() {
  const [accounts, setAccounts] = useState<AccountSummary[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [retryKey, setRetryKey] = useState(0)

  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState<AccountSummary | null>(null)
  const [cookieFor, setCookieFor] = useState<AccountSummary | null>(null)
  const [loginFor, setLoginFor] = useState<AccountSummary | null>(null)
  const [appPwdFor, setAppPwdFor] = useState<AccountSummary | null>(null)
  const [proxyFor, setProxyFor] = useState<AccountSummary | null>(null)
  const [mailboxFor, setMailboxFor] = useState<AccountSummary | null>(null)
  const [deleteFor, setDeleteFor] = useState<AccountSummary | null>(null)
  const [deleting, setDeleting] = useState(false)

  const { show } = useToast()

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const data = await request<AccountSummary[]>('/api/accounts')
      setAccounts(data)
      setError('')
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '網路連線失敗，請檢查服務狀態')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    let cancelled = false
    request<AccountSummary[]>('/api/accounts')
      .then((data) => {
        if (cancelled) return
        setAccounts(data)
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
  }, [retryKey])

  function handleRetry() {
    setLoading(true)
    setRetryKey((k) => k + 1)
  }

  async function handleDelete() {
    if (!deleteFor) return
    setDeleting(true)
    try {
      await request(`/api/accounts/${deleteFor.id}`, { method: 'DELETE' })
      setDeleteFor(null)
      show('帳號已刪除')
      void load()
    } catch (err) {
      show(err instanceof ApiError ? err.message : '刪除失敗')
    } finally {
      setDeleting(false)
    }
  }

  const activeCount = accounts.filter((a) => a.status === 'active').length
  const attentionCount = accounts.filter((a) => a.status !== 'active').length
  const aliasTotal = accounts.reduce((sum, a) => sum + a.alias_total, 0)

  return (
    <div className="page">
      <header className="page-head">
        <div>
          <h2 className="page-title">帳號管理</h2>
          <p className="page-sub">管理 iCloud 帳號、Cookie 與登入憑證</p>
        </div>
        <div className="page-actions">
          <button onClick={handleRetry} title="重新載入帳號列表">
            重新整理
          </button>
          <button
            className="primary"
            onClick={() => {
              setEditing(null)
              setFormOpen(true)
            }}
          >
            <IconPlus size={16} />
            新增帳號
          </button>
        </div>
      </header>

      <div className="stat-grid">
        <div className="stat">
          <span className="stat-icon">
            <IconAccounts size={20} />
          </span>
          <div className="stat-body">
            <div className="stat-value">{accounts.length}</div>
            <div className="stat-label">帳號總數</div>
          </div>
        </div>
        <div className="stat">
          <span className="stat-icon is-success">
            <IconCheck size={20} />
          </span>
          <div className="stat-body">
            <div className="stat-value">{activeCount}</div>
            <div className="stat-label">狀態正常</div>
          </div>
        </div>
        <div className="stat">
          <span className={attentionCount > 0 ? 'stat-icon is-warning' : 'stat-icon'}>
            <IconAlert size={20} />
          </span>
          <div className="stat-body">
            <div className="stat-value">{attentionCount}</div>
            <div className="stat-label">待設定 / 異常</div>
          </div>
        </div>
        <div className="stat">
          <span className="stat-icon">
            <IconAliases size={20} />
          </span>
          <div className="stat-body">
            <div className="stat-value">{aliasTotal}</div>
            <div className="stat-label">別名總數</div>
          </div>
        </div>
      </div>

      <AsyncState
        loading={loading}
        error={error}
        empty={accounts.length === 0}
        emptyText="還沒有任何帳號，點右上角「新增帳號」開始。"
        onRetry={handleRetry}
      >
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>帳號</th>
                <th>狀態</th>
                <th>別名</th>
                <th>憑證</th>
                <th>最近驗證</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              {accounts.map((acc) => {
                const creds = credList(acc)
                return (
                  <tr key={acc.id}>
                    <td>
                      <span className="cell-strong">{acc.name}</span>
                      <span className="cell-secondary">
                        {acc.icloud_email || acc.real_email || '—'}
                      </span>
                      <span className="cell-secondary cell-mono">{acc.id}</span>
                    </td>
                    <td>
                      <StatusBadge status={acc.status} />
                      {acc.status_message && (
                        <span className="cell-secondary">{acc.status_message}</span>
                      )}
                    </td>
                    <td>
                      <span className="cell-strong">
                        {acc.alias_active} / {acc.alias_total}
                      </span>
                      <span className="cell-secondary">啟用 / 總數</span>
                    </td>
                    <td>
                      {creds.length > 0 ? (
                        <div className="row-actions">
                          {creds.map((text) => (
                            <span className="badge badge-neutral" key={text}>
                              {text}
                            </span>
                          ))}
                        </div>
                      ) : (
                        <span className="hint">尚未設定</span>
                      )}
                    </td>
                    <td>{acc.last_validated || '—'}</td>
                    <td>
                      <div className="row-actions">
                        <button
                          className="small"
                          onClick={() => {
                            setEditing(acc)
                            setFormOpen(true)
                          }}
                        >
                          <IconEdit size={13} />
                          編輯
                        </button>
                        <button className="small" onClick={() => setCookieFor(acc)}>
                          更新 Cookie
                        </button>
                        <button className="small" onClick={() => setLoginFor(acc)}>
                          <IconKey size={13} />
                          iCloud 登入
                        </button>
                        <button className="small" onClick={() => setAppPwdFor(acc)}>
                          App 密碼
                        </button>
                        <button className="small" onClick={() => setMailboxFor(acc)}>
                          收件信箱
                        </button>
                        <button className="small" onClick={() => setProxyFor(acc)}>
                          代理
                        </button>
                        <Link className="btn small" to={`/aliases?account_id=${acc.id}`}>
                          <IconAliases size={13} />
                          別名
                        </Link>
                        <Link className="btn small" to={`/inbox?account_id=${acc.id}`}>
                          <IconInbox size={13} />
                          收件匣
                        </Link>
                        <button
                          className="icon-button danger"
                          aria-label={`刪除帳號 ${acc.name}`}
                          title="刪除帳號"
                          onClick={() => setDeleteFor(acc)}
                        >
                          <IconTrash size={14} />
                        </button>
                      </div>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      </AsyncState>

      <AccountFormDialog
        open={formOpen}
        onClose={() => setFormOpen(false)}
        onSaved={() => {
          setFormOpen(false)
          show('帳號已儲存')
          void load()
        }}
        editing={
          editing
            ? {
                id: editing.id,
                name: editing.name,
                icloudEmail: editing.icloud_email,
                host: editing.host,
              }
            : null
        }
      />

      {cookieFor && (
        <CookieDialog
          accountId={cookieFor.id}
          open
          onClose={() => setCookieFor(null)}
          onSaved={() => {
            setCookieFor(null)
            show('Cookie 已更新')
            void load()
          }}
        />
      )}

      {loginFor && (
        <ICloudLoginDialog
          accountId={loginFor.id}
          open
          onClose={() => setLoginFor(null)}
          onSaved={() => {
            setLoginFor(null)
            show('登入成功，Cookie 已更新')
            void load()
          }}
        />
      )}

      {appPwdFor && (
        <AppPasswordDialog
          accountId={appPwdFor.id}
          open
          onClose={() => setAppPwdFor(null)}
          onSaved={() => {
            setAppPwdFor(null)
            show('App 專用密碼已設定')
            void load()
          }}
        />
      )}

      {proxyFor && (
        <ProxyDialog
          accountId={proxyFor.id}
          open
          onClose={() => setProxyFor(null)}
          onSaved={() => {
            setProxyFor(null)
            show('代理已更新')
            void load()
          }}
        />
      )}

      {mailboxFor && (
        <MailboxDialog
          accountId={mailboxFor.id}
          current={mailboxFor.mailbox}
          open
          onClose={() => setMailboxFor(null)}
          onSaved={() => {
            setMailboxFor(null)
            show('收件信箱已接入')
            void load()
          }}
        />
      )}

      {deleteFor && (
        <ConfirmDialog
          title="刪除帳號"
          message={`將移除本機的帳號設定「${deleteFor.name}」，不會影響 Apple 帳號本身。`}
          requireText={deleteFor.name}
          requireLabel="輸入帳號名稱以確認"
          open
          busy={deleting}
          onClose={() => setDeleteFor(null)}
          onConfirm={() => void handleDelete()}
        />
      )}
    </div>
  )
}
