package acme

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"librevita.org/pkg/errors"
)

// CloudflareProvider implements DNSProvider using Cloudflare v4 REST API.
type CloudflareProvider struct {
	apiToken string
	zoneID   string
	baseURL  string
	client   *http.Client
}

// NewCloudflareProvider creates a new Cloudflare DNS provider.
func NewCloudflareProvider(apiToken, zoneID string) *CloudflareProvider {
	return &CloudflareProvider{
		apiToken: apiToken,
		zoneID:   zoneID,
		baseURL:  "https://api.cloudflare.com/client/v4",
		client:   &http.Client{Timeout: 15 * time.Second},
	}
}

type cfResponse[T any] struct {
	Success  bool     `json:"success"`
	Errors   []cfErr  `json:"errors"`
	Messages []string `json:"messages"`
	Result   T        `json:"result"`
}

type cfErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type cfZone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type cfRecord struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
}

func (p *CloudflareProvider) getZoneID(ctx context.Context, domain string) (string, error) {
	if p.zoneID != "" {
		return p.zoneID, nil
	}

	cleanDomain := strings.TrimPrefix(domain, "*.")
	cleanDomain = strings.TrimPrefix(cleanDomain, ".")
	parts := strings.Split(cleanDomain, ".")

	for i := 0; i < len(parts)-1; i++ {
		zoneCandidate := strings.Join(parts[i:], ".")
		url := fmt.Sprintf("%s/zones?name=%s&status=active", p.baseURL, zoneCandidate)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return "", errors.Wrap(err, "acme cloudflare: new request")
		}
		req.Header.Set("Authorization", "Bearer "+p.apiToken)
		req.Header.Set("Content-Type", "application/json")

		resp, err := p.client.Do(req)
		if err != nil {
			return "", errors.Wrap(err, "acme cloudflare: list zones")
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", errors.Wrap(err, "acme cloudflare: read body")
		}

		var parsed cfResponse[[]cfZone]
		if err := json.Unmarshal(body, &parsed); err == nil && parsed.Success && len(parsed.Result) > 0 {
			return parsed.Result[0].ID, nil
		}
	}

	return "", errors.Newf("acme cloudflare: could not find active zone for domain %q", domain)
}

func (p *CloudflareProvider) Present(ctx context.Context, domain, keyAuthRecord string) error {
	zoneID, err := p.getZoneID(ctx, domain)
	if err != nil {
		return err
	}

	recordName := ChallengeRecordName(domain)
	payload := map[string]any{
		"type":    "TXT",
		"name":    recordName,
		"content": keyAuthRecord,
		"ttl":     120,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return errors.Wrap(err, "acme cloudflare: marshal record")
	}

	url := fmt.Sprintf("%s/zones/%s/dns_records", p.baseURL, zoneID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return errors.Wrap(err, "acme cloudflare: new record request")
	}
	req.Header.Set("Authorization", "Bearer "+p.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return errors.Wrap(err, "acme cloudflare: create record")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return errors.Wrap(err, "acme cloudflare: read response")
	}

	var parsed cfResponse[cfRecord]
	if err := json.Unmarshal(body, &parsed); err != nil {
		return errors.Wrap(err, "acme cloudflare: unmarshal response")
	}
	if !parsed.Success {
		var errMsg string
		if len(parsed.Errors) > 0 {
			errMsg = parsed.Errors[0].Message
		}
		return errors.Newf("acme cloudflare: failed to create txt record: %s", errMsg)
	}
	return nil
}

func (p *CloudflareProvider) CleanUp(ctx context.Context, domain, keyAuthRecord string) error {
	zoneID, err := p.getZoneID(ctx, domain)
	if err != nil {
		return err
	}

	recordName := ChallengeRecordName(domain)
	url := fmt.Sprintf("%s/zones/%s/dns_records?type=TXT&name=%s", p.baseURL, zoneID, recordName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return errors.Wrap(err, "acme cloudflare: cleanup get request")
	}
	req.Header.Set("Authorization", "Bearer "+p.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return errors.Wrap(err, "acme cloudflare: cleanup list records")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return errors.Wrap(err, "acme cloudflare: cleanup read response")
	}

	var parsed cfResponse[[]cfRecord]
	if err := json.Unmarshal(body, &parsed); err != nil {
		return errors.Wrap(err, "acme cloudflare: cleanup parse response")
	}

	for _, rec := range parsed.Result {
		if rec.Content == keyAuthRecord {
			delURL := fmt.Sprintf("%s/zones/%s/dns_records/%s", p.baseURL, zoneID, rec.ID)
			delReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, delURL, nil)
			if err != nil {
				continue
			}
			delReq.Header.Set("Authorization", "Bearer "+p.apiToken)
			delReq.Header.Set("Content-Type", "application/json")
			delResp, err := p.client.Do(delReq)
			if err == nil {
				_ = delResp.Body.Close()
			}
		}
	}
	return nil
}
