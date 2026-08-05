package config

import (
	"os"

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
	// If config file doesn't exist, try auto-generating from config.sample.yaml
	if _, err := os.Stat(path); os.IsNotExist(err) {
		samplePath := "config.sample.yaml"
		if sampleData, sampleErr := os.ReadFile(samplePath); sampleErr == nil {
			if writeErr := os.WriteFile(path, sampleData, 0644); writeErr == nil {
				// Successfully initialized config.yaml from config.sample.yaml
			}
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
