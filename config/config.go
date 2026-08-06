package config

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Settings  Settings            `yaml:"settings"`
	Defaults  Defaults            `yaml:"defaults"`
	Providers map[string]Provider `yaml:"providers"`
	Hosts     []Host              `yaml:"hosts"`
}

type Settings struct {
	Listen             string  `yaml:"listen"`
	IntervalSeconds    int     `yaml:"interval_seconds"`
	CustomCAFile       string  `yaml:"custom_ca_file"`
	InsecureSkipVerify bool    `yaml:"insecure_skip_verify"`
	LogFile            string  `yaml:"log_file"`
	WebAuth            WebAuth `yaml:"web_auth"`
}

type WebAuth struct {
	Enabled      bool   `yaml:"enabled"`
	Username     string `yaml:"username"`
	PasswordHash string `yaml:"password_hash"`
}

type Defaults struct {
	IPv4APIs []string `yaml:"ipv4_apis"`
	IPv6APIs []string `yaml:"ipv6_apis"`
}

type Provider struct {
	Provider          string            `yaml:"provider"` // cloudflare, namecheap, custom_http
	APIToken          string            `yaml:"api_token,omitempty"`
	ZoneID            string            `yaml:"zone_id,omitempty"`
	Password          string            `yaml:"password,omitempty"`
	Method            string            `yaml:"method,omitempty"`
	URL               string            `yaml:"url,omitempty"`
	Headers           map[string]string `yaml:"headers,omitempty"`
	Body              string            `yaml:"body,omitempty"`
	ExpectedStatus    int               `yaml:"expected_status,omitempty"`
	ExpectedBodyRegex string            `yaml:"expected_body_regex,omitempty"`
}

type Host struct {
	ID       string   `yaml:"id"`
	Name     string   `yaml:"name"`
	Enabled  bool     `yaml:"enabled"`
	Provider string   `yaml:"provider"`
	Domains  []string `yaml:"domains"`
	Proxied  bool     `yaml:"proxied"`
	IPv4     IPConfig `yaml:"ipv4"`
	IPv6     IPConfig `yaml:"ipv6"`
}

type IPConfig struct {
	Enabled   bool   `yaml:"enabled"`
	Mode      string `yaml:"mode"` // external_api, interface, base_offset, arp_mac, prefix_stitching, eui64_mac, command
	URL       string `yaml:"url,omitempty"`
	Interface string `yaml:"interface,omitempty"`
	Match     string `yaml:"match,omitempty"`
	Offset    string `yaml:"offset,omitempty"`
	Suffix    string `yaml:"suffix,omitempty"`
	MAC       string `yaml:"mac,omitempty"`
	Command   string `yaml:"command,omitempty"`
	LastIP    string `yaml:"last_ip,omitempty"`
}

func LoadConfig(path string) (*Config, error) {
	// If config file doesn't exist, create a clean minimal default config directly from code
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
	if len(cfg.Defaults.IPv4APIs) == 0 {
		cfg.Defaults.IPv4APIs = []string{
			"https://ipv4.yuaner.tw/ip",
			"https://api.ipify.org",
			"https://v4.ident.me",
			"https://checkip.amazonaws.com",
		}
	}
	if len(cfg.Defaults.IPv6APIs) == 0 {
		cfg.Defaults.IPv6APIs = []string{
			"https://ipv6.yuaner.tw/ip",
			"https://api6.ipify.org",
			"https://v6.ident.me",
			"https://6.ipw.cn",
		}
	}

	return cfg, nil
}

func (c *Config) Save(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (w *WebAuth) VerifyPassword(password string) bool {
	if !w.Enabled {
		return true
	}
	err := bcrypt.CompareHashAndPassword([]byte(w.PasswordHash), []byte(password))
	return err == nil
}

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

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

func (c *Config) ImportCSV(csvContent string) (int, error) {
	lines := strings.Split(csvContent, "\n")
	if len(lines) < 2 {
		return 0, fmt.Errorf("CSV 內容無有效資料列")
	}

	count := 0
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if i == 0 || line == "" {
			continue // skip header
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

		// Update or append
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
