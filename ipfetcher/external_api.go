package ipfetcher

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"muddns/logger"
)

var (
	IPv4Client = &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				var dialer net.Dialer
				return dialer.DialContext(ctx, "tcp4", addr)
			},
		},
		Timeout: 5 * time.Second,
	}

	IPv6Client = &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				var dialer net.Dialer
				return dialer.DialContext(ctx, "tcp6", addr)
			},
		},
		Timeout: 5 * time.Second,
	}
)

func FetchExternalIP(customURL string, isIPv6 bool, defaults []string, hostID string) (string, error) {
	client := IPv4Client
	if isIPv6 {
		client = IPv6Client
	}

	// 1. Try custom URL if provided
	if customURL != "" {
		ip, err := queryAPI(client, customURL)
		if err == nil && isValidIP(ip, isIPv6) {
			logger.Log(logger.DEBUG, hostID, "Fetched IP %s from custom URL: %s", ip, customURL)
			return ip, nil
		}
		logger.Log(logger.WARN, hostID, "Custom IP API %s failed: %v, falling back to defaults", customURL, err)
	}

	// 2. Try default failover list
	for _, apiURL := range defaults {
		ip, err := queryAPI(client, apiURL)
		if err == nil && isValidIP(ip, isIPv6) {
			logger.Log(logger.DEBUG, hostID, "Fetched IP %s from default API: %s", ip, apiURL)
			return ip, nil
		}
		logger.Log(logger.DEBUG, hostID, "Default IP API %s failed: %v", apiURL, err)
	}

	return "", fmt.Errorf("all external IP APIs failed for IPv6=%v", isIPv6)
}

func queryAPI(client *http.Client, apiURL string) (string, error) {
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "muddns/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("http status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	ipStr := strings.TrimSpace(string(body))
	return ipStr, nil
}

func isValidIP(ipStr string, isIPv6 bool) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	if isIPv6 {
		return ip.To4() == nil
	}
	return ip.To4() != nil
}
