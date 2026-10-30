import { describe, expect, it } from 'vitest'
import { formatSender, relayNote, senderAddress } from './sender'

const relay = 'noreply_at_tm_openai_com_w52g946f86yy7f_d9hy2897@icloud.com'

describe('sender', () => {
  it('能還原時以原始寄件人為主', () => {
    const m = { from: relay, from_name: 'OpenAI', original_from: 'noreply@tm.openai.com' }
    expect(senderAddress(m)).toBe('noreply@tm.openai.com')
    expect(formatSender(m)).toBe('OpenAI <noreply@tm.openai.com>')
    expect(relayNote(m)).toBe(`經 iCloud 轉發：${relay}`)
  })

  it('一般寄件人照原樣顯示且沒有轉發說明', () => {
    expect(formatSender({ from: 'a@example.com' })).toBe('a@example.com')
    expect(formatSender({ from: 'a@example.com', from_name: 'A' })).toBe('A <a@example.com>')
    expect(relayNote({ from: 'a@example.com' })).toBeUndefined()
  })

  it('缺少寄件人時顯示破折號', () => {
    expect(formatSender({ from: '' })).toBe('—')
  })
})
