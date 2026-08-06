package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"muddns/config"
	"muddns/lib/logger"
)

// CustomHTTPProvider 實作通用 HTTP GET/POST/PUT Webhook 更新介面
type CustomHTTPProvider struct {
	Config config.Provider
}

func NewCustomHTTPProvider(p config.Provider) *CustomHTTPProvider {
	return &CustomHTTPProvider{Config: p}
}

// UpdateRecord 替換 URL / Body 樣板變數並發送自訂 HTTP 請求
func (p *CustomHTTPProvider) UpdateRecord(ctx context.Context, domain string, recordType RecordType, ip string, proxied bool) error {
	parts := strings.Split(domain, ".")
	subdomain := "@"
	rootdomain := domain
	if len(parts) >= 2 {
		subdomain = strings.Join(parts[:len(parts)-2], ".")
		if subdomain == "" {
			subdomain = "@"
		}
		rootdomain = strings.Join(parts[len(parts)-2:], ".")
	}

	replacer := strings.NewReplacer(
		"#{subdomain}", subdomain,
		"#{rootdomain}", rootdomain,
		"#{domain}", domain,
		"#{ip}", ip,
		"#{type}", string(recordType),
	)

	reqURL := replacer.Replace(p.Config.URL)
	reqBodyStr := replacer.Replace(p.Config.Body)

	method := strings.ToUpper(p.Config.Method)
	if method == "" {
		method = "GET"
	}

	var bodyReader io.Reader
	if reqBodyStr != "" {
		bodyReader = strings.NewReader(reqBodyStr)
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return fmt.Errorf("建立 Custom HTTP 請求失敗: %w", err)
	}

	for k, v := range p.Config.Headers {
		req.Header.Set(k, replacer.Replace(v))
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Custom HTTP 請求發送失敗: %w", err)
	}
	defer resp.Body.Close()

	respBodyBytes, _ := io.ReadAll(resp.Body)
	respBody := string(respBodyBytes)

	expectedStatus := p.Config.ExpectedStatus
	if expectedStatus == 0 {
		expectedStatus = 200
	}

	if resp.StatusCode != expectedStatus {
		return fmt.Errorf("Custom HTTP 回應 Status Code 為 %d，期望為 %d", resp.StatusCode, expectedStatus)
	}

	if p.Config.ExpectedBodyRegex != "" {
		re, err := regexp.Compile(p.Config.ExpectedBodyRegex)
		if err != nil {
			return fmt.Errorf("無效的正則驗證表達式: %w", err)
		}
		if !re.MatchString(respBody) {
			return fmt.Errorf("Custom HTTP 回應內容 %q 不匹配正則條件 %q", respBody, p.Config.ExpectedBodyRegex)
		}
	}

	logger.Log(logger.SUCCESS, "", "Custom HTTP 成功更新 %s (%s) -> %s", domain, recordType, ip)
	return nil
}
