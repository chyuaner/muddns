package ipfetcher

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"muddns/lib/logger"
)

// FetchExternalIP 透過外網 HTTP Echo API 查詢公網 IP
// 支援指定單一 URL 或嘗試預設備援 API 列表，若遇到失敗或非 IP 內容會自動重試備援 API
func FetchExternalIP(customURL string, isIPv6 bool, defaults []string, hostID string) (string, error) {
	var targetURLs []string

	// 如果使用者指定了自訂 URL 放在首位，備援 API 放在後續
	if customURL != "" {
		targetURLs = append(targetURLs, customURL)
	}
	targetURLs = append(targetURLs, defaults...)

	if len(targetURLs) == 0 {
		return "", fmt.Errorf("沒有可用的外部 API URL")
	}

	client := &http.Client{
		Timeout: 5 * time.Second, // 避免請求卡住，設定 5 秒超時
	}

	var lastErr error
	for _, apiURL := range targetURLs {
		req, err := http.NewRequest("GET", apiURL, nil)
		if err != nil {
			lastErr = err
			continue
		}
		// 模擬標準 curl User-Agent 以利 Echo API 傳回純文字 IP
		req.Header.Set("User-Agent", "curl/8.0.0")
		req.Header.Set("Accept", "text/plain")

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			logger.Log(logger.WARN, hostID, "Echo API %s 存取失敗: %v，嘗試下一個備援...", apiURL, err)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()

		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("HTTP Status %d", resp.StatusCode)
			logger.Log(logger.WARN, hostID, "Echo API %s 回應狀態非 200 (%d)，嘗試下一個...", apiURL, resp.StatusCode)
			continue
		}

		rawOutput := string(body)
		cleanIP := parseAndExtractIP(rawOutput, isIPv6)
		if cleanIP != "" {
			return cleanIP, nil
		}

		lastErr = fmt.Errorf("API 回應內容無法解析出合規的 %s 地址", map[bool]string{false: "IPv4", true: "IPv6"}[isIPv6])
		logger.Log(logger.WARN, hostID, "Echo API %s 回應內容非有效 IP，嘗試下一個備援...", apiURL)
	}

	return "", fmt.Errorf("所有外部 Echo API 查詢皆失敗: %v", lastErr)
}

// parseAndExtractIP 解析並驗證字串中的 IP 地址（若回傳包含 HTML/JSON 也能精準提取出 IP）
func parseAndExtractIP(input string, isIPv6 bool) string {
	trimmed := strings.TrimSpace(input)

	// 1. 優先嘗試純 IP 解析
	parsed := net.ParseIP(trimmed)
	if parsed != nil {
		if isIPv6 && parsed.To4() == nil {
			return parsed.String()
		}
		if !isIPv6 && parsed.To4() != nil {
			return parsed.String()
		}
	}

	// 2. 若包含 HTML/JSON 雜訊，使用正則提取正確的 IP 地址
	if isIPv6 {
		reIPv6 := regexp.MustCompile(`(?:[0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}|(?:[0-9a-fA-F]{1,4}:){1,7}:|(?:[0-9a-fA-F]{1,4}:){1,6}:[0-9a-fA-F]{1,4}|(?:[0-9a-fA-F]{1,4}:){1,5}(?::[0-9a-fA-F]{1,4}){1,2}|(?:[0-9a-fA-F]{1,4}:){1,4}(?::[0-9a-fA-F]{1,4}){1,3}|(?:[0-9a-fA-F]{1,4}:){1,3}(?::[0-9a-fA-F]{1,4}){1,4}|(?:[0-9a-fA-F]{1,4}:){1,2}(?::[0-9a-fA-F]{1,4}){1,5}|[0-9a-fA-F]{1,4}:(?:(?::[0-9a-fA-F]{1,4}){1,6})|:(?:(?::[0-9a-fA-F]{1,4}){1,7}|:)|::(?:ffff(?::0{1,4}){0,1}:){0,1}(?:(?:25[0-5]|(?:2[0-4]|1{0,1}[0-9]){0,1}[0-9])\.){3}(?:25[0-5]|(?:2[0-4]|1{0,1}[0-9]){0,1}[0-9])`)
		match := reIPv6.FindString(trimmed)
		if match != "" {
			if ip := net.ParseIP(match); ip != nil && ip.To4() == nil {
				return ip.String()
			}
		}
	} else {
		reIPv4 := regexp.MustCompile(`\b(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\b`)
		match := reIPv4.FindString(trimmed)
		if match != "" {
			if ip := net.ParseIP(match); ip != nil && ip.To4() != nil {
				return ip.String()
			}
		}
	}

	return ""
}
