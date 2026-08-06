package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudflare/cloudflare-go"
	"muddns/lib/logger"
)

// CloudflareProvider 實作 Cloudflare DNS API 更新
type CloudflareProvider struct {
	APIToken string
	ZoneID   string
}

func NewCloudflareProvider(apiToken string, zoneID string) *CloudflareProvider {
	return &CloudflareProvider{
		APIToken: apiToken,
		ZoneID:   zoneID,
	}
}

// UpdateRecord 呼叫 Cloudflare API 更新指定域名的 A / AAAA 紀錄
func (p *CloudflareProvider) UpdateRecord(ctx context.Context, domain string, recordType RecordType, ip string, proxied bool) error {
	api, err := cloudflare.NewWithAPIToken(p.APIToken)
	if err != nil {
		return fmt.Errorf("Cloudflare 初始化失敗: %w", err)
	}

	zoneID := p.ZoneID
	// 若未手動提供 ZoneID，則透過域名自動向 Cloudflare 查詢 ZoneID
	if zoneID == "" {
		parts := strings.Split(domain, ".")
		if len(parts) >= 2 {
			rootDomain := parts[len(parts)-2] + "." + parts[len(parts)-1]
			zID, err := api.ZoneIDByName(rootDomain)
			if err != nil {
				return fmt.Errorf("由域名 %s 自動查詢 Zone ID 失敗: %w", rootDomain, err)
			}
			zoneID = zID
		} else {
			return fmt.Errorf("無法自域名 %s 解析出根域名", domain)
		}
	}

	rc := cloudflare.ZoneIdentifier(zoneID)

	// 搜尋是否已有存在的 DNS 紀錄
	records, _, err := api.ListDNSRecords(ctx, rc, cloudflare.ListDNSRecordsParams{
		Name: domain,
		Type: string(recordType),
	})
	if err != nil {
		return fmt.Errorf("Cloudflare 查詢 DNS 紀錄失敗: %w", err)
	}

	// 若已有紀錄，檢查是否需要更新
	if len(records) > 0 {
		rec := records[0]
		if rec.Content == ip && rec.Proxied != nil && *rec.Proxied == proxied {
			logger.Log(logger.INFO, "", "Cloudflare DNS 紀錄 %s (%s) 已是最新值 (%s)，跳過更新", domain, recordType, ip)
			return nil
		}

		// 執行更新
		_, err := api.UpdateDNSRecord(ctx, rc, cloudflare.UpdateDNSRecordParams{
			ID:      rec.ID,
			Type:    string(recordType),
			Name:    domain,
			Content: ip,
			Proxied: &proxied,
			TTL:     1, // 1 代表 Auto TTL
		})
		if err != nil {
			return fmt.Errorf("Cloudflare 更新 DNS 紀錄失敗: %w", err)
		}
		logger.Log(logger.SUCCESS, "", "Cloudflare 成功更新 %s (%s) -> %s (Proxied: %t)", domain, recordType, ip, proxied)
		return nil
	}

	// 若不存在紀錄，則新建筆紀錄
	_, err = api.CreateDNSRecord(ctx, rc, cloudflare.CreateDNSRecordParams{
		Type:    string(recordType),
		Name:    domain,
		Content: ip,
		Proxied: &proxied,
		TTL:     1,
	})
	if err != nil {
		return fmt.Errorf("Cloudflare 建立 DNS 紀錄失敗: %w", err)
	}

	logger.Log(logger.SUCCESS, "", "Cloudflare 成功建立 %s (%s) -> %s (Proxied: %t)", domain, recordType, ip, proxied)
	return nil
}

func formatResourceType(t cloudflare.ResourceType) cloudflare.ResourceType {
	return t
}
