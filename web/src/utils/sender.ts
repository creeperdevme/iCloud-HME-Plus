/**
 * 寄件人顯示用的純函式。
 *
 * 經隱藏郵件地址轉發的信，from 是 iCloud 改寫後的位址（例如
 * noreply_at_tm_openai_com_xxx@icloud.com），original_from 是後端推回的
 * 原始寄件人。畫面以原始寄件人為主，改寫後的位址只當補充資訊：
 * 回信要回到它才不會暴露真實信箱。
 */

import type { InboxMessage } from '../api/types'

type Sender = Pick<InboxMessage, 'from' | 'from_name' | 'original_from'>

/** 要顯示的寄件人位址：能還原就用原始寄件人。 */
export function senderAddress(m: Sender): string {
  return m.original_from || m.from
}

/**
 * 寄件人名稱；沒有名稱的寄件人常把位址本身當名稱
 * （例如 "no-reply@qoder.com"），這種情況視為沒有名稱，避免重複顯示。
 */
export function senderName(m: Sender): string {
  const name = m.from_name?.trim() ?? ''
  return name.toLowerCase() === senderAddress(m).toLowerCase() ? '' : name
}

/** 單行顯示：「Name <address>」，缺哪個就只顯示另一個。 */
export function formatSender(m: Sender): string {
  const address = senderAddress(m)
  const name = senderName(m)
  if (name && address) return `${name} <${address}>`
  return name || address || '—'
}

/** 有還原出原始寄件人時，回傳 iCloud 轉發位址的說明文字。 */
export function relayNote(m: Sender): string | undefined {
  return m.original_from ? `經 iCloud 轉發：${m.from}` : undefined
}
