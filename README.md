# iCloud HME Plus

[English](#english) | 中文

用瀏覽器管理 Apple iCloud **「隱藏我的電子郵件」**（Hide My Email，HME）別名：建立別名、收信、產生隨機信箱。內建繁體中文管理介面，前端已內嵌，部署只需要一個執行檔。

---

## 功能

- **別名管理** — 建立、停用、啟用、刪除 HME 別名
- **收件匣** — 讀取寄到別名的郵件（IMAP 優先，Cookie 備援）
- **隨機信箱** — 一鍵產生臨時別名，預設 24 小時後自動刪除
- **多帳號** — 同時管理多個 iCloud 帳號，支援 `icloud.com` 與 `icloud.com.cn`
- **單一執行檔** — 前端內嵌，不用另外部署網頁伺服器
- **安全** — 管理員登入、CSRF 驗證、登入限流、API 回應不吐祕密

## 快速開始

### 方式一：Docker（推薦）

```bash
docker run -d \
  --name icloud-hme \
  -p 8081:8081 \
  -v /path/to/data:/app/data \
  -e ICLOUD_HME_ADMIN_PASSWORD='你的強密碼' \
  ghcr.io/creeperdevme/icloud-hme-plus:latest
```

- 映像檔為 **`linux/amd64`**。
- `-v` 掛載的目錄用來保存帳號設定（`accounts.json`）與隨機信箱追蹤，**請務必掛載**，否則容器重建後設定就沒了。
- 升級時要**先 `docker pull`** 再重建容器；`docker run` 預設不會重新拉取 `:latest`。

### 方式二：執行檔

從 [Releases](https://github.com/creeperdevme/iCloud-HME-Plus/releases) 下載對應平台：

| 平台 | 檔名 |
|---|---|
| Linux x86_64 | `icloud-hme_linux_amd64` |
| Linux ARM64 | `icloud-hme_linux_arm64` |
| macOS Intel | `icloud-hme_darwin_amd64` |
| macOS Apple Silicon | `icloud-hme_darwin_arm64` |
| Windows x86_64 | `icloud-hme_windows_amd64.exe` |

```bash
export ICLOUD_HME_ADMIN_PASSWORD='你的強密碼'
chmod +x icloud-hme_linux_amd64
./icloud-hme_linux_amd64
```

> 執行檔由推送 `v*` tag 自動產生。若 Releases 是空的，代表還沒發過版，請改用 Docker 或自行編譯。

### 方式三：自行編譯

需要 **Go 1.26+** 與 **Node.js 22+**。

```bash
git clone https://github.com/creeperdevme/iCloud-HME-Plus.git
cd iCloud-HME-Plus
./build.sh
```

`build.sh` 會依序做完：前端安裝相依 → 前端測試 → 前端建置 → Go 測試 → 編譯。

## 設定

### 環境變數

| 變數 | 說明 | 預設 |
|---|---|---|
| `ICLOUD_HME_ADMIN_PASSWORD` | 管理員密碼，**必填**，至少 8 個字元 | 無（未設定會拒絕啟動） |
| `ICLOUD_HME_SESSION_TTL` | 登入工作階段有效期 | `12h`（`15m`–`168h`） |
| `ICLOUD_HME_TEMP_TTL` | 隨機信箱自動刪除時間 | `24h`（`1m`–`720h`） |
| `ICLOUD_HME_SECURE_COOKIE` | 走 HTTPS 反向代理時設為 `true` | `false` |

管理員工作階段只存在記憶體，重新啟動後需要重新登入。

### 開始使用

1. 開啟 `http://localhost:8081`，用 `ICLOUD_HME_ADMIN_PASSWORD` 登入
2. 到「帳號管理」→「新增帳號」，填 **iCloud 信箱的 Prefix** 即可（例如填 `owner`，會自動變成 `owner@icloud.com`）
3. 在同一個對話框補上 Cookie 與（選填的）App 專用密碼

### 憑證怎麼拿

**Cookie — 建立別名必需**

1. 用瀏覽器登入 [icloud.com](https://www.icloud.com)（中國區用 [icloud.com.cn](https://www.icloud.com.cn)）
2. 按 F12 → Application → Cookies
3. 匯出成 `{"key":"value"}` 格式的 JSON

需要的關鍵 Cookie：

- `X-APPLE-WEBAUTH-TOKEN`
- `X-APPLE-WEBAUTH-USER`
- `X-APPLE-WEBAUTH-HSA-TRUST`
- `X-APPLE-DS-WEB-SESSION-TOKEN`

新增帳號對話框可以直接貼上「Get cookies.txt LOCALLY」匯出的內容，會自動識別格式，也可以四個欄位分開填。Cookie 通常約 24 小時過期，過期後重新取得即可。

**App 專用密碼 — 讀信優先路徑**

1. 登入 [appleid.apple.com](https://appleid.apple.com)
2. 進入「登入與安全性」→「App 專用密碼」→ 產生

填在「新增帳號」對話框，或事後用帳號列表上的「**App 密碼**」按鈕設定。兩種方式都會先實際以 IMAP 登入驗證，通過才儲存；從既有帳號設定時會沿用已存好的信箱，**不必再輸入一次完整位址**。

### 兩種憑證的差別

| | Cookie | App 專用密碼 |
|---|---|---|
| 建立／停用／刪除別名 | ✅ | ❌ |
| 讀取郵件 | ✅（備援） | ✅（優先） |

- **建立別名一定需要 Cookie。** App 專用密碼只能登入換取 Cookie，本身不足以呼叫 HME 介面。
- 讀信會優先走 IMAP（App 專用密碼），失敗才回退 Cookie；回應的 `method` 欄位會標示實際用了哪一種。

## 管理介面

| 頁面 | 用途 |
|---|---|
| 帳號管理 | 新增／編輯帳號、設定 Cookie 與 App 密碼、用 iCloud 密碼登入 |
| 隨機信箱 | 產生臨時別名、收信、設為保留或立即刪除 |
| 別名管理 | 列出、停用、啟用、刪除別名 |
| 收件匣 | 讀取與刪除郵件 |

## API

除 `POST /api/auth/login` 與 `GET /api/auth/session` 外，所有 `/api` 都需要登入。
工作階段 Cookie 名稱為 `hme_session`，非 GET 請求還要帶 `X-CSRF-Token` 標頭。

### 建立別名

```bash
POST /api/create
{"account_id": "acc_1", "label": "註冊某網站"}

# 回應
{"success": true, "data": {
  "email": "xyz123@icloud.com",
  "label": "註冊某網站",
  "created_at": "2026-01-15T10:30:00Z",
  "account_id": "acc_1"
}}
```

### 讀取郵件

```bash
GET /api/inbox?account_id=acc_1&alias=xyz123@icloud.com&limit=20&days=7
```

| 參數 | 說明 |
|---|---|
| `account_id` | 必填，帳號 ID |
| `alias` | 只看寄給這個別名的郵件 |
| `limit` | 回傳數量，預設 20（1–100） |
| `days` | 查找最近幾天，預設 7（1–90，僅 IMAP） |

```json
{"success": true, "data": {
  "account_id": "acc_1",
  "alias": "xyz123@icloud.com",
  "count": 2,
  "method": "imap",
  "messages": [
    {"id": "1042", "from": "noreply@example.com", "to": "xyz123@icloud.com",
     "subject": "歡迎註冊", "preview": "感謝您的註冊…", "date": "2026-07-09T14:32:10+08:00"}
  ]
}}
```

### 新增帳號

`icloud_email` 只給 Prefix 即可（`@` 後面由 `host` 決定）；`name` 留空會自動以 Prefix 當名稱。

```bash
POST /api/accounts
{
  "icloud_email": "owner",
  "host": "icloud.com",                                  # 選填
  "cookies": "{\"X-APPLE-WEBAUTH-TOKEN\":\"...\"}",      # 選填
  "app_password": "xxxx-xxxx-xxxx-xxxx",                 # 選填，會先以 IMAP 驗證
  "proxy": "http://..."                                  # 選填
}
```

`app_password` 沒通過 IMAP 驗證時**帳號仍會建立**，只是密碼不會被儲存；回應會多一個 `warning` 欄位說明原因。

### 端點總表

| 方法 | 路徑 | 用途 |
|---|---|---|
| POST | `/api/auth/login` | 登入，取得工作階段與 CSRF token |
| GET | `/api/auth/session` | 查詢目前工作階段 |
| POST | `/api/auth/logout` | 登出 |
| GET | `/api/accounts` | 列出所有帳號 |
| POST | `/api/accounts` | 新增帳號 |
| PATCH | `/api/accounts/:id` | 修改名稱／信箱／區域 |
| PUT | `/api/accounts/:id/cookies` | 更新 Cookie |
| PUT | `/api/accounts/:id/proxy` | 更新代理 |
| PUT | `/api/accounts/:id/mailbox` | 設定外部收件信箱（IMAP） |
| POST | `/api/accounts/:id/password` | 設定 App 專用密碼 |
| POST | `/api/accounts/:id/login` | 用 iCloud 密碼登入取得 Cookie |
| DELETE | `/api/accounts/:id` | 刪除帳號 |
| POST | `/api/create` | 建立 HME 別名 |
| GET | `/api/aliases` | 列出別名 |
| POST | `/api/aliases/:id/deactivate` | 停用別名 |
| POST | `/api/aliases/:id/reactivate` | 啟用別名 |
| DELETE | `/api/aliases/:id` | 刪除別名 |
| GET | `/api/inbox` | 讀取郵件列表 |
| GET | `/api/inbox/:message_id` | 讀取單封郵件 |
| DELETE | `/api/inbox/:message_id` | 刪除郵件 |
| POST | `/api/temp` | 建立隨機信箱 |
| GET | `/api/temp` | 列出追蹤中的隨機信箱 |
| POST | `/api/temp/:id/keep` | 切換是否自動刪除 |
| DELETE | `/api/temp/:id` | 立即刪除隨機信箱 |
| POST | `/api/reload` | 重新載入帳號設定 |

完整請求／回應格式、錯誤碼與 curl 範例見 [API.md](API.md)。

> **錯誤碼慣例**：`401` 只代表管理員工作階段失效；上游 iCloud 的失敗一律回傳 `502`。

## 隨機信箱

一鍵取得隨機 HME 信箱，用來註冊服務或收一次性驗證信。

- 預設 **24 小時後自動刪除**，也可以手動立即刪除，或設為**不自動刪除**
- 到期刪除由**伺服器的背景排程**執行（每分鐘檢查一次），關掉瀏覽器照樣會刪
- 標籤格式為 `temp-<word>-<word>-<四位數>`，例如 `temp-jade-reef-4821`
- 取消「不自動刪除」時，24 小時會**從當下重新起算**
- 只有**具備 Cookie** 的帳號能用來建立隨機信箱
- 讀信沿用 `GET /api/inbox?account_id=<acc>&alias=<email>`，沒有專用端點
- 自動刪除連續失敗 10 次後會停止追蹤並留下日誌，避免記錄卡死

## 專案結構

```
main.go              進入點：讀取設定、載入帳號、啟動服務
web/                 管理介面（React + TypeScript + Vite）
internal/
  account/           多帳號管理與持久化
  auth/              管理員工作階段、CSRF、登入限流
  hme/               iCloud HME Web API 用戶端、SRP 登入
  mail/              IMAP 與 Web API 兩種讀信實作
  server/            HTTP 路由與 handler
  tempmail/          隨機信箱追蹤
  webui/             內嵌前端資源
```

技術棧：**Go 1.26 + Gin**、**React 19 + TypeScript + Vite 8**、**go-imap**、**tls-client**（TLS 指紋模擬）。

## 常見問題

**建立別名回傳 401／403？**
Cookie 過期了，重新取得即可（通常約 24 小時效期）。

**讀取郵件逾時？**
確認網路能連到 `imap.mail.me.com:993`。

**如何只看某個別名的信？**
`GET /api/inbox?account_id=acc_1&alias=your_alias@icloud.com`

**可以同時管理多個 iCloud 帳號嗎？**
可以，在「帳號管理」新增多個，每個帳號有獨立的 `id`。

**讀信顯示「（無內文）」？**
請更新到最新版。舊版沒正確解析 multipart 郵件，會把正文清空。

## 開發

```bash
# 前端開發模式（vite dev server，/api 代理到 :8081）
npm --prefix web ci
npm --prefix web run dev

# 後端開發模式
export ICLOUD_HME_ADMIN_PASSWORD='你的強密碼'
go run main.go -debug

# 前端檢查（lint + test + build）
npm --prefix web run check

# 完整建置
./build.sh

# 交叉編譯
GOOS=linux GOARCH=amd64 go build -o icloud-hme .
```

推送 `main` 會觸發 **CI**（前端 lint／test／build、Go test／vet／build）與 **Docker 映像重建**。
推送 `v*` tag 則會建置多平台執行檔並建立 Release：

```bash
git tag v0.1.0 && git push origin --tags
```

程式碼註解與使用者可見的錯誤訊息一律使用中文；API 回應格式統一為 `{success, data, message}`。

## 授權

MIT License。本專案衍生自 [xiaozhou26/icloud-hme](https://github.com/xiaozhou26/icloud-hme)。

---

## English

A self-hosted dashboard for Apple iCloud **Hide My Email** aliases: create and manage aliases, read mail sent to them, and spin up temporary random mailboxes that auto-delete. The React UI is embedded in the single Go binary. The UI itself is in Chinese.

```bash
docker run -d --name icloud-hme -p 8081:8081 \
  -v /path/to/data:/app/data \
  -e ICLOUD_HME_ADMIN_PASSWORD='your-strong-password' \
  ghcr.io/creeperdevme/icloud-hme-plus:latest
```

Then open `http://localhost:8081`. The image is `linux/amd64`; mount the data volume or your account settings are lost on recreate.

| Env var | Description | Default |
|---|---|---|
| `ICLOUD_HME_ADMIN_PASSWORD` | Admin password, **required**, min 8 chars | none (refuses to start) |
| `ICLOUD_HME_SESSION_TTL` | Session TTL | `12h` |
| `ICLOUD_HME_TEMP_TTL` | Random mailbox auto-delete TTL | `24h` |
| `ICLOUD_HME_SECURE_COOKIE` | Set `true` behind a TLS reverse proxy | `false` |

Building from source needs Go 1.26+ and Node.js 22+; run `./build.sh`. Full API contract: [API.md](API.md).
