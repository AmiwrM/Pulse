package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"pulse/internal/model"
)

const defaultBase = "https://api.cloudflare.com/client/v4"

type Client struct {
	Token   string
	BaseURL string
	HTTP    *http.Client
}

func New(token string) *Client {
	return &Client{Token: token, BaseURL: defaultBase, HTTP: &http.Client{Timeout: 25 * time.Second}}
}

func (c *Client) Configured() bool { return strings.TrimSpace(c.Token) != "" }

type apiResponse struct {
	Success bool `json:"success"`
	Errors  []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
	Result     json.RawMessage `json:"result"`
	ResultInfo struct {
		Page       int `json:"page"`
		TotalPages int `json:"total_pages"`
	} `json:"result_info"`
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) (json.RawMessage, error) {
	if !c.Configured() {
		return nil, fmt.Errorf("توکن Cloudflare تنظیم نشده است")
	}
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(b)
	}
	base := c.BaseURL
	if base == "" {
		base = defaultBase
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(base, "/")+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 25 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	var ar apiResponse
	if err := json.Unmarshal(data, &ar); err != nil {
		return nil, fmt.Errorf("پاسخ نامعتبر از Cloudflare (کد %d)", resp.StatusCode)
	}
	if !ar.Success {
		msgs := make([]string, 0, len(ar.Errors))
		for _, e := range ar.Errors {
			msgs = append(msgs, fmt.Sprintf("%s (%d)", e.Message, e.Code))
		}
		if len(msgs) == 0 {
			return nil, fmt.Errorf("Cloudflare خطا برگرداند (کد %d)", resp.StatusCode)
		}
		return nil, errors.New(strings.Join(msgs, "، "))
	}
	if out != nil && len(ar.Result) > 0 {
		if err := json.Unmarshal(ar.Result, out); err != nil {
			return nil, err
		}
	}
	return ar.Result, nil
}

type VerifyResult struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

func (c *Client) Verify(ctx context.Context) (*VerifyResult, error) {
	var out VerifyResult
	if _, err := c.do(ctx, http.MethodGet, "/user/tokens/verify", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) ListZones(ctx context.Context) ([]model.Zone, error) {
	out := []model.Zone{}
	page := 1
	for {
		var zones []struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Status string `json:"status"`
		}
		_, err := c.do(ctx, http.MethodGet, fmt.Sprintf("/zones?per_page=50&page=%d&order=name&direction=asc", page), nil, &zones)
		if err != nil {
			return nil, err
		}
		for _, z := range zones {
			out = append(out, model.Zone{ID: z.ID, Name: z.Name, Status: z.Status})
		}
		if len(zones) < 50 || page >= 10 {
			break
		}
		page++
	}
	return out, nil
}

type rawRecord struct {
	ID      string `json:"id"`
	ZoneID  string `json:"zone_id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	Proxied bool   `json:"proxied"`
}

func (c *Client) ListRecords(ctx context.Context, zoneID, recordType, search string) ([]model.DNSRecord, error) {
	if zoneID == "" {
		return nil, fmt.Errorf("شناسه Zone مشخص نیست")
	}
	out := []model.DNSRecord{}
	page := 1
	search = strings.ToLower(strings.TrimSpace(search))
	for {
		q := url.Values{}
		q.Set("per_page", "100")
		q.Set("page", fmt.Sprint(page))
		if recordType != "" {
			q.Set("type", recordType)
		}
		var records []rawRecord
		_, err := c.do(ctx, http.MethodGet, "/zones/"+url.PathEscape(zoneID)+"/dns_records?"+q.Encode(), nil, &records)
		if err != nil {
			return nil, err
		}
		for _, r := range records {
			if search != "" && !strings.Contains(strings.ToLower(r.Name), search) && !strings.Contains(r.Content, search) {
				continue
			}
			out = append(out, model.DNSRecord{
				ID: r.ID, ZoneID: r.ZoneID, Type: r.Type, Name: r.Name, Content: r.Content, TTL: r.TTL, Proxied: r.Proxied,
			})
		}
		if len(records) < 100 || page >= 5 {
			break
		}
		page++
	}
	return out, nil
}

func (c *Client) GetRecord(ctx context.Context, zoneID, recordID string) (*model.DNSRecord, error) {
	var r rawRecord
	_, err := c.do(ctx, http.MethodGet, "/zones/"+url.PathEscape(zoneID)+"/dns_records/"+url.PathEscape(recordID), nil, &r)
	if err != nil {
		return nil, err
	}
	return &model.DNSRecord{ID: r.ID, ZoneID: r.ZoneID, Type: r.Type, Name: r.Name, Content: r.Content, TTL: r.TTL, Proxied: r.Proxied}, nil
}

// UpdateRecordContent changes only the content of a DNS record (safe PATCH).
func (c *Client) UpdateRecordContent(ctx context.Context, zoneID, recordID, content string) (*model.DNSRecord, error) {
	var r rawRecord
	_, err := c.do(ctx, http.MethodPatch, "/zones/"+url.PathEscape(zoneID)+"/dns_records/"+url.PathEscape(recordID),
		map[string]string{"content": content}, &r)
	if err != nil {
		return nil, err
	}
	return &model.DNSRecord{ID: r.ID, ZoneID: r.ZoneID, Type: r.Type, Name: r.Name, Content: r.Content, TTL: r.TTL, Proxied: r.Proxied}, nil
}
