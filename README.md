# iCloud HME Plus 本地管理工具

[English](#english) | 中文

透過逆向 iCloud Web 介面與 IMAP 郵件協定，實現 Apple iCloud 隱藏郵件別名的建立、列出與郵件收取功能。內建中文管理介面（React 單頁應用程式，隨二進位檔內嵌發佈）。

## 功能特色

- ✅ **中文管理介面** — 瀏覽器開啟 `http://localhost:8081` 即可使用
- ✅ **建立 HME 別名** — 自動產生 iCloud 隱藏郵件地址
- ✅ **列出所有別名** — 檢視帳號下的所有 HME 別名
- ✅ **收取郵件** — 透過 IMAP 或 Web API 讀取寄到 HME 別名的郵件
- ✅ **雙路徑讀信** — 郵件讀取優先走 IMAP (App Password)，無 App Password 時回退 Web API (Cookie)
- ✅ **多帳號管理** — 支援多個 iCloud 帳號同時管理
- ✅ **雙認證模式** — Cookie（建立別名 + 讀信回退）與 App Password（IMAP 優先）
- ✅ **隨機信箱（臨時別名）** — 一鍵取得隨機 HME 信箱，預設 24 小時後自動刪除，可手動立即刪除或設為不自動刪除
- ✅ **安全模型** — 單一管理員工作階段、CSRF 驗證、登入限流、回應脫敏

## 快速開始

### 1. 安裝

#### 方式一：下載二進位發行版（推薦）

從 [GitHub Releases](https://github.com/xiaozhou26/icloud-hme/releases) 下載對應平台的二進位檔：

| 平台 | 檔案 |
|---|---|
| Linux x86_64 | `icloud-hme_linux_amd64` |
| Linux ARM64 | `icloud-hme_linux_arm64` |
| macOS Intel | `icloud-hme_darwin_amd64` |
| macOS Apple Silicon | `icloud-hme_darwin_arm64` |
| Windows x86_64 | `icloud-hme_windows_amd64.exe` |

```bash
# 範例：Linux 下直接執行（必須先設定管理員密碼）
export ICLOUD_HME_ADMIN_PASSWORD='change-this-before-running-2026'
chmod +x icloud-hme_linux_amd64
./icloud-hme_linux_amd64
```

#### 方式二：Docker

```bash
# 拉取映像檔
docker pull ghcr.io/xiaozhou26/icloud-hme:latest

# 執行（將本機 data 目錄掛載進去）
docker run -d \
  --name icloud-hme \
  -p 8081:8081 \
  -v /path/to/data:/app/data \
  -e ICLOUD_HME_ADMIN_PASSWORD='change-this-before-running-2026' \
  ghcr.io/xiaozhou26/icloud-hme:latest
```

> ⚠️ 上面的密碼僅為範例，**不可照抄**，請務必更換為至少 8 個字元的強密碼。

映像檔支援 `linux/amd64` 與 `linux/arm64` 兩種架構，會自動適配。

#### 方式三：原始碼編譯（需要 Go 1.26+ 與 Node.js 22.12+ 雙工具鏈）

```bash
# 前置需求: Go 1.26+、Node.js 22.12+
git clone https://github.com/xiaozhou26/icloud-hme.git
cd icloud-hme

# 一鍵建置（安裝前端相依套件 → 前端測試 → 前端建置 → Go 測試 → 編譯）
./build.sh

# 或者手動分步建置
npm --prefix web ci
npm --prefix web run build
go build -o icloud-hme .
```

### 2. 安全設定（必讀）

管理介面與 API 均需要管理員登入，升級後所有 API 都必須先透過 `POST /api/auth/login` 取得工作階段：

| 環境變數 | 說明 | 預設 |
|---|---|---|
| `ICLOUD_HME_ADMIN_PASSWORD` | 管理員密碼，**必填**，至少 8 個字元 | 無（缺少時拒絕啟動） |
| `ICLOUD_HME_SESSION_TTL` | 工作階段有效期限 | `12h`（範圍 `15m`–`168h`） |
| `ICLOUD_HME_SECURE_COOKIE` | 透過 TLS 反向代理部署時設為 `true` | `false` |
| `ICLOUD_HME_TEMP_TTL` | 隨機信箱自動刪除時間 | `24h`（範圍 `1m`–`720h`） |

> **Breaking Change（v0.3+）**：升級後未設定 `ICLOUD_HME_ADMIN_PASSWORD` 將拒絕啟動；
> 原有匿名 API 呼叫將收到 `401 AUTH_REQUIRED`。管理員工作階段只存在記憶體中，行程重新啟動即失效。

### 3. 設定帳號

在程式 `data/` 目錄下建立 `accounts.json`（參考儲存庫內的 `accounts.json.template`）：

```json
{
  "accounts": {
    "acc_1": {
      "id": "acc_1",
      "name": "主號",
      "real_email": "owner@example.com",
      "icloud_email": "owner@icloud.com",
      "cookies": {
        "X-APPLE-WEBAUTH-TOKEN": "token_value",
        "X-APPLE-WEBAUTH-USER": "v=1:s=1:d=22789132008"
      },
      "host": "icloud.com",
      "proxy": "http://user:pass@host:port",
      "app_password": "xxxx-xxxx-xxxx-xxxx",
      "status": "active"
    }
  }
}
```

> **提示:** 也可以透過管理介面的「帳號」頁面動態新增帳號，無須手動編輯 JSON 檔。`cookies`、`app_password`、`proxy` 都是可選的。

### 4. 啟動服務

```bash
# 二進位方式（預設 data 目錄）
export ICLOUD_HME_ADMIN_PASSWORD='your-strong-password'
./icloud-hme_linux_amd64

# 指定連接埠與資料目錄
./icloud-hme_linux_amd64 -addr :9090 -data ./my_data

# 除錯模式（啟用請求日誌）
./icloud-hme_linux_amd64 -debug

# 檢視完整參數
./icloud-hme_linux_amd64 -h
```

服務預設監聽 `:8081`。瀏覽器開啟 `http://localhost:8081` 進入管理介面（帳號 / 別名 / 隨機信箱 / 收件匣）。完整 API 契約見 [API.md](API.md)。

## API 介面

> **認證**：除 `POST /api/auth/login` 與 `GET /api/auth/session` 外，所有 `/api` 介面都需要管理員工作階段 Cookie（`hme_session`）；非 GET/HEAD/OPTIONS 請求還需攜帶 `X-CSRF-Token` 請求標頭。完整契約與 curl 範例見 [API.md](API.md)。

### 核心介面

#### 建立 HME 別名

```bash
POST /api/create

# 請求主體
{
  "account_id": "acc_1",      # 必填: 帳號 ID
  "label": "註冊某網站"        # 可選: 別名標籤
}

# 回應
{
  "success": true,
  "data": {
    "email": "xyz123@icloud.com",
    "label": "註冊某網站",
    "created_at": "2024-01-15T10:30:00Z",
    "account_id": "acc_1"
  }
}
```

#### 讀取郵件

```bash
GET /api/inbox?account_id=acc_1&alias=xyz123@icloud.com&limit=20&days=7

# 參數說明:
#   account_id - 必填: 帳號 ID
#   alias      - 可選: 只讀取寄到該別名的郵件
#   limit      - 可選: 回傳郵件數量 (預設 20)
#   days       - 可選: 查找最近幾天的郵件 (預設 7,僅 IMAP 模式)

# 回應
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
        "from": "noreply@example.com",
        "to": "xyz123@icloud.com",
        "subject": "歡迎註冊",
        "preview": "感謝您的註冊...",
        "date": "2026-07-09T14:32:10+08:00"
      }
    ]
  }
}

# 讀取方式 (自動選擇):
#   method: "imap"    — 透過 App Password 認證 (優先)
#   method: "web_api" — 透過 Cookie 認證,無須 App Password (回退)
```

### 帳號管理介面

#### 列出所有帳號

```bash
GET /api/accounts

# 回應
{
  "success": true,
  "data": [
    {"id": "acc_1", "name": "主號"},
    {"id": "acc_2", "name": "副號"}
  ]
}
```

#### 新增帳號

**簡化版（cookies 可選）:**

```bash
POST /api/accounts

# 請求主體
{
  "name": "新帳號",
  "host": "icloud.com",           # 可選
  "proxy": "http://..."           # 可選
}

# 回應 - 狀態為 pending,需登入
{
  "success": true,
  "data": {
    "id": "acc_xxx",
    "name": "新帳號",
    "status": "pending"
  }
}
```

**完整版（帶 Cookie）:**

```bash
POST /api/accounts

# 請求主體
{
  "name": "新帳號",
  "cookies": "{\"x-apple-session-token\":\"token_value\"}",  # JSON 或 Header 格式
  "host": "icloud.com",           # 可選
  "proxy": "http://..."           # 可選
}

# 回應
{
  "success": true,
  "data": {
    "id": "acc_3",
    "name": "新帳號",
    "status": "active"
  }
}
```

#### 帳號登入（取得 Cookie）

```bash
POST /api/accounts/:id/login

# 請求主體
{
  "password": "使用者的一般 iCloud 密碼",  # 不是 App Password
  "otp_code": "123456"                  # 可選,2FA 驗證碼
}

# 回應
{
  "success": true,
  "data": {
    "id": "acc_1",
    "cookies": {
      "x-apple-session-token": "...",
      "X-APPLE-WEBAUTH-TOKEN": "..."
    }
  }
}
```

#### 刪除帳號

```bash
DELETE /api/accounts/:id

# 回應
{
  "success": true,
  "data": {"id": "acc_3"}
}
```

#### 設定 App Password

```bash
POST /api/accounts/:id/password

# 請求主體
{
  "icloud_email": "your_email@icloud.com",
  "app_password": "xxxx-xxxx-xxxx-xxxx"
}

# 回應
{
  "success": true,
  "data": {
    "id": "acc_1",
    "icloud_email": "your_email@icloud.com"
  }
}
```

### 別名管理介面

#### 列出所有別名

```bash
GET /api/aliases?account_id=acc_1

# 回應
{
  "success": true,
  "data": {
    "account_id": "acc_1",
    "count": 15,
    "aliases": [
      {
        "email": "xyz123@icloud.com",
        "label": "註冊某網站",
        "created_at": "2024-01-15T10:30:00Z"
      }
    ]
  }
}
```

#### 停用別名

```bash
POST /api/aliases/:id/deactivate

# 請求主體
{
  "account_id": "acc_1"
}

# 回應
{
  "success": true,
  "data": {
    "anonymous_id": "abc123",
    "success": true
  }
}
```

#### 啟用別名

```bash
POST /api/aliases/:id/reactivate

# 請求主體
{
  "account_id": "acc_1"
}

# 回應
{
  "success": true,
  "data": {
    "anonymous_id": "abc123",
    "success": true
  }
}
```

#### 刪除別名

```bash
DELETE /api/aliases/:id

# 請求主體
{
  "account_id": "acc_1"
}

# 回應
{
  "success": true,
  "data": {
    "anonymous_id": "abc123"
  }
}
```

### 隨機信箱（臨時別名）

一鍵取得隨機的 iCloud 隱藏郵件信箱，用來註冊服務或收取一次性驗證信。預設 **24 小時後自動刪除**，可手動立即刪除，也可標記為**不自動刪除**。

**行為重點：**

- 開啟「隨機信箱」頁面時，若目前沒有任何追蹤中的隨機信箱，會自動建立一個。
- 別名本身由 iCloud 隨機產生；本伺服器只負責產生好辨識的隨機標籤（格式 `temp-<word>-<word>-<四位數>`，例如 `temp-jade-reef-4821`）。
- 到期刪除由**伺服器的背景清理程式**執行（每分鐘檢查一次），不依賴瀏覽器是否開著，因此關掉網頁也會照常刪除。
- 自動刪除連續失敗 10 次後會停止追蹤，並在日誌留下訊息（避免上游已刪除的別名讓記錄永遠卡住）。
- 關閉「不自動刪除」時，到期時間會**從當下重新起算 24 小時**。
- 只有**有 Cookie** 的帳號能用來建立隨機信箱（App 專用密碼只能拿來登入換 Cookie，本身不足以呼叫 HME 介面）。
- 追蹤記錄存放於 `<資料目錄>/temp_mailboxes.json`。
- 讀取隨機信箱的郵件請沿用既有的 `GET /api/inbox?account_id=<acc>&alias=<email>`，**沒有**專用的讀信端點。

**環境變數：** `ICLOUD_HME_TEMP_TTL` 可調整自動刪除時間，預設 `24h`，允許範圍 `1m` 到 `720h`。

```bash
POST   /api/temp                 建立隨機信箱
       body: {"account_id": "acc_xxx"}   // account_id 可省略，省略時使用第一個有 Cookie 的帳號
       -> data: TempMailbox

GET    /api/temp                 列出追蹤中的隨機信箱
       -> data: {"count": 2, "mailboxes": [TempMailbox, ...], "ttl_seconds": 86400}

POST   /api/temp/:id/keep        切換是否自動刪除
       body: {"keep": true}        // true = 不自動刪除；false = 重新起算 TTL
       -> data: TempMailbox

DELETE /api/temp/:id             立即刪除隨機信箱
       -> data: {"id": "...", "email": "...", "removed": true,
                 "upstream_warning": "..."}   // upstream_warning 只在 iCloud 端刪除失敗時出現
```

**可能錯誤碼：**

- `VALIDATION_ERROR`（400）— 沒有任何有 Cookie 的帳號可用，或指定的帳號沒有 Cookie；訊息會引導使用者前往「帳號管理」處理。
- `ACCOUNT_NOT_FOUND`（404）— 指定的 `account_id` 不存在。
- `NOT_FOUND`（404）— 指定的隨機信箱不在追蹤清單中。
- `UPSTREAM_FAILURE`（502）— iCloud 端建立失敗，或未回傳別名識別碼。

> **注意**：本專案的不變式是 **HTTP 401 只代表管理員工作階段失效**，上游 iCloud 的失敗一律使用 502，不可使用 401。`TempMailbox` 物件欄位與完整契約見 [API.md](API.md)。

## 認證方式

### 方式一: Cookie 認證 (推薦,功能最完整)

Cookie 認證可實現所有功能:建立別名、讀取郵件、管理別名。

**適用範圍:**
- 建立/停用/啟用/刪除 HME 別名 ✅
- 讀取郵件 (透過 iCloud Web API,無須 App Password) ✅

**取得 Cookie:**

1. 使用瀏覽器登入 [icloud.com](https://www.icloud.com) 或 [icloud.com.cn](https://www.icloud.com.cn) (中國區)
2. 開啟瀏覽器開發者工具 (F12)
3. 進入 Application → Cookies
4. 匯出全部 Cookie 為 `{"key":"value"}` 格式的 JSON

**關鍵 Cookie (必需):**
- `X-APPLE-WEBAUTH-TOKEN` — 認證 token
- `X-APPLE-WEBAUTH-USER` — 含 dsid (`v=1:s=1:d=22789132008`)
- `X-APPLE-WEBAUTH-HSA-TRUST` — 裝置信任 token
- `X-APPLE-DS-WEB-SESSION-TOKEN` — 工作階段 token

**注意:** 匯出的 Cookie 值不要包含多餘的引號或跳脫字元。

### 方式二: App Password 認證 (IMAP,優先讀取郵件)

App Password 用於 IMAP 讀取郵件,是郵件讀取的優先路徑 (支援伺服器端依收件人搜尋)。

**產生 App Password:**

1. 登入 [appleid.apple.com](https://appleid.apple.com)
2. 進入「登入與安全性」→「App 專用密碼」
3. 產生新密碼,用於 iCloud HME Plus

### 郵件讀取雙路徑

`GET /api/inbox` 會自動選擇讀取方式:

1. **優先: IMAP (App Password)** — 設定了 App Password 時使用,支援伺服器端依收件人 (`TO`) 搜尋
2. **回退: Web API (Cookie)** — 無 App Password 或 IMAP 失敗時,透過 `mccgateway` 端點讀取,在本機依別名過濾

回應中包含 `"method": "web_api"` 或 `"method": "imap"` 欄位,標示實際使用的讀取方式。

## 專案架構

```
icloud-hme/
├── main.go                 # 進入點: 讀取安全設定、載入帳號、啟動服務
├── web/                    # 前端專案 (React + TypeScript + Vite)
│   └── src/                #   管理介面原始碼
├── accounts.json           # 帳號設定檔 (自動產生)
├── go.mod
└── internal/
    ├── account/
    │   ├── manager.go      # 多帳號管理器 (持久化、用戶端工廠)
    │   └── public.go       # 公開 DTO (Summary) 與輸入驗證
    ├── auth/
    │   ├── manager.go      # 管理員工作階段 + CSRF
    │   └── limiter.go      # 登入失敗限流
    ├── hme/
    │   ├── client.go       # iCloud HME Web 用戶端 (Cookie 認證)
    │   └── auth.go         # SRP 登入 (帳號密碼 + 2FA 取得 Cookie)
    ├── mail/
    │   ├── client.go       # IMAP 郵件用戶端 (App Password 認證)
    │   └── web_client.go   # Web 郵件用戶端 (Cookie 認證,無須 App Password)
    ├── server/
    │   ├── server.go       # 路由分組 (認證 + CSRF)
    │   ├── backend.go      # 業務介面與 Manager 轉接器
    │   ├── auth.go         # 登入/工作階段/登出 handler 與中介軟體
    │   ├── account_handlers.go  # 帳號管理 handler
    │   └── middleware.go   # 安全回應標頭、請求大小上限
    └── webui/
        └── embed.go        # 內嵌前端資源 + SPA fallback
```

### 核心模組

- **account.Manager**: 管理多個 iCloud 帳號,負責設定持久化與用戶端建立
- **hme.Client**: 封裝 iCloud HME Web API,支援 Cookie 認證
- **hme.auth**: SRP 協定登入,支援帳號密碼 + 可選 2FA
- **mail.Client**: IMAP 郵件用戶端 (App Password,優先讀取郵件)
- **mail.WebClient**: 透過 iCloud Web API (mccgateway) 讀取郵件,無須 App Password
- **server.Server**: HTTP API 服務 + 管理介面靜態資源

## 技術棧

- **Go 1.26+** / **Gin** — HTTP 框架
- **React 19 + TypeScript + Vite 8** — 管理介面
- **go-imap** — IMAP 協定實作
- **tls-client** — TLS 指紋模擬 (繞過 iCloud 反爬蟲)

## 常見問題

### Q: 建立別名回傳 401/403 錯誤?

**A:** Cookie 已過期，需要重新取得。iCloud Cookie 有效期限通常為 24 小時。

### Q: 讀取郵件回傳逾時?

**A:** 檢查網路連線，確保可以存取 `imap.mail.me.com:993`。

### Q: 如何檢視某個別名收到了哪些郵件?

**A:** 呼叫 `GET /api/inbox?account_id=acc_1&alias=your_alias@icloud.com`

### Q: 支援同時管理多個 iCloud 帳號嗎?

**A:** 支援，在 `accounts.json` 中設定多個帳號即可，每個帳號有獨立的 `id`。

### Q: 隨機信箱關掉網頁後還會自動刪除嗎?

**A:** 會。到期刪除由伺服器的背景清理程式執行（每分鐘檢查一次），與瀏覽器是否開著無關。

## 開發指南

### 本機開發

```bash
# 前端開發模式 (vite dev server, /api 代理到 :8081)
npm --prefix web ci
npm --prefix web run dev

# 後端開發模式
export ICLOUD_HME_ADMIN_PASSWORD='your-strong-password'
go run main.go -debug

# 前端檢查 (lint + test + build)
npm --prefix web run check

# 完整建置 (含前端)
./build.sh

# 交叉編譯
GOOS=linux GOARCH=amd64 go build -o icloud-hme .
GOOS=windows GOARCH=amd64 go build -o icloud-hme.exe .
```

### 發佈

推送 `v*` tag 到 GitHub 會自動觸發 CI：

```bash
git tag v0.2.0 && git push origin --tags
```

Actions 會自動建置多平台二進位檔、Docker 映像檔（`ghcr.io/xiaozhou26/icloud-hme`）並建立 Release。

### 程式碼規範

- 程式碼註解使用中文
- 錯誤訊息回傳給使用者時使用中文
- API 回應格式統一: `{success: bool, data: any, message: string}`

## 授權條款

MIT License

---
## 社群

友情連結：[LINUX DO](https://linux.do)

## English

A local management tool for Apple iCloud Hide My Email (HME) aliases, supporting creation, listing, and email reading through reverse-engineered iCloud Web API and IMAP protocol. Ships with a built-in Chinese management UI (React SPA embedded in the single binary).

### Features

- Built-in management UI at `http://localhost:8081`
- Create HME aliases automatically
- List all aliases for an account
- Read emails sent to HME aliases via IMAP or Web API
- Temporary random mailboxes with automatic deletion (default 24h TTL)
- Manage multiple iCloud accounts
- Dual authentication: Cookie and App Password
- Security: single-admin session, CSRF checks, login rate limiting, redacted API responses

### Quick Start

#### Option 1: Binary (GitHub Releases)

Download the latest binary from [GitHub Releases](https://github.com/xiaozhou26/icloud-hme/releases):

| Platform | File |
|---|---|
| Linux x86_64 | `icloud-hme_linux_amd64` |
| Linux ARM64 | `icloud-hme_linux_arm64` |
| macOS Intel | `icloud-hme_darwin_amd64` |
| macOS Apple Silicon | `icloud-hme_darwin_arm64` |
| Windows x86_64 | `icloud-hme_windows_amd64.exe` |

```bash
# Linux example (admin password is REQUIRED, min 8 chars)
export ICLOUD_HME_ADMIN_PASSWORD='change-this-before-running-2026'
chmod +x icloud-hme_linux_amd64
./icloud-hme_linux_amd64
```

#### Option 2: Docker

```bash
docker pull ghcr.io/xiaozhou26/icloud-hme:latest

docker run -d \
  --name icloud-hme \
  -p 8081:8081 \
  -v /path/to/data:/app/data \
  -e ICLOUD_HME_ADMIN_PASSWORD='change-this-before-running-2026' \
  ghcr.io/xiaozhou26/icloud-hme:latest
```

> The password above is only an example — do NOT copy it. Use a strong password with at least 8 characters.

#### Option 3: Build from source (Go 1.26+ and Node.js 22.12+)

```bash
git clone https://github.com/xiaozhou26/icloud-hme.git
cd icloud-hme

# One-shot build (frontend deps → frontend test → frontend build → Go test → binary)
./build.sh

# Or step by step
npm --prefix web ci
npm --prefix web run build
go build -o icloud-hme .
```

### Configuration

| Env var | Description | Default |
|---|---|---|
| `ICLOUD_HME_ADMIN_PASSWORD` | Admin password, **required**, min 8 chars | none (refuses to start) |
| `ICLOUD_HME_SESSION_TTL` | Session TTL | `12h` (range `15m`–`168h`) |
| `ICLOUD_HME_SECURE_COOKIE` | Set `true` when deployed behind TLS | `false` |
| `ICLOUD_HME_TEMP_TTL` | Random mailbox auto-delete TTL | `24h` (range `1m`–`720h`) |

> **Breaking change (v0.3+)**: without `ICLOUD_HME_ADMIN_PASSWORD` the server refuses to start; all API endpoints now require login (`401 AUTH_REQUIRED`). Admin sessions are in-memory only and are lost on restart.

Create `data/accounts.json` (see `accounts.json.template`) and start the server (default port `:8081`). Open `http://localhost:8081` to use the management UI. Full API contract: [API.md](API.md).

Temporary random mailboxes are created with `POST /api/temp`; they are deleted automatically after 24 hours by a server-side cleanup job (configurable via `ICLOUD_HME_TEMP_TTL`).
