// Package config 負責 muddns 設定檔 (config.yaml) 的載入、儲存、純淨初始化與 CSV 匯入匯出。
package config

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
)

// Config 代表整份 muddns 系統設定架構
type Config struct {
	Settings  Settings            `yaml:"settings"`  // 系統全域設定
	Defaults  Defaults            `yaml:"defaults"`  // 預設 Echo API 清單
	Providers map[string]Provider `yaml:"providers"` // 註冊的 DNS 服務商 Key-Value 對
	Hosts     []Host              `yaml:"hosts"`     // 託管的主機與域名策略清單
}

// Settings 系統全域參數設定
type Settings struct {
	Listen             string  `yaml:"listen"`               // Web UI 監聽埠 (例: :9876)
	IntervalSeconds    int     `yaml:"interval_seconds"`     // 輪詢更新間隔秒數 (例: 300)
	CustomCAFile       string  `yaml:"custom_ca_file"`       // 自訂 CA 憑證檔案路徑
	InsecureSkipVerify bool    `yaml:"insecure_skip_verify"` // 是否跳過 TLS 憑證驗證
	LogFile            string  `yaml:"log_file"`             // 實體日誌檔案路徑
	WebAuth            WebAuth `yaml:"web_auth"`             // Web UI 帳號密碼認證設定
}

// WebAuth Web 管理介面認證參數
type WebAuth struct {
	Enabled      bool   `yaml:"enabled"`       // 是否啟用認證
	Username     string `yaml:"username"`      // 管理員帳號
	PasswordHash string `yaml:"password_hash"` // Bcrypt 加密後的密碼 Hash
}

// Defaults 預設外網 Echo API 地址清單
type Defaults struct {
	IPv4APIs []string `yaml:"ipv4_apis"` // 預設 IPv4 查詢 API
	IPv6APIs []string `yaml:"ipv6_apis"` // 預設 IPv6 查詢 API
}

// Provider DNS 服務商連線金鑰與參數設定
type Provider struct {
	Provider          string            `yaml:"provider"`                     // 服務商類型: cloudflare, namecheap, custom_http
	APIToken          string            `yaml:"api_token,omitempty"`          // Cloudflare API Token
	ZoneID            string            `yaml:"zone_id,omitempty"`            // Cloudflare Zone ID (可選)
	Password          string            `yaml:"password,omitempty"`           // Namecheap DDNS 密碼
	Method            string            `yaml:"method,omitempty"`             // Custom HTTP Method (GET/POST/PUT)
	URL               string            `yaml:"url,omitempty"`                // Custom HTTP URL 樣板
	Headers           map[string]string `yaml:"headers,omitempty"`            // Custom HTTP Headers
	Body              string            `yaml:"body,omitempty"`               // Custom HTTP Body 樣板
	ExpectedStatus    int               `yaml:"expected_status,omitempty"`    // Custom HTTP 預期 Status Code
	ExpectedBodyRegex string            `yaml:"expected_body_regex,omitempty"` // Custom HTTP 預期 Response 正則
}

// Host 單一主機託管設定
type Host struct {
	ID       string   `yaml:"id"`       // 主機唯一識別 ID (例: host-nas)
	Name     string   `yaml:"name"`     // 主機顯示名稱 (例: 家庭 NAS)
	Enabled  bool     `yaml:"enabled"`  // 是否啟用更新
	Provider string   `yaml:"provider"` // 綁定的 DNS Provider ID
	Domains  []string `yaml:"domains"`  // 綁定的完整域名列表
	Proxied  bool     `yaml:"proxied"`  // Cloudflare 橘色雲朵代理設定
	IPv4     IPConfig `yaml:"ipv4"`     // IPv4 更新策略
	IPv6     IPConfig `yaml:"ipv6"`     // IPv6 更新策略
}

// IPConfig 單一 IP 類型的更新策略
type IPConfig struct {
	Enabled   bool   `yaml:"enabled"`             // 是否啟用該 IP 類型
	Mode      string `yaml:"mode"`                // 模式: external_api, interface, base_offset, arp_mac, prefix_stitching, eui64_mac, command
	URL       string `yaml:"url,omitempty"`       // 外網 API URL
	Interface string `yaml:"interface,omitempty"` // 網卡介面名稱 (如 eth0)
	Match     string `yaml:"match,omitempty"`     // 正則表達式或索引 (@1, @2)
	Offset    string `yaml:"offset,omitempty"`    // 數值偏移量
	Suffix    string `yaml:"suffix,omitempty"`    // 固定 IPv6 後綴 (如 ::100)
	MAC       string `yaml:"mac,omitempty"`       // MAC 地址
	Command   string `yaml:"command,omitempty"`   // 自訂 Bash 指令
	LastIP    string `yaml:"last_ip,omitempty"`   // 上次成功更新的 IP (快取)
}

// LoadConfig 自指定路徑載入 config.yaml。若檔案不存在，會自動生成純淨的預設 config.yaml。
func LoadConfig(path string) (*Config, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		defaultHash, _ := HashPassword("admin")
		defaultCfg := &Config{
			Settings: Settings{
				Listen:          ":9876",
				IntervalSeconds: 300,
				WebAuth: WebAuth{
					Enabled:      true,
					Username:     "admin",
					PasswordHash: defaultHash,
				},
			},
			Defaults: Defaults{
				IPv4APIs: []string{
					"https://ipv4.yuaner.tw/ip",
					"https://api.ipify.org",
					"https://v4.ident.me",
				},
				IPv6APIs: []string{
					"https://ipv6.yuaner.tw/ip",
					"https://api6.ipify.org",
					"https://v6.ident.me",
				},
			},
			Providers: make(map[string]Provider),
			Hosts:     []Host{},
		}
		if data, err := yaml.Marshal(defaultCfg); err == nil {
			_ = os.WriteFile(path, data, 0644)
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	if cfg.Settings.Listen == "" {
		cfg.Settings.Listen = ":9876"
	}
	if cfg.Settings.IntervalSeconds <= 0 {
		cfg.Settings.IntervalSeconds = 300
	}

	return cfg, nil
}

// Save 將當前 Config 結構實體寫入回實體 YAML 檔案
func (c *Config) Save(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// VerifyPassword 驗證密碼是否符合 Bcrypt Hash
func (w *WebAuth) VerifyPassword(password string) bool {
	if !w.Enabled {
		return true
	}
	err := bcrypt.CompareHashAndPassword([]byte(w.PasswordHash), []byte(password))
	return err == nil
}

// HashPassword 將明文密碼使用 Bcrypt 演算法加密成 Hash
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// ExportCSV 將系統內所有主機設定導出為 OPNsense 風格的 CSV 字串
func (c *Config) ExportCSV() (string, error) {
	var sb strings.Builder
	sb.WriteString("id,name,enabled,provider,domains,proxied,ipv4_enabled,ipv4_mode,ipv4_interface,ipv4_val,ipv6_enabled,ipv6_mode,ipv6_interface,ipv6_val\n")

	for _, h := range c.Hosts {
		domainStr := strings.Join(h.Domains, ";")
		
		v4Val := h.IPv4.Match
		if h.IPv4.Offset != "" { v4Val = h.IPv4.Offset }
		if h.IPv4.MAC != "" { v4Val = h.IPv4.MAC }
		if h.IPv4.URL != "" { v4Val = h.IPv4.URL }
		if h.IPv4.Command != "" { v4Val = h.IPv4.Command }

		v6Val := h.IPv6.Suffix
		if h.IPv6.MAC != "" { v6Val = h.IPv6.MAC }
		if h.IPv6.Match != "" { v6Val = h.IPv6.Match }
		if h.IPv6.URL != "" { v6Val = h.IPv6.URL }
		if h.IPv6.Command != "" { v6Val = h.IPv6.Command }

		line := fmt.Sprintf(
			"%s,%s,%t,%s,%s,%t,%t,%s,%s,%s,%t,%s,%s,%s\n",
			h.ID, h.Name, h.Enabled, h.Provider, domainStr, h.Proxied,
			h.IPv4.Enabled, h.IPv4.Mode, h.IPv4.Interface, v4Val,
			h.IPv6.Enabled, h.IPv6.Mode, h.IPv6.Interface, v6Val,
		)
		sb.WriteString(line)
	}

	return sb.String(), nil
}

// ImportCSV 解析傳入的 CSV 字串並寫入主機設定清單中
func (c *Config) ImportCSV(csvContent string) (int, error) {
	lines := strings.Split(csvContent, "\n")
	if len(lines) < 2 {
		return 0, fmt.Errorf("CSV 內容無有效資料列")
	}

	count := 0
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if i == 0 || line == "" {
			continue
		}

		parts := strings.Split(line, ",")
		if len(parts) < 14 {
			continue
		}

		id := strings.TrimSpace(parts[0])
		name := strings.TrimSpace(parts[1])
		enabled := strings.TrimSpace(parts[2]) == "true"
		provider := strings.TrimSpace(parts[3])
		domains := strings.Split(strings.TrimSpace(parts[4]), ";")
		proxied := strings.TrimSpace(parts[5]) == "true"

		v4Enabled := strings.TrimSpace(parts[6]) == "true"
		v4Mode := strings.TrimSpace(parts[7])
		v4Iface := strings.TrimSpace(parts[8])
		v4Val := strings.TrimSpace(parts[9])

		v6Enabled := strings.TrimSpace(parts[10]) == "true"
		v6Mode := strings.TrimSpace(parts[11])
		v6Iface := strings.TrimSpace(parts[12])
		v6Val := strings.TrimSpace(parts[13])

		v4Cfg := IPConfig{
			Enabled:   v4Enabled,
			Mode:      v4Mode,
			Interface: v4Iface,
		}
		switch v4Mode {
		case "external_api": v4Cfg.URL = v4Val
		case "interface": v4Cfg.Match = v4Val
		case "base_offset": v4Cfg.Offset = v4Val
		case "arp_mac": v4Cfg.MAC = v4Val
		case "command": v4Cfg.Command = v4Val
		}

		v6Cfg := IPConfig{
			Enabled:   v6Enabled,
			Mode:      v6Mode,
			Interface: v6Iface,
		}
		switch v6Mode {
		case "prefix_stitching": v6Cfg.Suffix = v6Val
		case "eui64_mac": v6Cfg.MAC = v6Val
		case "interface": v6Cfg.Match = v6Val
		case "external_api": v6Cfg.URL = v6Val
		case "command": v6Cfg.Command = v6Val
		}

		host := Host{
			ID:       id,
			Name:     name,
			Enabled:  enabled,
			Provider: provider,
			Domains:  domains,
			Proxied:  proxied,
			IPv4:     v4Cfg,
			IPv6:     v6Cfg,
		}

		found := false
		for idx, existing := range c.Hosts {
			if existing.ID == id {
				c.Hosts[idx] = host
				found = true
				break
			}
		}
		if !found {
			c.Hosts = append(c.Hosts, host)
		}
		count++
	}

	return count, nil
}
