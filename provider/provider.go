package provider

import (
	"context"
)

type RecordType string

const (
	RecordTypeA    RecordType = "A"
	RecordTypeAAAA RecordType = "AAAA"
)

type TargetRecord struct {
	HostID     string
	Domain     string     // e.g. nas.example.com
	Subdomain  string     // e.g. nas
	RootDomain string     // e.g. example.com
	IP         string     // Calculated IPv4 or IPv6
	Type       RecordType // A or AAAA
	Proxied    bool       // Cloudflare proxy toggle
}

type Provider interface {
	Name() string
	UpdateRecord(ctx context.Context, record TargetRecord) error
}
