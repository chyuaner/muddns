package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"muddns/lib/logger"
)

// NamecheapProvider 實作 Namecheap Dynamic DNS API
type NamecheapProvider struct {
	Password string
}

func NewNamecheapProvider(password string) *NamecheapProvider {
	return &NamecheapProvider{Password: password}
}

// UpdateRecord 呼叫 Namecheap DDNS API 更新指定域名
func (p *NamecheapProvider) UpdateRecord(ctx context.Context, domain string, recordType RecordType, ip string, proxied bool) error {
	parts := strings.Split(domain, ".")
	if len(parts) < 2 {
		return fmt.Errorf("無效的域名格式: %s", domain)
	}

	subdomain := strings.Join(parts[:len(parts)-2], ".")
	if subdomain == "" {
		subdomain = "@"
	}
	rootdomain := strings.Join(parts[len(parts)-2:], ".")

	apiURL := fmt.Sprintf("https://dynamicdns.park-your-domain.com/update?host=%s&domain=%s&password=%s&ip=%s",
		subdomain, rootdomain, p.Password, ip)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Namecheap DDNS 請求失敗: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	if strings.Contains(bodyStr, "<ErrCount>0</ErrCount>") {
		logger.Log(logger.SUCCESS, "", "Namecheap 成功更新 %s -> %s", domain, ip)
		return nil
	}

	return fmt.Errorf("Namecheap DDNS 更新失敗，回應內容: %s", bodyStr)
}
