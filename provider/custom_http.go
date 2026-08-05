package provider

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"muddns/logger"
)

type CustomHTTPConfig struct {
	Method            string            `yaml:"method"`
	URL               string            `yaml:"url"`
	Headers           map[string]string `yaml:"headers"`
	Body              string            `yaml:"body"`
	ExpectedStatus    int               `yaml:"expected_status"`
	ExpectedBodyRegex string            `yaml:"expected_body_regex"`
}

type CustomHTTPProvider struct {
	name   string
	cfg    CustomHTTPConfig
	client *http.Client
}

func NewCustomHTTPProvider(name string, cfg CustomHTTPConfig) *CustomHTTPProvider {
	if cfg.ExpectedStatus == 0 {
		cfg.ExpectedStatus = 200
	}
	if cfg.Method == "" {
		cfg.Method = "GET"
	}
	return &CustomHTTPProvider{
		name: name,
		cfg:  cfg,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (p *CustomHTTPProvider) Name() string {
	return p.name
}

func (p *CustomHTTPProvider) replaceVariables(tmpl string, record TargetRecord) string {
	sub, root := ExtractRootDomain(record.Domain)
	if record.Subdomain != "" {
		sub = record.Subdomain
	}
	if record.RootDomain != "" {
		root = record.RootDomain
	}

	replacer := strings.NewReplacer(
		"#{ip}", record.IP,
		"#{domain}", record.Domain,
		"#{subdomain}", sub,
		"#{rootdomain}", root,
		"#{type}", string(record.Type),
		"#{timestamp}", fmt.Sprintf("%d", time.Now().Unix()),
	)
	return replacer.Replace(tmpl)
}

func (p *CustomHTTPProvider) UpdateRecord(ctx context.Context, record TargetRecord) error {
	finalURL := p.replaceVariables(p.cfg.URL, record)
	finalBody := p.replaceVariables(p.cfg.Body, record)

	var bodyReader io.Reader
	if finalBody != "" {
		bodyReader = bytes.NewBufferString(finalBody)
	}

	req, err := http.NewRequestWithContext(ctx, strings.ToUpper(p.cfg.Method), finalURL, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	for k, v := range p.cfg.Headers {
		req.Header.Set(k, p.replaceVariables(v, record))
	}

	logger.Log(logger.INFO, record.HostID, "Custom HTTP [%s] sending request to %s", p.name, finalURL)
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("custom http request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	respStr := string(respBytes)

	if resp.StatusCode != p.cfg.ExpectedStatus {
		return fmt.Errorf("unexpected status code %d (expected %d), body: %s", resp.StatusCode, p.cfg.ExpectedStatus, respStr)
	}

	if p.cfg.ExpectedBodyRegex != "" {
		matched, err := regexp.MatchString(p.cfg.ExpectedBodyRegex, respStr)
		if err != nil {
			return fmt.Errorf("regex match error: %w", err)
		}
		if !matched {
			return fmt.Errorf("response body does not match regex '%s': %s", p.cfg.ExpectedBodyRegex, respStr)
		}
	}

	logger.Log(logger.SUCCESS, record.HostID, "Custom HTTP [%s] updated %s record %s -> %s", p.name, record.Type, record.Domain, record.IP)
	return nil
}
