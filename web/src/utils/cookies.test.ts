import { describe, expect, it } from 'vitest'
import {
  KEY_COOKIES,
  formatLabel,
  hasAnyKeyCookie,
  parseCookies,
  splitKeyCookies,
} from './cookies'
import { toCookiePayload } from '../components/CookieFields'

/** 四個關鍵 Cookie 的名稱。 */
const KEY_NAMES = KEY_COOKIES.map((item) => item.key)

describe('parseCookies', () => {
  it('解析 Netscape cookies.txt：跳過 # 註解、保留 #HttpOnly_ 行', () => {
    const raw = [
      '# Netscape HTTP Cookie File',
      '# 這一行是註解，必須被忽略',
      '#HttpOnly_.icloud.com\tTRUE\t/\tTRUE\t1893456000\tX-APPLE-WEBAUTH-TOKEN\ttoken-value',
      '.icloud.com\tTRUE\t/\tTRUE\t1893456000\tX-APPLE-WEBAUTH-USER\tuser-value',
    ].join('\n')

    const result = parseCookies(raw)

    expect(result.format).toBe('netscape')
    expect(result.cookies).toEqual({
      'X-APPLE-WEBAUTH-TOKEN': 'token-value',
      'X-APPLE-WEBAUTH-USER': 'user-value',
    })
    // 註解行不得變成 Cookie
    expect(Object.keys(result.cookies)).toHaveLength(2)
  })

  it('解析 JSON 物件', () => {
    const result = parseCookies('{"X-APPLE-WEBAUTH-TOKEN":"abc"}')

    expect(result.format).toBe('json')
    expect(result.cookies).toEqual({ 'X-APPLE-WEBAUTH-TOKEN': 'abc' })
  })

  it('解析 JSON 陣列（name/value 形式）', () => {
    const result = parseCookies(
      '[{"name":"X-APPLE-WEBAUTH-TOKEN","value":"abc"},{"name":"extra","value":"1"}]',
    )

    expect(result.format).toBe('json')
    expect(result.cookies).toEqual({ 'X-APPLE-WEBAUTH-TOKEN': 'abc', extra: '1' })
  })

  it('解析 Cookie Header 字串', () => {
    const result = parseCookies('a=1; b=2')

    expect(result.format).toBe('header')
    expect(result.cookies).toEqual({ a: '1', b: '2' })
  })

  it('無法識別與空字串都回傳 unknown 與空 cookies', () => {
    expect(parseCookies('這不是任何 Cookie 格式')).toEqual({
      format: 'unknown',
      cookies: {},
    })
    expect(parseCookies('   ')).toEqual({ format: 'unknown', cookies: {} })
    expect(parseCookies('=沒有名稱')).toEqual({ format: 'unknown', cookies: {} })
  })
})

describe('formatLabel', () => {
  it('回傳各格式的中文名稱', () => {
    expect(formatLabel('netscape')).toBe('cookies.txt（Netscape）')
    expect(formatLabel('json')).toBe('JSON')
    expect(formatLabel('header')).toBe('Cookie Header')
    expect(formatLabel('unknown')).toBe('未知格式')
  })
})

describe('splitKeyCookies', () => {
  it('依名稱（不分大小寫）拆出四個關鍵 Cookie 與額外 Cookie', () => {
    const { fields, extras } = splitKeyCookies({
      'x-apple-webauth-token': 'token-value',
      'X-APPLE-WEBAUTH-USER': 'user-value',
      'X-Apple-Webauth-Hsa-Trust': 'trust-value',
      'X-APPLE-DS-WEB-SESSION-TOKEN': 'session-value',
      other: 'extra-value',
    })

    // 欄位一律使用標準名稱，方便直接對應輸入框
    expect(fields).toEqual({
      [KEY_NAMES[0]]: 'token-value',
      [KEY_NAMES[1]]: 'user-value',
      [KEY_NAMES[2]]: 'trust-value',
      [KEY_NAMES[3]]: 'session-value',
    })
    expect(extras).toEqual({ other: 'extra-value' })
  })

  it('缺少的關鍵 Cookie 不會出現在 fields', () => {
    const { fields, extras } = splitKeyCookies({ 'X-APPLE-WEBAUTH-TOKEN': 'only-token' })

    expect(Object.keys(fields)).toEqual(['X-APPLE-WEBAUTH-TOKEN'])
    expect(extras).toEqual({})
  })
})

describe('hasAnyKeyCookie', () => {
  it('只要有一個關鍵 Cookie 非空白即為 true', () => {
    expect(hasAnyKeyCookie({})).toBe(false)
    expect(hasAnyKeyCookie({ 'X-APPLE-WEBAUTH-TOKEN': '   ' })).toBe(false)
    expect(hasAnyKeyCookie({ 'X-APPLE-WEBAUTH-TOKEN': ' abc ' })).toBe(true)
  })
})

describe('toCookiePayload', () => {
  it('修剪值並丟棄空值', () => {
    const payload = toCookiePayload({
      fields: {
        'X-APPLE-WEBAUTH-TOKEN': '  abc  ',
        'X-APPLE-WEBAUTH-USER': '   ',
        'X-APPLE-WEBAUTH-HSA-TRUST': '',
        'X-APPLE-DS-WEB-SESSION-TOKEN': 'session',
      },
      extras: { other: '  extra  ', blank: '  ' },
    })

    expect(payload).toEqual({
      'X-APPLE-WEBAUTH-TOKEN': 'abc',
      'X-APPLE-DS-WEB-SESSION-TOKEN': 'session',
      other: 'extra',
    })
  })

  it('額外 Cookie 不得覆蓋同名關鍵 Cookie', () => {
    const payload = toCookiePayload({
      fields: { 'X-APPLE-WEBAUTH-TOKEN': 'from-field' },
      extras: { 'X-APPLE-WEBAUTH-TOKEN': 'from-extras' },
    })

    expect(payload).toEqual({ 'X-APPLE-WEBAUTH-TOKEN': 'from-field' })
  })

  it('全部為空時回傳空物件', () => {
    expect(toCookiePayload({ fields: {}, extras: {} })).toEqual({})
  })
})
