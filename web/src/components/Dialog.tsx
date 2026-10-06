import { useEffect, useRef, type ReactNode } from 'react'

interface DialogProps {
  title: string
  description?: string
  open: boolean
  onClose: () => void
  wide?: boolean
  children: ReactNode
}

/**
 * 可存取的對話框：Escape 關閉、Tab 焦點鎖定、關閉後焦點回到觸發元素。
 *
 * 遮罩使用 div（而非 button）承載，避免對話框內容的按鈕成為 button 的
 * 後代而產生無效的 HTML 巢狀結構。
 */
export default function Dialog({
  title,
  description,
  open,
  onClose,
  wide = false,
  children,
}: DialogProps) {
  const ref = useRef<HTMLDivElement>(null)
  const lastFocused = useRef<Element | null>(null)
  // onClose 幾乎都是呼叫端每次渲染新建的 inline 箭頭函式，因此不能放進下面
  // 焦點鎖定 effect 的依賴陣列：那會讓 effect 在每次輸入（每次 re-render）都
  // 重建，cleanup 先把焦點還給觸發元素、setup 再把焦點搶到對話框內第一顆
  // 按鈕，導致使用者每敲一個字就掉焦點。改用 ref 保存最新回呼。
  const onCloseRef = useRef(onClose)

  useEffect(() => {
    onCloseRef.current = onClose
  }, [onClose])

  useEffect(() => {
    if (!open) return
    lastFocused.current = document.activeElement
    const node = ref.current
    if (!node) return
    const focusables = () =>
      Array.from(
        node.querySelectorAll<HTMLElement>(
          'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])',
        ),
      ).filter((el) => !el.hasAttribute('disabled'))
    focusables()[0]?.focus()

    const handleKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.stopPropagation()
        onCloseRef.current()
        return
      }
      if (e.key !== 'Tab') return
      const items = focusables()
      if (items.length === 0) return
      const first = items[0]
      const last = items[items.length - 1]
      if (e.shiftKey && document.activeElement === first) {
        e.preventDefault()
        last.focus()
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault()
        first.focus()
      }
    }
    document.addEventListener('keydown', handleKey)
    return () => {
      document.removeEventListener('keydown', handleKey)
      if (lastFocused.current instanceof HTMLElement) {
        lastFocused.current.focus()
      }
    }
    // 只在開關狀態改變時建立/拆除監聽與焦點，不可依賴 onClose（見上方說明）。
  }, [open])

  if (!open) return null

  return (
    // 遮罩點擊關閉只是滑鼠便利性：鍵盤使用者改用 Escape（見上方 keydown 處理），
    // 對話框內也有一顆「關閉對話框」按鈕，因此不需要再額外提供鍵盤事件。
    // 遮罩本身以 role="presentation" 移出無障礙樹，避免出現無語意的互動元素。
    <div
      className="dialog-backdrop"
      role="presentation"
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose()
      }}
    >
      <div
        ref={ref}
        className={wide ? 'dialog is-wide' : 'dialog'}
        role="dialog"
        aria-modal="true"
        aria-label={title}
      >
        <div className="dialog-head">
          <div>
            <h3 className="dialog-title">{title}</h3>
            {description && <p className="dialog-desc">{description}</p>}
          </div>
          <button
            type="button"
            className="icon-button"
            aria-label="關閉對話框"
            title="關閉"
            onClick={onClose}
          >
            <svg
              width="16"
              height="16"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              aria-hidden="true"
            >
              <path d="M18 6 6 18" />
              <path d="m6 6 12 12" />
            </svg>
          </button>
        </div>
        {children}
      </div>
    </div>
  )
}
