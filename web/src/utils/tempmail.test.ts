import { describe, expect, it } from 'vitest'
import type { AccountSummary, TempMailbox } from '../api/types'
import {
  ZERO_TIME_PREFIX,
  expiresAtMs,
  formatRemaining,
  isUsableAccount,
  latestMailbox,
} from './tempmail'

function mailbox(overrides: Partial<TempMailbox> = {}): TempMailbox {
  return {
    id: 'temp_1',
    email: 'temp-jade-reef-4821@icloud.com',
    account_id: 'acc_1',
    label: 'temp-jade-reef-4821',
    created_at: '2026-10-06T10:00:00Z',
    expires_at: '2026-10-07T10:00:00Z',
    keep: false,
    ...overrides,
  }
}

function account(overrides: Partial<AccountSummary> = {}): AccountSummary {
  return {
    id: 'acc_1',
    name: '主號',
    real_email: 'a@example.com',
    icloud_email: 'a@icloud.com',
    host: 'icloud.com',
    status: 'active',
    alias_total: 0,
    alias_active: 0,
    has_cookies: false,
    has_app_password: false,
    has_proxy: false,
    last_validated: '',
    created_at: '2026-10-01T00:00:00Z',
    ...overrides,
  }
}

describe('formatRemaining', () => {
  it('24 小時剛建立好時只顯示「1 天」，不會出現 0 小時 0 分', () => {
    expect(formatRemaining(24 * 3600 * 1000)).toBe('1 天')
  })

  it('超過一天時補上小時', () => {
    expect(formatRemaining((26 * 3600 + 30 * 60) * 1000)).toBe('1 天 2 小時')
  })

  it('1 小時以上以「小時 + 分」顯示（規格範例）', () => {
    expect(formatRemaining((23 * 3600 + 58 * 60) * 1000)).toBe('23 小時 58 分')
    expect(formatRemaining(3600 * 1000)).toBe('1 小時 0 分')
  })

  it('不足 1 小時時附上秒數，讓畫面每秒都有變化', () => {
    expect(formatRemaining(59 * 60 * 1000 + 30_000)).toBe('59 分 30 秒')
    expect(formatRemaining(61_000)).toBe('1 分 1 秒')
    expect(formatRemaining(5_000)).toBe('5 秒')
  })

  it('到期或已過期時不顯示負數', () => {
    expect(formatRemaining(0)).toBe('不到 1 秒')
    expect(formatRemaining(-5000)).toBe('不到 1 秒')
  })
})

describe('expiresAtMs', () => {
  it('一般情況回傳到期時間', () => {
    expect(expiresAtMs(mailbox())).toBe(Date.parse('2026-10-07T10:00:00Z'))
  })

  it('keep=true 時回傳 null（不自動刪除，不是已過期）', () => {
    expect(expiresAtMs(mailbox({ keep: true, expires_at: ZERO_TIME_PREFIX }))).toBeNull()
  })

  it('零值時間即使 keep=false 也回傳 null', () => {
    expect(
      expiresAtMs(mailbox({ keep: false, expires_at: `${ZERO_TIME_PREFIX}T00:00:00Z` })),
    ).toBeNull()
  })

  it('無法解析的時間回傳 null', () => {
    expect(expiresAtMs(mailbox({ expires_at: 'not-a-date' }))).toBeNull()
  })
})

describe('isUsableAccount', () => {
  it('只有具備 Cookie 的帳號才可用', () => {
    expect(isUsableAccount(account({ has_cookies: true }))).toBe(true)
    expect(isUsableAccount(account())).toBe(false)
  })

  // 建立 HME 別名一定要有 Cookie；App 專用密碼只能拿來登入換 Cookie，
  // 若在這裡放行，就會把註定失敗的帳號列出來。
  it('只有 App 密碼而沒有 Cookie 時不算可用', () => {
    expect(isUsableAccount(account({ has_app_password: true }))).toBe(false)
  })

  it('兩者都有時可用（因為有 Cookie）', () => {
    expect(isUsableAccount(account({ has_cookies: true, has_app_password: true }))).toBe(true)
  })
})

describe('latestMailbox', () => {
  it('取出 created_at 最新的一筆', () => {
    const older = mailbox({ id: 'temp_1', created_at: '2026-10-06T09:00:00Z' })
    const newer = mailbox({ id: 'temp_2', created_at: '2026-10-06T11:00:00Z' })
    expect(latestMailbox([older, newer])?.id).toBe('temp_2')
    expect(latestMailbox([newer, older])?.id).toBe('temp_2')
  })

  it('空清單回傳 null', () => {
    expect(latestMailbox([])).toBeNull()
  })
})
