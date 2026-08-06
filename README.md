# muddns - 多主機/多域名 IPv4 & IPv6 DDNS 管理系統

`muddns` 是一款專為 Alpine Linux、Docker 及 Linux 伺服器打造的極致輕量多主機 DDNS 管理系統。支援家用動態 IPv6 前綴拼接、學術網路 (TANet) 公網 IPv4 網段與偏移量計算、以及對稱式 `external_api` 外網查詢。

---

## 🌟 核心特色

- **雙重模式 (Dual Mode)**：支援背景 Web UI 服務 (`serve`) 以及 `crontab` 定時單次觸發 (`sync`)。
- **純淨初始化 (Zero-Dependency Auto-Init)**：未檢測到 `config.yaml` 時由程式自動生成乾淨極簡的純淨設定檔（預設 `admin/admin` 帳號密碼），不預填示範主機，`config.sample.yaml` 轉為純粹參考範例。
- **全新 Bash 指令模式 (`command`)**：支援填入自訂 Shell/Bash 命令（例如 `curl`, `ip addr`, `awk` 等），自動解析 stdout 輸出作為 IPv4 或 IPv6 位址。
- **漸進增強 Web UI (SSR + HTMX)**：
  - **Dashboard 可展開主機詳細列 (`[▼ 展開]`)**：點擊一鍵展開顯示該主機完整的 IPv4/IPv6 模式、介面、正則/指令、最新算得 IP 與時間。
  - **新增大量主機欄位重整與動態動態表頭**：
    - 「主機啟用狀態」置於表格最左側欄位，「Cloudflare Proxied」置於「刪除」按鈕左側。
    - 當切換 IPv4/IPv6 模式時，表格標題（如 `IPv4 參數 (Bash Command)` 或 `IPv6 參數 (Suffix)`）會動態隨模式名稱更新。
  - **RAW Config 編輯器 (`/config/raw`)**：可在網頁上直接編輯與儲存 `config.yaml` 原始 YAML 內容（內建 YAML 語法解析校驗防錯）。
  - **DNS Provider 分組展延 (Grouped Dashboard)**：Dashboard 主機清單自動依 DNS 服務商分類群組化，支援可收合式卡片 (`<details open>`) 檢視。
  - **OPNsense 風格 CSV 匯入/匯出 (`/hosts/export.csv`, `/hosts/import.csv`)**：支援一鍵下載完整 CSV 清單或批量貼上/上傳 CSV 快速導入多主機。
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
go build -o muddns .
```

第一次執行時，系統會自動檢測並產生純淨極簡的 `config.yaml`（預設帳號密碼 `admin` / `admin`）。
`config.yaml` 已加入 `.gitignore`，確保您的 API Token 與密碼不會意外提交至版本庫。

---

## 📂 專案模組架構

```text
muddns/
├── main.go               # 🎯 專案進入點 (僅 15 行，呼叫 cli 模組)
├── cli/                  # 💻 CLI 命令解析器 (serve, sync, status, version)
│   └── cli.go
├── config/               # ⚙️ 設定檔 YAML 載入、儲存與 CSV 匯入匯出
│   └── config.go
├── lib/                  # 📚 核心功能與工具庫
│   ├── ipfetcher/        # IP 計算與取得 (API, 網卡, Bash 指令, 前綴拼接, EUI-64)
│   ├── provider/         # DNS 服務商更新 API (Cloudflare, Namecheap, Custom HTTP)
│   └── logger/           # 記憶體環狀日誌與 Console 輸出
└── web/                  # 🌐 Web UI 服務與控制層 (遵循 Go 主流標準慣例)
    ├── types.go          # 📐 型別與結構體定義 (Server, PageData, BatchRow)
    ├── server.go         # 🏗️ WebServer 生命週期管理 (Start & Shutdown)
    ├── router.go         # 🛣️ 路由註冊 (Go 1.22+ 原生 "GET /path" / "POST /path" 標註)
    ├── handlers.go       # 🎮 控制器處理常式 (Request Handlers / Controllers)
    ├── embed.go          # 📦 靜態資產打包 (Go 1.16+ 原生 go:embed 特性)
    └── templates/        # 🎨 HTML 視圖範本檔案 (dashboard, providers, raw config...)
```

### 💡 Web 模組對照 MVC 概念說明

對於有傳統 Web 框架（Laravel / Express / Rails）經驗的開發者，本專案 `/web` 模組可以輕鬆對應至 MVC 架構：

| Go 檔案 / 目錄 | 傳統 MVC 角色 | 職責與說明 |
| :--- | :--- | :--- |
| **`web/types.go`** | **Model (ViewModel)** | 定義傳遞給 HTML 前端視圖渲染的資料結構 (`PageData`, `BatchRow`)。 |
| **`web/templates/`** | **View (視圖層)** | 拆分的 HTML 視圖範本（`header.html`, `dashboard.html`, `providers.html` 等）。 |
| **`web/handlers.go`** | **Controller (控制器)** | 處理 HTTP 請求、呼叫算 IP / 儲存 YAML 邏輯，並將結果傳遞給視圖渲染。 *(Go 社群慣稱為 Handler)* |
| **`web/router.go`** | **Router & Middleware** | 宣告 URL 路由表（採用 Go 1.22+ `"GET /path"` / `"POST /path"` 語法）與 BasicAuth 權限過濾器。 |
| **`web/embed.go`** | **Asset Bundler** | 使用 Go 1.16+ `//go:embed` 標籤，在編譯時期將 `templates/` 目錄打入單一二進位執行檔。 |
| **`web/server.go`** | **App Server** | 管理 Web 伺服器的監聽啟動 (`Start`) 與安全平滑關閉 (`Shutdown`)。 |

---

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

## 🌟 開發備註

1. **極簡的編譯與執行**：
   * 由於 `main.go` 已經獨立為唯一的入口，可以直接編譯整個目錄：
     ```bash
     go build -o muddns .
     ```
   * 開發時也可直接免編譯執行：
     ```bash
     go run . serve
     ```
2. **`web/` 模組清晰三分**：
   * **[router.go](./web/router.go)**：專門用來查看整個 Web UI 的網址路由地圖（如 `/hosts/save` 對應哪個 Handler）。
   * **[handlers.go](./web/handlers.go)**：專門撰寫每個網頁頁面或 API 送出表單後的邏輯處理。
   * **[server.go](./web/server.go)**：宣告傳遞給前端範本的資料結構。
3. **`lib/` 內部工具庫集中化**：
   * 將 `ipfetcher`、`provider` 與 `logger` 統一整理入 [lib/](./lib) 資料夾中。
