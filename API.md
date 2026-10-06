# iCloud HME Plus API 文件

## 概述

HTTP JSON API，所有介面回傳統一格式：

```json
{
  "success": true,
  "data": {}
}
```

**失敗回應：**

```json
{
  "success": false,
  "code": "VALIDATION_ERROR",
  "message": "參數錯誤"
}
```

**穩定錯誤碼：** `AUTH_REQUIRED`、`INVALID_CREDENTIALS`、`RATE_LIMITED`、`CSRF_INVALID`、`VALIDATION_ERROR`、`ACCOUNT_NOT_FOUND`、`NOT_FOUND`、`OTP_REQUIRED`、`OTP_INVALID`、`UPSTREAM_UNAUTHORIZED`、`UPSTREAM_FAILURE`、`INTERNAL_ERROR`

**安全約定：**

- 除 `POST /api/auth/login` 與 `GET /api/auth/session` 外，所有 `/api` 介面都需要管理員工作階段
- 非 GET/HEAD/OPTIONS 請求必須攜帶 `X-CSRF-Token` 請求標頭
- 工作階段 Cookie：`hme_session`，`Path=/`、`HttpOnly`、`SameSite=Strict`；TLS 部署時設定 `ICLOUD_HME_SECURE_COOKIE=true` 以啟用 `Secure`
- 任何帳號回應**絕不包含** `cookies`、`app_password`、`proxy` 欄位（代理只暴露 `has_proxy` 布林值）
- 使用者可見的錯誤訊息不會拼接上游回應主體或機密

---

## 認證端點

### 1. 登入

```http
POST /api/auth/login
Content-Type: application/json

{"password": "管理員密碼"}
```

**成功回應：** 設定 `hme_session` Cookie

```json
{
  "success": true,
  "data": {
    "csrf_token": "隨機值",
    "expires_at": "2026-08-05T22:00:00+08:00"
  }
}
```

**錯誤：**

- `401 INVALID_CREDENTIALS` — 密碼錯誤（不設定 Cookie）
- `429 RATE_LIMITED` — 同一 IP 15 分鐘內失敗超過 5 次，回應帶 `Retry-After` 標頭

### 2. 查詢工作階段

```http
GET /api/auth/session
Cookie: hme_session=...
```

**成功回應：** 同上（csrf_token / expires_at）。工作階段無效時回傳 `401 AUTH_REQUIRED`。

### 3. 登出

```http
POST /api/auth/logout
Cookie: hme_session=...
X-CSRF-Token: <token>

{"success": true, "data": {"logged_out": true}}
```

---

## 帳號端點

### 4. 列出帳號

```http
GET /api/accounts
```

**回應：** `Summary[]`，排序為 active → pending → error，同狀態再依 name、id 排序。

```json
{
  "success": true,
  "data": [
    {
      "id": "acc_12345678",
      "name": "主號",
      "real_email": "owner@example.com",
      "icloud_email": "owner@icloud.com",
      "host": "icloud.com",
      "status": "active",
      "alias_total": 15,
      "alias_active": 12,
      "has_cookies": true,
      "has_app_password": true,
      "has_proxy": false,
      "last_validated": "2026-08-04T09:00:00+08:00",
      "status_message": "",
      "created_at": "2026-08-01T09:00:00+08:00"
    }
  ]
}
```

**禁止出現的欄位：** `cookies`、`app_password`、`proxy`。`status_message` 只會對應固定文案（pending → 「等待設定或驗證憑證」，error → 「憑證驗證失敗」），不會回傳內部錯誤原文。

### 5. 新增帳號

```http
POST /api/accounts
X-CSRF-Token: <token>

{
  "name": "新帳號",
  "icloud_email": "owner@icloud.com",
  "host": "icloud.com",
  "proxy": "http://user:pass@host:port",
  "cookies": "X-APPLE-WEBAUTH-TOKEN=abc; X-APPLE-WEBAUTH-USER=def"
}
```

- `name` 必填，去除空白後 1–64 個字元
- `icloud_email` 必填，以 `net/mail` 驗證且位址值必須等於輸入
- `host` 只能是 `icloud.com` 或 `icloud.com.cn`（預設 `icloud.com`）
- `proxy` 可選，必須是 `http`/`https`/`socks5` URL
- `cookies` 可選，支援 Cookie Header 字串或 JSON 文字
- 無 Cookie 時狀態為 `pending`，不會存取網路

**成功回應：** `201`，回傳 `Summary`。

### 6. 編輯帳號基本資料

```http
PATCH /api/accounts/:id
X-CSRF-Token: <token>

{"name": "新名稱", "host": "icloud.com.cn"}
```

只接受可選的 `name`、`icloud_email`、`host`，至少須存在一個欄位。回應會回傳更新後的 `Summary`。帳號不存在時回傳 `404 ACCOUNT_NOT_FOUND`。

### 7. 更新代理

```http
PUT /api/accounts/:id/proxy
X-CSRF-Token: <token>

{"proxy": "http://user:pass@host:port"}
```

空字串表示清除代理。回應只回傳更新後的 `Summary`（代理值從不回顯）。

### 8. 更新 Cookie

```http
PUT /api/accounts/:id/cookies
X-CSRF-Token: <token>

{"cookies": "a=1; b=2"}
```

`cookies` 同時相容字串與物件：

```json
{"cookies": {"a": "1", "b": "2"}}
```

兩種輸入最終都會交給 `account.ParseCookieInput`。回應只回傳更新後的 `Summary`。

### 9. 設定 App 專用密碼

```http
POST /api/accounts/:id/password
X-CSRF-Token: <token>

{"icloud_email": "your_email@icloud.com", "app_password": "xxxx-xxxx-xxxx-xxxx"}
```

伺服器端會以 IMAP 連線驗證憑證。成功時回傳 `Summary`；IMAP 驗證失敗時回傳 `502 UPSTREAM_FAILURE`。

### 10. iCloud 密碼登入（取得 Cookie）

```http
POST /api/accounts/:id/login
X-CSRF-Token: <token>

{"password": "使用者的一般 iCloud 密碼", "otp_code": "123456"}
```

- `otp_code` 可選，啟用 2FA 時使用
- 需要 OTP：`409 OTP_REQUIRED`
- 驗證碼錯誤：`401 OTP_INVALID`
- 成功：**只回傳 `Summary`，絕不回傳 Cookies**（Cookie 會自動持久化到帳號設定）

### 11. 刪除帳號

```http
DELETE /api/accounts/:id
X-CSRF-Token: <token>
```

**回應：** `{"id": "acc_3"}`。不存在時回傳 `404 ACCOUNT_NOT_FOUND`。

## 業務端點

### 12. 建立 HME 別名

```http
POST /api/create
X-CSRF-Token: <token>

{"account_id": "acc_1", "label": "註冊某網站"}
```

- `account_id` 必填
- `label` 可選，最長 200 個字元

**回應：**

```json
{
  "success": true,
  "data": {
    "email": "xyz123@icloud.com",
    "label": "註冊某網站",
    "created_at": "2026-01-15T10:30:00+08:00",
    "account_id": "acc_1"
  }
}
```

### 13. 讀取郵件

```http
GET /api/inbox?account_id=acc_1&alias=xyz123@icloud.com&limit=20&days=7
```

- `account_id` 必填
- `alias` 可選，只回傳寄給該別名的郵件
- `limit` 1–100（預設 20）
- `days` 1–90（預設 7）；非法的整數會直接回傳 `400 VALIDATION_ERROR`

**回應（IMAP 優先，Web API 回退）：**

```json
{
  "success": true,
  "data": {
    "account_id": "acc_1",
    "alias": "xyz123@icloud.com",
    "count": 2,
    "method": "imap",
    "messages": [
      {
        "id": "1042",
        "from": "GitHub <noreply@github.com>",
        "to": "xyz123@icloud.com",
        "subject": "[GitHub] Please verify your email address",
        "date": "2026-07-09T14:32:10+08:00",
        "preview": "Almost done! To finish setting up your account..."
      }
    ]
  }
}
```

`method` 為 `imap` 或 `web_api`。IMAP 路徑支援伺服器端依收件人搜尋；Web API 路徑會先拉取再於本機過濾。

### 14. 列出別名

```http
GET /api/aliases?account_id=acc_1
```

**回應：** alias 物件的欄位風格為 camelCase（相容 iCloud 原始格式）：

```json
{
  "success": true,
  "data": {
    "account_id": "acc_1",
    "count": 15,
    "aliases": [
      {
        "email": "xyz123@icloud.com",
        "anonymousId": "abc123",
        "label": "註冊某網站",
        "active": true,
        "createdAt": "2026-01-15T10:30:00Z"
      }
    ]
  }
}
```

### 15. 停用/啟用/刪除別名

```http
POST /api/aliases/:id/deactivate
POST /api/aliases/:id/reactivate
DELETE /api/aliases/:id
X-CSRF-Token: <token>

{"account_id": "acc_1"}
```

- `:id` 為別名的 `anonymousId`，不可為空且 URL 解碼後不超過 256 個字元
- `account_id` 必填
- 刪除後無法復原；直接刪除失敗時會先停用再刪除

### 16. 建立隨機信箱（臨時別名）

```http
POST /api/temp
X-CSRF-Token: <token>

{"account_id": "acc_1"}
```

- `account_id` 可省略；省略時使用第一個有 Cookie 的帳號
- 只有**有 Cookie** 的帳號能用來建立隨機信箱（App 專用密碼只能拿來登入換 Cookie，本身不足以呼叫 HME 介面）
- 別名本身由 iCloud 隨機產生；本服務只負責產生好辨識的隨機標籤（格式 `temp-<word>-<word>-<四位數>`，例如 `temp-jade-reef-4821`）
- 預設 **24 小時後自動刪除**，可手動立即刪除，也可標記為**不自動刪除**
- 到期刪除由**伺服器的背景清理程式**執行（每分鐘檢查一次），不依賴瀏覽器是否開著
- 開啟「隨機信箱」頁面時，若目前沒有任何追蹤中的隨機信箱，會自動建立一個

**回應：** `TempMailbox`。沒有任何有 Cookie 的帳號可用，或指定帳號沒有 Cookie 時回傳 `400 VALIDATION_ERROR`（訊息會引導使用者前往「帳號管理」處理）；`account_id` 不存在時回傳 `404 ACCOUNT_NOT_FOUND`。

### 17. 列出隨機信箱

```http
GET /api/temp
```

**回應：**

```json
{
  "success": true,
  "data": {
    "count": 2,
    "mailboxes": [
      {
        "id": "abc123",
        "email": "jade.reef.4821@icloud.com",
        "account_id": "acc_xxx",
        "label": "temp-jade-reef-4821",
        "created_at": "2026-10-06T14:00:00Z",
        "expires_at": "2026-10-07T14:00:00Z",
        "keep": false,
        "attempts": 0,
        "last_error": ""
      }
    ],
    "ttl_seconds": 86400
  }
}
```

`ttl_seconds` 為目前的自動刪除秒數（對應 `ICLOUD_HME_TEMP_TTL`，預設 86400 秒）。

### 18. 切換隨機信箱是否自動刪除

```http
POST /api/temp/:id/keep
X-CSRF-Token: <token>

{"keep": true}
```

- `true` = 不自動刪除；`false` = 重新起算 TTL
- 關閉「不自動刪除」（`keep=false`）時，到期時間會**從當下重新起算 24 小時**
- `keep` 為 `true` 時，`expires_at` 是零值（`0001-01-01T00:00:00Z`）

**回應：** `TempMailbox`。不在追蹤清單中時回傳 `404 NOT_FOUND`。

### 19. 立即刪除隨機信箱

```http
DELETE /api/temp/:id
X-CSRF-Token: <token>
```

**回應：**

```json
{
  "success": true,
  "data": {
    "id": "abc123",
    "email": "jade.reef.4821@icloud.com",
    "removed": true,
    "upstream_warning": "iCloud 端刪除失敗的說明"
  }
}
```

- `upstream_warning` 只在 iCloud 端刪除失敗時出現
- 不在追蹤清單中時回傳 `404 NOT_FOUND`

**`TempMailbox` 物件：**

```json
{
  "id": "別名的 anonymousId，刪除時使用",
  "email": "jade.reef.4821@icloud.com",
  "account_id": "acc_xxx",
  "label": "temp-jade-reef-4821",
  "created_at": "2026-10-06T14:00:00Z",
  "expires_at": "2026-10-07T14:00:00Z",
  "keep": false,
  "attempts": 0,
  "last_error": ""
}
```

- `keep` 為 `true` 時 `expires_at` 是零值（`0001-01-01T00:00:00Z`）
- `attempts` 為自動刪除的連續失敗次數；連續失敗 10 次後會停止追蹤，並在日誌留下訊息（避免上游已刪除的別名讓記錄永遠卡住）
- 讀取隨機信箱的郵件請沿用既有的 `GET /api/inbox?account_id=<acc>&alias=<email>`，**沒有**專用的讀信端點
- 追蹤記錄存放於 `<資料目錄>/temp_mailboxes.json`
- 環境變數 `ICLOUD_HME_TEMP_TTL` 可調整自動刪除時間，預設 `24h`，允許範圍 `1m` 到 `720h`
- 可能錯誤碼：`VALIDATION_ERROR`（400，沒有任何有 Cookie 的帳號可用，或指定帳號沒有 Cookie）、`ACCOUNT_NOT_FOUND`（404，`account_id` 不存在）、`NOT_FOUND`（404，隨機信箱不在追蹤清單中）、`UPSTREAM_FAILURE`（502，iCloud 端建立失敗或未回傳別名識別碼）
- 本專案的不變式：**HTTP 401 只代表管理員工作階段失效**；上游 iCloud 的失敗一律使用 502，不可使用 401

### 20. 重新載入設定

```http
POST /api/reload
X-CSRF-Token: <token>
```

重新讀取 `accounts.json`。

---

## curl 使用範例（Cookie Jar + CSRF）

```bash
BASE="http://localhost:8081"

# 1. 登入，將 Cookie 儲存到 jar
curl -c cookies.txt -X POST "$BASE/api/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"password":"你的管理員密碼"}'

# 2. 從回應中擷取 csrf_token(可用 jq)
CSRF=$(curl -b cookies.txt "$BASE/api/auth/session" | jq -r '.data.csrf_token')

# 3. 讀取帳號清單(GET 無須 CSRF)
curl -b cookies.txt "$BASE/api/accounts"

# 4. 新增帳號(mutation 需要 CSRF 標頭)
curl -b cookies.txt -X POST "$BASE/api/accounts" \
  -H "Content-Type: application/json" \
  -H "X-CSRF-Token: $CSRF" \
  -d '{"name":"新帳號","icloud_email":"owner@icloud.com"}'

# 5. 建立別名
curl -b cookies.txt -X POST "$BASE/api/create" \
  -H "Content-Type: application/json" \
  -H "X-CSRF-Token: $CSRF" \
  -d '{"account_id":"acc_1","label":"GitHub"}'

# 6. 讀取郵件
curl -b cookies.txt "$BASE/api/inbox?account_id=acc_1&limit=10"

# 7. 建立隨機信箱(省略 account_id 時使用第一個有 Cookie 的帳號)
curl -b cookies.txt -X POST "$BASE/api/temp" \
  -H "Content-Type: application/json" \
  -H "X-CSRF-Token: $CSRF" \
  -d '{"account_id":"acc_1"}'

# 8. 列出追蹤中的隨機信箱
curl -b cookies.txt "$BASE/api/temp"
```

---

## 認證方式（iCloud 帳號端）

### Cookie 認證（功能最完整）

用於建立/停用/啟用/刪除別名、讀取郵件（Web API 回退）。

**取得方式：**
1. 以瀏覽器登入 [icloud.com](https://www.icloud.com) 或 [icloud.com.cn](https://www.icloud.com.cn)（中國區）
2. F12 → Application → Cookies
3. 匯出 Cookie 為 `{"key":"value"}` JSON，貼到管理介面的「更新 Cookie」

**關鍵 Cookie：** `X-APPLE-WEBAUTH-TOKEN`（認證 token）、`X-APPLE-WEBAUTH-USER`（含 dsid）、`X-APPLE-WEBAUTH-HSA-TRUST`（裝置信任）、`X-APPLE-DS-WEB-SESSION-TOKEN`（工作階段）

**有效期限：** 約 24 小時

### App Password 認證（IMAP 優先讀取郵件）

用於 IMAP 讀取郵件（優先路徑，支援伺服器端依收件人搜尋）。在 [appleid.apple.com](https://appleid.apple.com) → 登入與安全性 → App 專用密碼 產生。

---

## 技術說明

**Web API 路徑** (`internal/mail/web_client.go`)：
1. 呼叫 `setup.icloud.com.cn/setup/ws/1/validate` 取得 `mccgateway` URL
2. 呼叫 `mccgateway/mailws2/v1/thread/search` 讀取郵件

**⚠️ 已知問題：**
- `validate` 回傳的 mccgateway URL 可能帶有 `:443` 連接埠，tls-client 的 cookie jar 是依不帶連接埠的 host 儲存 cookie，因此帶連接埠的請求會無法附加 cookie 而導致 403；**解決方式：** 解析 URL 後剝離連接埠號

**IMAP 路徑** (`internal/mail/client.go`)：標準 IMAP 協定，連線 `imap.mail.me.com:993`，需要 App Password。

**升級差異（相對於舊版）：**
- 全部 API 都需要管理員登入（`401 AUTH_REQUIRED`）
- 帳號回應不再回傳 Cookie/密碼/代理原文，改用 `has_cookies`/`has_app_password`/`has_proxy`
- `POST /api/accounts/:id/login` 成功回應不再回傳 `cookies` 欄位
- `accounts.json` 使用 `{"accounts": {id: {...}}}` map wrapper 格式（參考 `accounts.json.template`）

## 限制

- **建立頻率**：iCloud 會限制別名建立頻率，過快會回傳 429（伺服器端會自動重試最多 5 次）
- **Cookie 有效期限**：約 24 小時，需定期更新
- **郵件讀取**：依賴 IMAP 連線，逾時預設 30 秒
- **隨機信箱**：預設 24 小時後由伺服器背景清理程式自動刪除，可用 `ICLOUD_HME_TEMP_TTL` 調整（範圍 `1m` 到 `720h`）
- **請求主體大小上限**：1 MiB
