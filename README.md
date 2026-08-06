# muddns - 多主機/多域名 IPv4 & IPv6 DDNS 管理系統

`muddns` 是一款專為 Alpine Linux、Docker 及 Linux 伺服器打造的極致輕量多主機 DDNS 管理系統。支援家用動態 IPv6 前綴拼接、學術網路 (TANet) 公網 IPv4 網段與偏移量計算、以及對稱式 `external_api` 外網查詢。

---

## 💡 本專案解決痛點

現行大部分常見的 DDNS 工具（如 ddns-go、Inadyn 等）大多設計為**「單機自我更新模式」**——即預設僅能取得並更新「執行該程式當下本機」的公網 IP。然而在現代網路與家庭實驗室 (HomeLab) 環境中，這種設計會遇到以下兩大重大痛點：

1. **IPv6 前綴變更與內網多主機批量更新痛點 (SLAAC / Prefix Delegation)**：
   家用動態 IPv6（如中華電信 PPPoE 浮動 `/56` 或 `/64` 前綴）每次重撥都會改變前綴，而內網中的 NAS、PVE 虛擬機、樹莓派與 Docker 容器皆各自擁有獨立的公網 IPv6 位址。若為每台裝置分別安裝用戶端，不僅維護極度繁瑣，且在 SLAAC 環境下難以運作。
   👉 **`muddns` 的解法**：只需在路由器/閘道器（如 OPNsense / RouterOS）或單一核心伺服器上部署 `muddns`，即可自動擷取 WAN 動態前綴，並透過「固定後綴拼接 (`prefix_stitching`)」、「EUI-64 MAC 自動換算 (`eui64_mac`)」或「ARP 鄰居表探測」，**由單一服務集中計算並批量更新內網所有 IPv6 裝置的 DNS 紀錄**。

2. **跨主機與多網卡集中化管理需求 (Centralized Multi-Host DDNS)**：
   傳統工具無法在一台機器上集中控管分散在不同網卡 (Dual WAN / PPPoE)、不同子網段甚至遠端節點的多台主機 DNS。
   👉 **`muddns` 的解法**：提供集中式主機清單（支援 OPNsense 風格 CSV 批量匯入/匯出）、指定網卡介面獨立觸發 (`-i pppoe1` / `?interface=pppoe1`) 與自訂 Bash 指令模式 (`command`)，**讓單一 `muddns` 實例成為全網的集中化 DDNS 控制中心**。

---

## 🌟 核心特色

- **雙重與多維觸發模式 (Dual & Multi-Trigger Mode)**：
  - **背景 Web UI 輪詢 (`serve`)**：常駐背景服務並提供 GUI 管理介面。
  - **CLI / Crontab 單次同步 (`sync`)**：支援指定特定 Host (`-h`) 或指定網卡介面 (`-i pppoe0`) 獨立觸發。
  - **🌐 HTTP API 遠端觸發 (`GET / POST /api/sync`)**：專為 OPNsense / pfSense / RouterOS 重撥或 WAN 介面重連事件 (devd / newwanip) 設計，支援 `curl` 傳入驗證 Token (`auth`) 與網卡介面過濾 (`interface=pppoe1`)。
  - **⚙️ Linux Systemd 常駐服務 (`service install / uninstall`)**：比照 `ddns-go` 支援一鍵將 muddns 註冊、啟動或移除為 Linux 常駐服務。
- **純淨初始化 (Zero-Dependency Auto-Init)**：未檢測到 `config.yaml` 時由程式自動生成乾淨極簡的純淨設定檔（預設 `admin/admin` 帳號密碼），不預填示範主機，`config.sample.yaml` 轉為純粹參考範例。
- **全新 Bash 指令模式 (`command`)**：支援填入自訂 Shell/Bash 命令（例如 `curl`, `ip addr`, `awk` 等），自動解析 stdout 輸出作為 IPv4 或 IPv6 位址。
- **漸進增強 Web UI (SSR + HTMX + Vanilla CSS)**：
  - **無 JS 降級完全相容 (No-JS Fallback)**：所有選單、對話框與表單均可在關閉 JavaScript 的環境下原生運作！對話框關閉/取消按鈕使用原生超連結 `<a>`，並透過 `.js-only` 自動於無 JS 環境下隱藏純 JS 互動按鈕。
  - **舊版瀏覽器友善 (Progressive Enhancement)**：針對不支援 HTML5 `<dialog>` 標籤的老舊瀏覽器（如舊版 Safari、Android Webview），自動透過漸進降級 CSS 轉為頂部卡片樣式呈現，確保 100% 瀏覽器跨平台相容。
  - **PJAX / View Transitions 全站動態換頁**：全站導入 HTMX `hx-boost` 與 CSS View Transitions API，提供媲美 SPA 單頁應用的滑順換頁體驗，無 JS 時自動無縫降級為標準 SSR。
  - **HTML5 `<dialog>` 彈窗對話框**：新增與編輯主機、Provider 管理均採用原生 HTML5 `<dialog>` 對話框，具備背景毛玻璃模糊 (Backdrop Blur) 與跳出動畫，且編輯時完整保留背景表格。
  - **OPNsense 風格可收合群組表格**：Dashboard 主機清單自動依 DNS 服務商分類群組化，支援可收合式表格列與視覺主題優化。
  - **全域系統設定頁面 (`/settings`) & RAW Config 編輯器 (`/config/raw`)**：可在 Web UI 調整 `config.yaml` 的所有全域 settings 參數（監聽埠、輪詢秒數、API Token、TLS 跳過驗證、日誌路徑、WebAuth 等）或直接編輯 YAML 原文。
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
# 進入專案目錄
cd muddns

# 1. 下載並安裝第三方依賴套件 (相當於 npm install)
go mod download

# 2. 編譯為單一二進位執行檔
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
# 同步更新全部啟用的主機
./muddns sync -c config.yaml

# 指定僅同步特定 Host ID
./muddns sync -c config.yaml -h host-nas
```

#### 🔀 雙 WAN 網卡獨立同步 (-i, --interface)
適合搭配 OPNsense / pfSense / RouterOS 重撥或介面重連事件觸發：
```bash
# 僅同步綁定 pppoe0 網卡介面的主機 (沒綁定 pppoe0 的主機自動跳過)
./muddns sync -c config.yaml -i pppoe0

# 完整參數寫法
./muddns sync -c config.yaml --interface pppoe1
```

#### 🌐 HTTP API 遠端觸發介面 (GET / POST /api/sync)
適合由 OPNsense WAN 重新連線事件 (devd / newwanip) 或遠端 curl 呼叫觸發：
```bash
# 1. 觸發更新指定網卡介面 (例如 pppoe1 重新連線時)
curl -s "http://127.0.0.1:9876/api/sync?interface=pppoe1&auth=5F9KRRh71FRcAYzgr1HSPnRIy02ueVle6ZabR7cua7ca46d0"

# 2. 未指定 interface 時，更新全部主機
curl -s "http://127.0.0.1:9876/api/sync?auth=5F9KRRh71FRcAYzgr1HSPnRIy02ueVle6ZabR7cua7ca46d0"

# 3. 亦支援 POST 方法與 Authorization Header 標頭
curl -X POST "http://127.0.0.1:9876/api/sync" \
     -H "Authorization: Bearer 5F9KRRh71FRcAYzgr1HSPnRIy02ueVle6ZabR7cua7ca46d0" \
     -d "interface=pppoe1"
```

#### ⚙️ Linux Systemd 常駐服務安裝 (比照 ddns-go)
```bash
# 自動將 muddns 註冊並啟動為 Systemd 常駐服務 (自動帶入絕對路徑)
sudo ./muddns service install -c /etc/muddns/config.yaml

# 快捷寫法 (同 service install)
sudo ./muddns install

# 管理服務狀態 (status, start, stop, restart)
sudo ./muddns service status

# 解除安裝並移除常駐服務
sudo ./muddns service uninstall
# 或
sudo ./muddns uninstall
```

#### Dry-run 測試與排錯 (計算 IP 但不上傳 DNS)
```bash
# 乾跑計算全部主機 IP
./muddns status

# 僅乾跑檢測指定網卡介面的主機
./muddns status -i eth0
```

#### 開啟詳細除錯模式
```bash
./muddns sync -c config.yaml --debug
```

---

## 📅 Crontab 與 OPNsense 整合範例

### 1. Crontab 定時排程
若希望完全關閉背景服務，每 5 分鐘由 `cron` 觸發單次更新：
```bash
*/5 * * * * /usr/local/bin/muddns sync -c /etc/muddns/config.yaml > /dev/null 2>&1
```

### 2. OPNsense / pfSense 雙 WAN 介面重連事件腳本 (WAN Reconnect Event Hook)
當雙 WAN 網路中特定介面（如 `pppoe0`）斷線重連時，在腳本中呼叫帶 `-i` 參數的指令，即可精準僅更新該 WAN 關聯的主機：
```bash
#!/bin/sh
# $1 為網卡介面名稱 (例如 pppoe0)
INTERFACE=$1

if [ -n "$INTERFACE" ]; then
    /usr/local/bin/muddns sync -c /etc/muddns/config.yaml -i "$INTERFACE"
else
    /usr/local/bin/muddns sync -c /etc/muddns/config.yaml
fi
```

---

## 📄 `config.yaml` 設定範例

```yaml
settings:
  listen: ":9876"
  interval_seconds: 300
  default_ipv4_interface: "eth0" # 預設 IPv4 網卡介面
  default_ipv6_interface: "eth0" # 預設 IPv6 網卡介面
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
