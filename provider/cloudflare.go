package provider

import (
	"context"
	"fmt"
	"strings"

	"muddns/logger"

	"github.com/cloudflare/cloudflare-go"
)

type CloudflareProvider struct {
	name   string
	api    *cloudflare.API
	zoneID string
}

func NewCloudflareProvider(name string, apiToken string, zoneID string) (*CloudflareProvider, error) {
	api, err := cloudflare.NewWithAPIToken(apiToken)
	if err != nil {
		return nil, fmt.Errorf("cloudflare client init failed: %w", err)
	}
	return &CloudflareProvider{
		name:   name,
		api:    api,
		zoneID: zoneID,
	}, nil
}

func (p *CloudflareProvider) Name() string {
	return p.name
}

func (p *CloudflareProvider) UpdateRecord(ctx context.Context, record TargetRecord) error {
	zoneID := p.zoneID
	if zoneID == "" {
		// Auto-fetch Zone ID if not provided
		zid, err := p.api.ZoneIDByName(record.RootDomain)
		if err != nil {
			return fmt.Errorf("failed to get zone id for %s: %w", record.RootDomain, err)
		}
		zoneID = zid
	}

	rc := cloudflare.ZoneIdentifier(zoneID)

	// List existing DNS records matching type and domain
	records, _, err := p.api.ListDNSRecords(ctx, rc, cloudflare.ListDNSRecordsParams{
		Type: string(record.Type),
		Name: record.Domain,
	})
	if err != nil {
		return fmt.Errorf("cloudflare list dns records failed: %w", err)
	}

	proxied := record.Proxied

	if len(records) == 0 {
		// Create new record
		logger.Log(logger.INFO, record.HostID, "Cloudflare creating new %s record %s -> %s", record.Type, record.Domain, record.IP)
		_, err := p.api.CreateDNSRecord(ctx, rc, cloudflare.CreateDNSRecordParams{
			Type:    string(record.Type),
			Name:    record.Domain,
			Content: record.IP,
			TTL:     1, // Automatic
			Proxied: &proxied,
		})
		if err != nil {
			return fmt.Errorf("cloudflare create record failed: %w", err)
		}
		logger.Log(logger.SUCCESS, record.HostID, "Cloudflare created %s record %s -> %s", record.Type, record.Domain, record.IP)
		return nil
	}

	// Update existing record if content or proxied changed
	existing := records[0]
	if existing.Content == record.IP && existing.Proxied != nil && *existing.Proxied == proxied {
		logger.Log(logger.INFO, record.HostID, "Cloudflare record %s (%s) is already up to date: %s", record.Domain, record.Type, record.IP)
		return nil
	}

	logger.Log(logger.INFO, record.HostID, "Cloudflare updating %s record %s -> %s", record.Type, record.Domain, record.IP)
	_, err = p.api.UpdateDNSRecord(ctx, rc, cloudflare.UpdateDNSRecordParams{
		ID:      existing.ID,
		Type:    string(record.Type),
		Name:    record.Domain,
		Content: record.IP,
		TTL:     1,
		Proxied: &proxied,
	})
	if err != nil {
		return fmt.Errorf("cloudflare update record failed: %w", err)
	}

	logger.Log(logger.SUCCESS, record.HostID, "Cloudflare updated %s record %s -> %s", record.Type, record.Domain, record.IP)
	return nil
}

func ExtractRootDomain(domain string) (subdomain string, rootDomain string) {
	parts := strings.Split(domain, ".")
	if len(parts) <= 2 {
		return "@", domain
	}
	subdomain = strings.Join(parts[:len(parts)-2], ".")
	rootDomain = strings.Join(parts[len(parts)-2:], ".")
	return subdomain, rootDomain
}
