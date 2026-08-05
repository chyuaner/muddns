# muddns - 多主機/多域名 IPv4 & IPv6 DDNS 管理系統

`muddns` 是一款專為 Alpine Linux、Docker 及 Linux 伺服器打造的極致輕量多主機 DDNS 管理系統。支援家用動態 IPv6 前綴拼接、學術網路 (TANet) 公網 IPv4 網段與偏移量計算、以及對稱式 `external_api` 外網查詢。

---

## 🌟 核心特色

- **雙重模式 (Dual Mode)**：支援背景 Web UI 服務 (`serve`) 以及 `crontab` 定時單次觸發 (`sync`)。
- **漸進增強 Web UI (SSR + HTMX)**：
  - **Light Mode / Dark Mode 重新精緻配色**：修正 Light Mode 下輸入框背景色與邊框，呈現極具質感的清爽純白與淡灰 (slate-50/100) 風格，支援系統偏好自動檢測與右上角 `☀️ / 🌙` 切換。
  - **模組化 HTML 範本架構 (`web/templates/`)**：拆分為獨立且邏輯清晰的範本檔案（`header.html`, `footer.html`, `dashboard.html`, `providers.html`, `logs.html`, `settings.html`），方便前端自行編輯維護。
  - **DNS 服務商管理 (Provider Manager)**：支援直接在網頁新增、編輯與刪除 Cloudflare、Namecheap 或 Custom HTTP 服務商金鑰。
  - 無 JS 時相容傳統 HTML `<form>` 表單與老舊瀏覽器；開啟 JS 時支援不刷頁表格編輯、批量勾選與**即時 IP 預覽 (Live Preview)**。
- **IPv4 / IPv6 完全對稱模式**：
  - `external_api`：外網 Echo API 查詢（預設包含 `https://ipv4.yuaner.tw/ip` 與 `https://ipv6.yuaner.tw/ip` 並具備高可用自動備援機制）。
  - `interface`：網卡 IP 讀取與正則/索引 (`@1`, `@2`) 匹配。
  - `base_offset` / `prefix_stitching` / `eui64_mac`：IPv4 網段偏移計算 / IPv6 動態前綴＋固定後綴拼接 / EUI-64 MAC 自動轉換。
- **模組化 DNS 驅動器**：
  - **Cloudflare**（使用官方 `github.com/cloudflare/cloudflare-go` SDK，支援 `proxied` 開關）。
  - **Namecheap**（標準動態 DNS API）。
  - **Custom HTTP**（萬能 GET/POST 請求，支援 `#{ip}`, `#{domain}`, `#{subdomain}`, `#{type}` 樣板變數與正則回應驗證）。
- **完善日誌與除錯**：頂部選單固定 Log 連結、主機列表 `[📜 查看日誌]` 快捷過濾、內建環形記憶體日誌池 (Ring Buffer) 與 `--debug` 模式。
- **零依賴與極致輕量**：單一 Golang 二進位檔 (CGO_ENABLED=0，RAM < 15MB)，單一 `config.yaml` 檔，自動同步 Linux 作業系統層級 `/etc/ssl/certs` CA 白名單。

---

## 🚀 快速開始

### 1. 編譯與安裝
```bash
cd /home/yuan/Documents/Git/Yuan/muddns
go build -o muddns main.go embed.go
```

第一次執行時，系統會自動檢測並從 `config.sample.yaml` 自動複製生成 `config.yaml`。
`config.yaml` 已加入 `.gitignore`，確保您的 API Token 與密碼不會意外提交至版本庫。

### 2. 常用命令

#### 啟動背景 Web UI 服務 (預設監聽 :9876)
```bash
./muddns serve -c config.yaml
```

#### 單次觸發同步 (Crontab / 腳本專用)
```bash
./muddns sync -c config.yaml
```

#### Dry-run 測試與排錯 (計算 IP 但不上傳 DNS)
```bash
./muddns status
```

#### 開啟詳細除錯模式
```bash
./muddns sync -c config.yaml --debug
```

---

## 📅 Crontab 設定範例

若希望完全關閉背景服務，每 5 分鐘由 `cron` 觸發單次更新：
```bash
*/5 * * * * /usr/local/bin/muddns sync -c /etc/muddns/config.yaml > /dev/null 2>&1
```

---

## 📄 `config.yaml` 設定範例

```yaml
settings:
  listen: ":9876"
  interval_seconds: 300
  web_auth:
    enabled: true
    username: "admin"
    password_hash: "$2b$10$5jtmdEsjq/e.RLoFzUPRy.o1GBmDxYIWV94BaQoBWD0/Y7/JnaENC" # 預設密碼: admin

defaults:
  ipv4_apis:
    - "https://ipv4.yuaner.tw/ip"
    - "https://api.ipify.org"
  ipv6_apis:
    - "https://ipv6.yuaner.tw/ip"
    - "https://api6.ipify.org"

providers:
  cf_main:
    provider: "cloudflare"
    api_token: "your-cloudflare-api-token"
    zone_id: "your-cloudflare-zone-id"

hosts:
  - id: "host-nas"
    name: "家庭 NAS"
    enabled: true
    provider: "cf_main"
    domains:
      - "nas.yourdomain.com"
    proxied: false
    ipv4:
      enabled: true
      mode: "external_api"
    ipv6:
      enabled: true
      mode: "prefix_stitching"
      interface: "eth0"
      suffix: "::100"
```
