// Package provider 定義 DNS 服務商介面與實作，包含 Cloudflare, Namecheap 以及 Custom HTTP 通用 Webhook 支援。
package provider

import (
	"context"
	"fmt"

	"muddns/config"
)

// RecordType 定義 DNS 紀錄類型 (A 代表 IPv4, AAAA 代表 IPv6)
type RecordType string

const (
	RecordA    RecordType = "A"
	RecordAAAA RecordType = "AAAA"
)

// DNSProvider 定義各個 DNS 服務商必須實現的統一介面 (Interface)
type DNSProvider interface {
	UpdateRecord(ctx context.Context, domain string, recordType RecordType, ip string, proxied bool) error
}

// NewProvider 根據設定檔中的 Provider 資訊工廠建立對應的 DNSProvider 實作
func NewProvider(p config.Provider) (DNSProvider, error) {
	switch p.Provider {
	case "cloudflare":
		return NewCloudflareProvider(p.APIToken, p.ZoneID), nil
	case "namecheap":
		return NewNamecheapProvider(p.Password), nil
	case "custom_http":
		return NewCustomHTTPProvider(p), nil
	default:
		return nil, fmt.Errorf("不支援的 DNS Provider 類型: %s", p.Provider)
	}
}
