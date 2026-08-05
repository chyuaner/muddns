package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"muddns/logger"
)

type NamecheapProvider struct {
	name     string
	password string
	client   *http.Client
}

func NewNamecheapProvider(name string, password string) *NamecheapProvider {
	return &NamecheapProvider{
		name:     name,
		password: password,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (p *NamecheapProvider) Name() string {
	return p.name
}

func (p *NamecheapProvider) UpdateRecord(ctx context.Context, record TargetRecord) error {
	sub, root := ExtractRootDomain(record.Domain)
	if record.Subdomain != "" {
		sub = record.Subdomain
	}
	if record.RootDomain != "" {
		root = record.RootDomain
	}

	endpoint := fmt.Sprintf(
		"https://dynamicdns.park-your-domain.com/update?host=%s&domain=%s&password=%s&ip=%s",
		url.QueryEscape(sub),
		url.QueryEscape(root),
		url.QueryEscape(p.password),
		url.QueryEscape(record.IP),
	)

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("namecheap http request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyStr := string(bodyBytes)

	if strings.Contains(bodyStr, "<ErrCount>0</ErrCount>") {
		logger.Log(logger.SUCCESS, record.HostID, "Namecheap updated %s record %s -> %s", record.Type, record.Domain, record.IP)
		return nil
	}

	return fmt.Errorf("namecheap update failed, response: %s", bodyStr)
}
