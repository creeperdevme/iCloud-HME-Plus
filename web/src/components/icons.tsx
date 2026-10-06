import { useId } from 'react'
import type { SVGProps } from 'react'

type IconProps = SVGProps<SVGSVGElement> & { size?: number }

function base({ size = 18, ...props }: IconProps): SVGProps<SVGSVGElement> {
  return {
    width: size,
    height: size,
    viewBox: '0 0 24 24',
    fill: 'none',
    stroke: 'currentColor',
    strokeWidth: 2,
    strokeLinecap: 'round' as const,
    strokeLinejoin: 'round' as const,
    'aria-hidden': true,
    ...props,
  }
}

export function IconAccounts(props: IconProps) {
  return (
    <svg {...base(props)}>
      <circle cx="9" cy="8" r="3.5" />
      <path d="M2.5 20c.8-3.2 3.4-5 6.5-5s5.7 1.8 6.5 5" />
      <path d="M16 4.5a3.5 3.5 0 0 1 0 7" />
      <path d="M17.5 15.2c1.8.8 3.2 2.3 4 4.8" />
    </svg>
  )
}

export function IconAliases(props: IconProps) {
  return (
    <svg {...base(props)}>
      <rect x="3" y="5" width="18" height="14" rx="3" />
      <path d="M3 9h18" />
      <path d="M7 14h5" />
      <path d="M7 17h2" />
    </svg>
  )
}

export function IconInbox(props: IconProps) {
  return (
    <svg {...base(props)}>
      <path d="M22 12h-6l-2 3h-4l-2-3H2" />
      <path d="M5.5 5h13l3.5 7v6a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2v-6z" />
    </svg>
  )
}

export function IconPlus(props: IconProps) {
  return (
    <svg {...base(props)}>
      <path d="M12 5v14" />
      <path d="M5 12h14" />
    </svg>
  )
}

export function IconLogout(props: IconProps) {
  return (
    <svg {...base(props)}>
      <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4" />
      <path d="M16 17l5-5-5-5" />
      <path d="M21 12H9" />
    </svg>
  )
}

export function IconTrash(props: IconProps) {
  return (
    <svg {...base(props)}>
      <path d="M3 6h18" />
      <path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
      <path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6" />
      <path d="M10 11v6" />
      <path d="M14 11v6" />
    </svg>
  )
}

export function IconEdit(props: IconProps) {
  return (
    <svg {...base(props)}>
      <path d="M17 3a2.8 2.8 0 1 1 4 4L7.5 20.5 2 22l1.5-5.5z" />
    </svg>
  )
}

export function IconCopy(props: IconProps) {
  return (
    <svg {...base(props)}>
      <rect x="9" y="9" width="13" height="13" rx="2" />
      <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
    </svg>
  )
}

export function IconSearch(props: IconProps) {
  return (
    <svg {...base(props)}>
      <circle cx="11" cy="11" r="8" />
      <path d="M21 21l-4.35-4.35" />
    </svg>
  )
}

export function IconCheck(props: IconProps) {
  return (
    <svg {...base(props)}>
      <path d="M20 6L9 17l-5-5" />
    </svg>
  )
}

export function IconClock(props: IconProps) {
  return (
    <svg {...base(props)}>
      <circle cx="12" cy="12" r="10" />
      <path d="M12 6v6l4 2" />
    </svg>
  )
}

export function IconChevronUp(props: IconProps) {
  return (
    <svg {...base(props)}>
      <path d="M6 14l6-6 6 6" />
    </svg>
  )
}

export function IconChevronDown(props: IconProps) {
  return (
    <svg {...base(props)}>
      <path d="M6 10l6 6 6-6" />
    </svg>
  )
}

export function IconAlert(props: IconProps) {
  return (
    <svg {...base(props)}>
      <path d="M10.3 3.9L1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z" />
      <path d="M12 9v4" />
      <path d="M12 17h.01" />
    </svg>
  )
}

export function IconInfo(props: IconProps) {
  return (
    <svg {...base(props)}>
      <circle cx="12" cy="12" r="10" />
      <path d="M12 16v-4" />
      <path d="M12 8h.01" />
    </svg>
  )
}

export function IconMail(props: IconProps) {
  return (
    <svg {...base(props)}>
      <rect x="2" y="4" width="20" height="16" rx="2" />
      <path d="M22 7l-10 6L2 7" />
    </svg>
  )
}

export function IconRefresh(props: IconProps) {
  return (
    <svg {...base(props)}>
      <path d="M23 4v6h-6" />
      <path d="M20.5 15a9 9 0 1 1-2-9.4L23 10" />
    </svg>
  )
}

export function IconInboxEmpty(props: IconProps) {
  return (
    <svg {...base({ size: 40, ...props })}>
      <path d="M22 12h-6l-2 3h-4l-2-3H2" />
      <path d="M5.5 5h13l3.5 7v6a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2v-6z" />
    </svg>
  )
}

export function IconKey(props: IconProps) {
  return (
    <svg {...base(props)}>
      <circle cx="7.5" cy="15.5" r="4.5" />
      <path d="M10.7 12.3L21 2" />
      <path d="M17 6l3 3" />
      <path d="M14 9l3 3" />
    </svg>
  )
}

export function IconShield(props: IconProps) {
  return (
    <svg {...base(props)}>
      <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
      <path d="M9 12l2 2 4-4" />
    </svg>
  )
}

export function IconLock(props: IconProps) {
  return (
    <svg {...base(props)}>
      <rect x="3" y="11" width="18" height="11" rx="2" />
      <path d="M7 11V7a5 5 0 0 1 10 0v4" />
    </svg>
  )
}

export function IconCookie(props: IconProps) {
  return (
    <svg {...base(props)}>
      <path d="M12 2a10 10 0 1 0 10 10 4 4 0 0 1-5-5 4 4 0 0 1-5-5z" />
      <path d="M8.5 8.5h.01" />
      <path d="M16 15.5h.01" />
      <path d="M12 12h.01" />
      <path d="M7 15h.01" />
    </svg>
  )
}

export function IconGlobe(props: IconProps) {
  return (
    <svg {...base(props)}>
      <circle cx="12" cy="12" r="10" />
      <path d="M2 12h20" />
      <path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z" />
    </svg>
  )
}

export function IconSparkles(props: IconProps) {
  return (
    <svg {...base(props)}>
      <path d="M12 3l1.6 4.4L18 9l-4.4 1.6L12 15l-1.6-4.4L6 9l4.4-1.6z" />
      <path d="M18 15l.8 2.2L21 18l-2.2.8L18 21l-.8-2.2L15 18l2.2-.8z" />
    </svg>
  )
}

/** 隨機信箱：信封加上時鐘，表示「會自動到期的臨時信箱」。 */
export function IconTempMail(props: IconProps) {
  return (
    <svg {...base(props)}>
      <rect x="1.5" y="4" width="17" height="12.5" rx="2.5" />
      <path d="M2.2 6.6l7.8 4.9 7.8-4.9" />
      <circle cx="18.3" cy="18" r="4.2" />
      <path d="M18.3 16.4v1.7l1.3.8" />
    </svg>
  )
}

/**
 * 圓角方形底：90×90 的方形，圓角 20.652633（座標為下方 `<g>` 的區域空間）。
 *
 * 取自使用者提供的向量檔，未經改寫，因此畫面上看到的就是原始圖形。
 */
const ICLOUD_MAIL_SQUARE =
  'm 21.652659,961.36222 48.694734,0 c 11.441552,0 20.652633,9.21108 20.652633,20.65264 ' +
  'l 0,48.69474 c 0,11.4416 -9.211081,20.6526 -20.652633,20.6526 l -48.694734,0 ' +
  'c -11.441563,0 -20.6526336,-9.211 -20.6526336,-20.6526 l 0,-48.69474 ' +
  'c 0,-11.44156 9.2110706,-20.65264 20.6526336,-20.65264 z'

/** 白色信封：折線、左右側身與底部收合的完整輪廓。（同樣取自原始向量檔） */
const ICLOUD_MAIL_ENVELOPE =
  'm 20.71875,536.59375 c -0.474202,0 -0.920938,0.0818 -1.34375,0.25 l 8.46875,8.71875 ' +
  '8.5625,8.875 0.15625,0.1875 0.25,0.25 0.25,0.25 0.5,0.53125 7.34375,7.53125 ' +
  'c 0.122269,0.076 0.476602,0.4042 0.753434,0.54258 0.356583,0.17824 0.743089,0.34255 ' +
  '1.141484,0.3568 0.42992,0.0154 0.869334,-0.10782 1.256181,-0.29601 ' +
  '0.289732,-0.14096 0.418572,-0.34294 0.755151,-0.60337 l 8.5,-8.78125 8.59375,-8.84375 ' +
  '8.28125,-8.53125 c -0.531643,-0.28806 -1.120466,-0.4375 -1.75,-0.4375 z ' +
  'm -2.59375,1.0625 c -0.903115,0.85572 -1.46875,2.14217 -1.46875,3.59375 l 0,28.625 ' +
  'c 0,1.17535 0.377499,2.24307 1,3.0625 l 1.1875,-1.125 8.84375,-8.59375 7.84375,-7.59375 ' +
  '-0.15625,-0.1875 -8.59375,-8.84375 -8.59375,-8.875 z ' +
  'm 57.1875,0.28125 -8.375,8.65625 -8.5625,8.84375 -0.15625,0.15625 8.15625,7.90625 ' +
  '8.84375,8.59375 0.53125,0.5 c 0.476164,-0.76402 0.75,-1.70518 0.75,-2.71875 l 0,-28.625 ' +
  'c 0,-1.29428 -0.448516,-2.46795 -1.1875,-3.3125 z ' +
  'm -38.78125,18.71875 -7.8125,7.59375 -8.875,8.59375 -1.125,1.09375 ' +
  'c 0.593096,0.38196 1.268042,0.625 2,0.625 l 51.71875,0 c 0.879957,0 1.678116,-0.33853 ' +
  '2.34375,-0.875 l -0.5625,-0.5625 -8.875,-8.59375 -8.15625,-7.875 -7.34375,7.5625 ' +
  'c -0.397465,0.2635 -0.663064,0.55576 -1.051168,0.73523 -0.624615,0.28885 -1.309163,0.53321 ' +
  '-1.997252,0.52267 -0.689922,-0.0106 -1.366428,-0.28061 -1.985577,-0.58517 ' +
  '-0.310792,-0.15288 -0.476438,-0.30481 -0.841003,-0.61023 z'

/**
 * 品牌標誌：藍色漸層圓角方形 + 白色信封。
 *
 * 幾何與漸層都直接取自使用者提供的向量檔（Inkscape 輸出的 602×602 圖形），
 * 沒有重畫或近似，因此保留原始的路徑資料與座標系：
 *
 * - 圖形本體在 1..601 之間，故 `viewBox="0 0 602 602"`。
 * - 內層 `<g>` 的 `matrix` 與路徑上的 `translate` 都照抄原檔；兩者必須成對保留，
 *   否則信封會跟底色分家（原檔靠這兩段轉換把兩者疊在一起）。
 * - 漸層是 `userSpaceOnUse`，方向由下（#70efff 淺青）到上（#5770ff 藍），
 *   也就是淺色在下、深色在上。
 *
 * 商標與圖示設計仍屬 Apple Inc. 所有。
 */
export function IconICloudMailLogo({ size = 40, ...props }: IconProps) {
  // 漸層 id 必須在整份文件內唯一，否則同頁多個實例會互相蓋掉。
  // useId 產生的字串含有「:」，放在 url(#…) 裡較為脆弱，這裡先移除。
  const gradientId = `icloud-mail-logo-${useId().replace(/:/g, '')}`

  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 602 602"
      fill="none"
      aria-hidden="true"
      {...props}
    >
      <defs>
        <linearGradient
          id={gradientId}
          gradientUnits="userSpaceOnUse"
          gradientTransform="matrix(0.15,0,0,0.15,0.85002387,961.21217)"
          x1="305.20093"
          y1="598.59198"
          x2="305.785"
          y2="8.2437592"
        >
          <stop offset="0" stopColor="#70efff" />
          <stop offset="1" stopColor="#5770ff" />
        </linearGradient>
      </defs>

      {/* 外層 translate 與內層 matrix 皆為原檔所有，缺一不可 */}
      <g transform="translate(0,-450.36218)">
        <g transform="matrix(6.6666666,0,0,6.6666666,-5.6668106,-5957.7191)">
          <path d={ICLOUD_MAIL_SQUARE} fill={`url(#${gradientId})`} fillRule="nonzero" />
          {/* 這個 translate 會抵銷外層的 translate，讓信封與底色對齊 */}
          <path
            transform="translate(0,450.36218)"
            d={ICLOUD_MAIL_ENVELOPE}
            fill="#ffffff"
            fillRule="nonzero"
          />
        </g>
      </g>
    </svg>
  )
}
