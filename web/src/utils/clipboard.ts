/**
 * 複製文字到剪貼簿。
 * 部分瀏覽器擴充功能會攔截 Clipboard API，因此保留傳統 API 作為降級路徑。
 */
export async function copyText(value: string): Promise<boolean> {
  if (!value) return false

  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(value)
      return true
    }
  } catch {
    // 繼續嘗試傳統複製，避免權限被拒時直接對使用者拋錯。
  }

  return copyWithLegacyApi(value)
}

function copyWithLegacyApi(value: string): boolean {
  const textarea = document.createElement('textarea')
  const previousFocus = document.activeElement as HTMLElement | null
  textarea.value = value
  textarea.setAttribute('readonly', '')
  textarea.setAttribute('aria-hidden', 'true')
  textarea.style.position = 'fixed'
  textarea.style.top = '-9999px'
  textarea.style.left = '-9999px'
  textarea.style.opacity = '0'
  document.body.appendChild(textarea)

  try {
    textarea.focus()
    textarea.select()
    textarea.setSelectionRange(0, value.length)
    if (typeof document.execCommand !== 'function') return false
    return document.execCommand('copy')
  } catch {
    return false
  } finally {
    textarea.remove()
    previousFocus?.focus()
  }
}
