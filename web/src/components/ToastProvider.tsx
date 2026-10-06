import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import { IconCheck, IconCopy } from './icons'
import { copyText } from '../utils/clipboard'

interface ToastContextValue {
  show: (message: string) => void
  showCopyable: (value: string, message?: string) => void
}

const ToastContext = createContext<ToastContextValue | null>(null)

interface ToastItem {
  id: number
  message: string
  copyValue?: string
  duration: number
}

const DEFAULT_DURATION = 4000
const COPYABLE_DURATION = 6000
const HOVER_EXIT_DELAY = 1200

export function useToast(): ToastContextValue {
  const ctx = useContext(ToastContext)
  if (!ctx) {
    throw new Error('useToast 必須在 ToastProvider 內使用')
  }
  return ctx
}

function Toast({ toast, onDismiss }: { toast: ToastItem; onDismiss: (id: number) => void }) {
  const timerRef = useRef<number | null>(null)
  const [copyState, setCopyState] = useState<'idle' | 'copied' | 'failed'>('idle')

  const clearTimer = useCallback(() => {
    if (timerRef.current !== null) {
      window.clearTimeout(timerRef.current)
      timerRef.current = null
    }
  }, [])

  const scheduleDismiss = useCallback(
    (delay: number) => {
      clearTimer()
      timerRef.current = window.setTimeout(() => {
        timerRef.current = null
        onDismiss(toast.id)
      }, delay)
    },
    [clearTimer, onDismiss, toast.id],
  )

  useEffect(() => {
    scheduleDismiss(toast.duration)
    return clearTimer
  }, [clearTimer, scheduleDismiss, toast.duration])

  async function handleCopy() {
    if (!toast.copyValue) return
    if (await copyText(toast.copyValue)) {
      setCopyState('copied')
    } else {
      setCopyState('failed')
    }
  }

  return (
    <div
      className="toast"
      role="status"
      aria-label={toast.copyValue ? `${toast.message}：${toast.copyValue}` : toast.message}
      onMouseEnter={clearTimer}
      onMouseLeave={() => scheduleDismiss(HOVER_EXIT_DELAY)}
      onFocusCapture={clearTimer}
      onBlurCapture={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget as Node | null)) {
          scheduleDismiss(HOVER_EXIT_DELAY)
        }
      }}
    >
      <IconCheck size={16} aria-hidden="true" />
      <div className="toast-content">
        <span>
          {toast.message}
          {toast.copyValue ? '：' : ''}
        </span>
        {toast.copyValue && <code className="toast-copy-value">{toast.copyValue}</code>}
      </div>
      {toast.copyValue && (
        <button
          type="button"
          className="toast-copy"
          onClick={() => void handleCopy()}
          title="複製別名信箱"
          aria-label="複製"
        >
          <IconCopy size={14} />
          {copyState === 'copied' ? '已複製' : copyState === 'failed' ? '複製失敗' : '複製'}
        </button>
      )}
    </div>
  )
}

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<ToastItem[]>([])
  const nextId = useRef(1)

  const enqueue = useCallback((message: string, copyValue?: string) => {
    const id = nextId.current++
    setToasts((prev) => [
      ...prev,
      {
        id,
        message,
        copyValue,
        duration: copyValue ? COPYABLE_DURATION : DEFAULT_DURATION,
      },
    ])
  }, [])

  const show = useCallback((message: string) => enqueue(message), [enqueue])
  const showCopyable = useCallback(
    (value: string, message = '別名已建立') => enqueue(message, value),
    [enqueue],
  )
  const dismiss = useCallback((id: number) => {
    setToasts((prev) => prev.filter((toast) => toast.id !== id))
  }, [])

  const value = useMemo(() => ({ show, showCopyable }), [show, showCopyable])

  return (
    <ToastContext.Provider value={value}>
      {children}
      <div className="toast-region" aria-live="polite">
        {toasts.map((t) => (
          <Toast key={t.id} toast={t} onDismiss={dismiss} />
        ))}
      </div>
    </ToastContext.Provider>
  )
}
