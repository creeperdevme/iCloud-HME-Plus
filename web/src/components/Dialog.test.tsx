import { useState } from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import Dialog from './Dialog'

/**
 * Dialog 的焦點回歸測試。
 *
 * 這裡刻意用 userEvent.type（逐字輸入）而不是 fireEvent.change（一次送出完整值），
 * 因為先前有個 bug 只有在「每次 re-render」時才會顯現：焦點鎖定 effect 把每次渲染
 * 都新建的 onClose 放進依賴陣列，導致每敲一個字就重建 effect，cleanup 把焦點還給
 * 觸發元素、setup 又把焦點搶到對話框內第一顆按鈕，使用者實際上只能輸入一個字元。
 * fireEvent.change 一次送出完整值，永遠不會踩到這個 bug。
 */
function Harness({ onClose }: { onClose: () => void }) {
  const [value, setValue] = useState('')
  const [open, setOpen] = useState(true)
  return (
    <>
      <button onClick={() => setOpen(true)}>開啟對話框</button>
      <Dialog
        title="測試對話框"
        open={open}
        // 每次渲染都是新的函式：這正是觸發上述 bug 的條件。
        onClose={() => {
          setOpen(false)
          onClose()
        }}
      >
        <label htmlFor="probe">密碼</label>
        <input id="probe" value={value} onChange={(e) => setValue(e.target.value)} />
      </Dialog>
    </>
  )
}

describe('Dialog 焦點行為', () => {
  it('連續輸入多個字元時焦點不會被搶走，輸入內容完整累積', async () => {
    const user = userEvent.setup()
    render(<Harness onClose={() => undefined} />)

    const input = screen.getByLabelText('密碼')
    await user.type(input, 'abcdef')

    expect(input).toHaveValue('abcdef')
    expect(document.activeElement).toBe(input)
  })

  it('開啟時焦點進入對話框，關閉後回到觸發元素', async () => {
    const user = userEvent.setup()
    const onClose = vi.fn()
    render(<Harness onClose={onClose} />)

    // 開啟時焦點在對話框內（第一顆可聚焦元素是關閉按鈕）
    expect(document.activeElement).toBe(screen.getByLabelText('關閉對話框'))

    // 先手動把焦點移到輸入框並輸入，再按 Escape 關閉
    await user.click(screen.getByLabelText('密碼'))
    await user.keyboard('{Escape}')

    expect(onClose).toHaveBeenCalledTimes(1)
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('Escape 關閉對話框，且不會因多次渲染而重複觸發', async () => {
    const user = userEvent.setup()
    const onClose = vi.fn()
    function Static() {
      return (
        <Dialog title="靜態" open onClose={onClose}>
          <input aria-label="欄位" />
        </Dialog>
      )
    }
    render(<Static />)

    await user.keyboard('{Escape}')
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('點擊遮罩關閉，點擊對話框內部不關閉', async () => {
    const user = userEvent.setup()
    const onClose = vi.fn()
    render(<Harness onClose={onClose} />)

    await user.click(screen.getByText('測試對話框'))
    expect(onClose).not.toHaveBeenCalled()

    const backdrop = document.querySelector('.dialog-backdrop')
    expect(backdrop).not.toBeNull()
    await user.click(backdrop as Element)
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('遮罩不是 button，避免按鈕巢狀造成無效 HTML', () => {
    render(<Harness onClose={() => undefined} />)
    const backdrop = document.querySelector('.dialog-backdrop')
    expect(backdrop?.tagName).toBe('DIV')
    expect(backdrop?.getAttribute('role')).toBe('presentation')
    // 對話框內的關閉按鈕必須是遮罩的子節點，而不是包住對話框的按鈕
    expect(backdrop?.querySelector('button[aria-label="關閉對話框"]')).not.toBeNull()
  })
})
