import { useEffect, useState } from 'react'
import { request, ApiError } from '../api/client'
import { useToast } from '../components/ToastProvider'
import Dialog from '../components/Dialog'

type Key = {
  id: string
  name: string
  created_at: string
  revoked_at?: string
}

type Webhook = {
  id: string
  name: string
  type: string
  url: string
  secret_set: boolean
  enabled: boolean
  events?: string[]
  body_template?: string
}

type WebhookType = 'telegram' | 'discord' | 'bark' | 'custom'

const webhookTemplates: Record<WebhookType, { label: string; url: string; name: string }> = {
  telegram: {
    label: 'Telegram',
    name: 'Telegram 通知',
    url: 'https://api.telegram.org/bot<TOKEN>/sendMessage?chat_id=<CHAT_ID>',
  },
  discord: {
    label: 'Discord',
    name: 'Discord 通知',
    url: 'https://discord.com/api/webhooks/<ID>/<TOKEN>',
  },
  bark: {
    label: 'Bark',
    name: 'Bark 通知',
    url: 'https://api.day.app/<KEY>/iCloud 驗證碼',
  },
  custom: {
    label: 'Custom',
    name: '自訂 Webhook',
    url: 'https://example.com/webhook',
  },
}

export default function IntegrationsPage() {
  const [keys, setKeys] = useState<Key[]>([])
  const [webhooks, setWebhooks] = useState<Webhook[]>([])
  const [keyName, setKeyName] = useState('')
  const [newKey, setNewKey] = useState('')
  const [webhookOpen, setWebhookOpen] = useState(false)
  const [webhookType, setWebhookType] = useState<WebhookType>('telegram')
  const [webhookName, setWebhookName] = useState(webhookTemplates.telegram.name)
  const [webhookURL, setWebhookURL] = useState(webhookTemplates.telegram.url)
  const [webhookBody, setWebhookBody] = useState('{\n  "event": {{event}},\n  "subject": {{payload.subject}},\n  "from": {{payload.from}},\n  "to": {{payload.to}},\n  "preview": {{payload.preview}},\n  "verification_code": {{payload.verification_code}}\n}')
  const [saving, setSaving] = useState(false)
  const { show } = useToast()

  async function load() {
    try {
      const [apiKeys, hooks] = await Promise.all([
        request<Key[]>('/api/api-keys'),
        request<Webhook[]>('/api/webhooks'),
      ])
      setKeys(apiKeys)
      setWebhooks(hooks)
    } catch (error) {
      show(error instanceof ApiError ? error.message : '載入設定失敗')
    }
  }

  /* eslint-disable react-hooks/set-state-in-effect, react-hooks/exhaustive-deps */
  useEffect(() => {
    void load()
  }, [])
  /* eslint-enable react-hooks/set-state-in-effect, react-hooks/exhaustive-deps */

  function selectWebhookType(type: WebhookType) {
    setWebhookType(type)
    setWebhookName(webhookTemplates[type].name)
    setWebhookURL(webhookTemplates[type].url)
    setWebhookBody(type === 'custom' ? '{\n  "event": {{event}},\n  "payload": {{payload}}\n}' : '{\n  "event": {{event}},\n  "subject": {{payload.subject}},\n  "from": {{payload.from}},\n  "preview": {{payload.preview}},\n  "verification_code": {{payload.verification_code}}\n}')
  }

  async function createKey() {
    try {
      const data = await request<{ key: string }>('/api/api-keys', {
        method: 'POST',
        body: { name: keyName || 'MCP Client' },
      })
      setNewKey(data.key)
      setKeyName('')
      await load()
    } catch (error) {
      show(error instanceof ApiError ? error.message : '建立 API Key 失敗')
    }
  }

  async function saveWebhook() {
    if (!webhookName.trim() || !webhookURL.trim()) return
    setSaving(true)
    try {
      await request('/api/webhooks', {
        method: 'POST',
        body: {
          name: webhookName.trim(),
          type: webhookType,
          url: webhookURL.trim(),
          events: ['email_received'],
          body_template: webhookBody,
        },
      })
      setWebhookOpen(false)
      show('Webhook 已新增')
      await load()
    } catch (error) {
      show(error instanceof ApiError ? error.message : '新增 Webhook 失敗')
    } finally {
      setSaving(false)
    }
  }

  async function remove(path: string, message: string) {
    try {
      await request(path, { method: 'DELETE' })
      show(message)
      await load()
    } catch (error) {
      show(error instanceof ApiError ? error.message : '操作失敗')
    }
  }

  return (
    <div className="page settings-page">
      <header className="page-head">
        <div>
          <h2 className="page-title">設定</h2>
          <p className="page-sub">管理 API Key、MCP 連線與驗證碼通知。</p>
        </div>
      </header>

      <section className="card settings-card">
        <div className="card-head">
          <div>
            <h3 className="card-title">API Key</h3>
            <p className="card-sub">建立 MCP 用的存取金鑰；原始 Key 只會顯示一次。</p>
          </div>
        </div>
        <div className="settings-inline-form">
          <input
            aria-label="API Key 名稱"
            placeholder="例如：Home Assistant"
            value={keyName}
            onChange={(event) => setKeyName(event.target.value)}
          />
          <button className="primary" onClick={() => void createKey()}>
            新增 API Key
          </button>
        </div>
        {newKey && (
          <div className="alert alert-success settings-secret">
            <span>請立即複製這組 Key：</span>
            <code>{newKey}</code>
          </div>
        )}
        <div className="table-wrap settings-table">
          <table>
            <thead>
              <tr><th>名稱</th><th>建立時間</th><th>狀態</th><th>操作</th></tr>
            </thead>
            <tbody>
              {keys.length === 0 && <tr><td colSpan={4} className="empty-cell">尚未建立 API Key</td></tr>}
              {keys.map((item) => (
                <tr key={item.id}>
                  <td><span className="cell-strong">{item.name}</span><span className="cell-secondary cell-mono">{item.id}</span></td>
                  <td>{new Date(item.created_at).toLocaleString('zh-TW')}</td>
                  <td><span className={item.revoked_at ? 'badge badge-error' : 'badge badge-active'}>{item.revoked_at ? '已撤銷' : '啟用'}</span></td>
                  <td>{!item.revoked_at && <button className="small danger" onClick={() => void remove(`/api/api-keys/${item.id}`, 'API Key 已撤銷')}>撤銷</button>}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <p className="hint settings-footnote">MCP Endpoint：<code>{`${window.location.origin}/api/mcp`}</code> · 使用 <code>Authorization: Bearer &lt;API_KEY&gt;</code></p>
      </section>

      <section className="card settings-card">
        <div className="card-head">
          <div>
            <h3 className="card-title">Webhook</h3>
            <p className="card-sub">收到郵件後，把完整郵件資料依自訂模板送到你選擇的服務。</p>
          </div>
          <button className="primary" onClick={() => setWebhookOpen(true)}>新增 Webhook</button>
        </div>
        <div className="table-wrap settings-table">
          <table>
            <thead>
              <tr><th>名稱</th><th>類型</th><th>網址</th><th>狀態</th><th>操作</th></tr>
            </thead>
            <tbody>
              {webhooks.length === 0 && <tr><td colSpan={5} className="empty-cell">尚未設定 Webhook</td></tr>}
              {webhooks.map((item) => (
                <tr key={item.id}>
                  <td className="cell-strong">{item.name}</td>
                  <td><span className="badge badge-neutral">{item.type}</span></td>
                  <td className="cell-mono settings-url" title={item.url}>{item.url}</td>
                  <td><span className={item.enabled ? 'badge badge-active' : 'badge badge-neutral'}>{item.enabled ? '啟用' : '停用'}</span></td>
                  <td><button className="small danger" onClick={() => void remove(`/api/webhooks/${item.id}`, 'Webhook 已刪除')}>刪除</button></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>

      <Dialog
        title="新增 Webhook"
        description="選擇服務後會自動填入設定範本，你只要替換尖括號內的內容。"
        open={webhookOpen}
        onClose={() => setWebhookOpen(false)}
      >
        <div className="webhook-type-grid" role="radiogroup" aria-label="Webhook 類型">
          {(Object.keys(webhookTemplates) as WebhookType[]).map((type) => (
            <button
              key={type}
              type="button"
              className={webhookType === type ? 'webhook-type is-selected' : 'webhook-type'}
              aria-pressed={webhookType === type}
              onClick={() => selectWebhookType(type)}
            >
              <strong>{webhookTemplates[type].label}</strong>
              <span>{type === 'custom' ? '自訂 JSON Webhook' : '預設通知範本'}</span>
            </button>
          ))}
        </div>
        <div className="form-field">
          <label htmlFor="webhook-name">名稱</label>
          <input id="webhook-name" value={webhookName} onChange={(event) => setWebhookName(event.target.value)} />
        </div>
        <div className="form-field">
          <label htmlFor="webhook-url">Webhook URL</label>
          <input id="webhook-url" value={webhookURL} onChange={(event) => setWebhookURL(event.target.value)} />
        </div>
        <div className="form-field">
          <label htmlFor="webhook-body">請求內容模板（JSON）</label>
          <textarea id="webhook-body" rows={9} value={webhookBody} onChange={(event) => setWebhookBody(event.target.value)} />
          <p className="hint">可用變數：<code>{'{{event}}'}</code>、<code>{'{{timestamp}}'}</code>、<code>{'{{payload}}'}</code>、<code>{'{{payload.subject}}'}</code>、<code>{'{{payload.preview}}'}</code> 等。</p>
        </div>
        <p className="hint">只接受 HTTPS URL。預設事件：收到郵件（email_received）。</p>
        <div className="form-actions">
          <button type="button" onClick={() => setWebhookOpen(false)}>取消</button>
          <button type="button" className="primary" disabled={saving || !webhookName.trim() || !webhookURL.trim()} onClick={() => void saveWebhook()}>
            {saving ? '儲存中…' : '儲存 Webhook'}
          </button>
        </div>
      </Dialog>
    </div>
  )
}
