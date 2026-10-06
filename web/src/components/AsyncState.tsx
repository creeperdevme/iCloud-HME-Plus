import type { ReactNode } from 'react'
import { IconAlert, IconInboxEmpty } from './icons'

interface AsyncStateProps {
  loading: boolean
  error: string
  empty: boolean
  emptyText?: string
  onRetry: () => void
  children: ReactNode
}

/** 統一的非同步狀態：載入骨架 / 錯誤 + 重試 / 空狀態 / 內容 */
export default function AsyncState({
  loading,
  error,
  empty,
  emptyText = '暫無資料',
  onRetry,
  children,
}: AsyncStateProps) {
  if (loading) {
    return (
      <div className="skeleton" role="status" aria-label="載入中" aria-busy="true">
        <span className="visually-hidden">載入中</span>
        <div className="skeleton-line" style={{ width: '30%' }} />
        <div className="skeleton-line" style={{ width: '85%' }} />
        <div className="skeleton-line" style={{ width: '70%' }} />
        <div className="skeleton-line" style={{ width: '90%' }} />
      </div>
    )
  }

  if (error) {
    return (
      <div className="card">
        <div className="alert alert-error" role="alert">
          <IconAlert size={18} />
          <span>{error}</span>
        </div>
        <div className="form-actions">
          <button onClick={onRetry}>重試</button>
        </div>
      </div>
    )
  }

  if (empty) {
    return (
      <div className="card">
        <p className="empty-state">
          <IconInboxEmpty className="empty-icon" />
          {emptyText}
        </p>
      </div>
    )
  }

  return <>{children}</>
}
