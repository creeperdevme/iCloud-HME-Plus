import type { InboxMessage } from '../api/types'
import { relayNote, senderAddress } from '../utils/sender'

/** 郵件列表的寄件人欄：名稱為主、位址在下，iCloud 轉發位址放在滑鼠提示。 */
export default function SenderCell({ message }: { message: InboxMessage }) {
  const address = senderAddress(message)
  return (
    <td title={relayNote(message)}>
      <span className="cell-strong">{message.from_name || address || '—'}</span>
      {message.from_name && address && <span className="cell-secondary">{address}</span>}
    </td>
  )
}
