// Package web 包含 Web UI 伺服器型別定義、路由註冊、Handler 與資產打包。
package web

import (
	"html/template"

	"muddns/config"
	"muddns/lib/logger"
)

// Server 代表 Web UI 伺服器的核心控制器結構
type Server struct {
	cfg        *config.Config
	configPath string
	tmpl       *template.Template
}

// PageData 代表傳遞給 HTML 範本 (View) 渲染的強型別資料結構
type PageData struct {
	ActiveTab           string                     `json:"active_tab"`            // 當前啟用的頁籤 (dashboard, providers, config_raw, logs, settings)
	Hosts               []config.Host              `json:"hosts"`                 // 所有主機清單
	GroupedHosts        map[string][]config.Host   `json:"grouped_hosts"`         // 依 Provider 分組的主機清單
	Providers           map[string]config.Provider `json:"providers"`             // 所有註冊的 DNS 服務商 Key-Value 對
	EditHost            *config.Host               `json:"edit_host"`             // 當前單一編輯的主機
	EditProvider        *config.Provider           `json:"edit_provider"`         // 當前單一編輯的 Provider
	EditProviderID      string                     `json:"edit_provider_id"`      // 編輯的 Provider ID
	RawYAML             string                     `json:"raw_yaml"`              // RAW YAML 文字檔內文
	IsBatchEdit         bool                       `json:"is_batch_edit"`         // 是否為批量編輯模式
	CommonProvider      string                     `json:"common_provider"`       // 批量通用 Provider
	CommonIPv4Enabled   bool                       `json:"common_ipv4_enabled"`   // 批量通用 IPv4 啟用狀態
	CommonIPv4Mode      string                     `json:"common_ipv4_mode"`      // 批量通用 IPv4 模式
	CommonIPv4Interface string                     `json:"common_ipv4_interface"` // 批量通用 IPv4 介面
	CommonIPv6Enabled   bool                       `json:"common_ipv6_enabled"`   // 批量通用 IPv6 啟用狀態
	CommonIPv6Mode      string                     `json:"common_ipv6_mode"`      // 批量通用 IPv6 模式
	CommonIPv6Interface string                     `json:"common_ipv6_interface"` // 批量通用 IPv6 介面
	BatchRows           []BatchRow                 `json:"batch_rows"`            // 批量編輯的主機資料列
	Logs                []logger.LogEntry          `json:"logs"`                  // 系統日誌列表
	WebAuth             config.WebAuth             `json:"web_auth"`              // 系統認證設定
	Message             string                     `json:"message"`               // 前端提示訊息 (綠色/黃色)
	Error               string                     `json:"error"`                 // 前端錯誤訊息 (紅色)
}

// BatchRow 代表大量主機表格中的單一主機輸入列
type BatchRow struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Enabled   bool   `json:"enabled"`
	DomainStr string `json:"domain_str"`
	IPv4Val   string `json:"ipv4_val"`
	IPv6Val   string `json:"ipv6_val"`
	Proxied   bool   `json:"proxied"`
}
