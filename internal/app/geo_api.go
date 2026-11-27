package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type geoInfo struct {
	IP          string `json:"ip"`
	Country     string `json:"country"`
	CountryCode string `json:"country_code"`
	Region      string `json:"region"`
	City        string `json:"city"`
	ISP         string `json:"isp"`
	Org         string `json:"org"`
	ASN         string `json:"asn"`
}

type geoCacheEntry struct {
	info  geoInfo
	until time.Time
}

var (
	geoMu    sync.Mutex
	geoCache = map[string]geoCacheEntry{}
)

func (a *App) handleGeoLookup(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimSpace(r.URL.Query().Get("ip"))
	ip := net.ParseIP(strings.Trim(raw, "[]"))
	if ip == nil {
		writeErr(w, 400, "آدرس IP معتبر نیست")
		return
	}
	ipText := ip.String()
	now := time.Now()
	geoMu.Lock()
	if entry, ok := geoCache[ipText]; ok && now.Before(entry.until) {
		geoMu.Unlock()
		writeData(w, entry.info)
		return
	}
	geoMu.Unlock()

	info, err := lookupGeoIP(r.Context(), ipText)
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	geoMu.Lock()
	geoCache[ipText] = geoCacheEntry{info: info, until: now.Add(6 * time.Hour)}
	geoMu.Unlock()
	writeData(w, info)
}

func lookupGeoIP(ctx context.Context, ip string) (geoInfo, error) {
	client := &http.Client{Timeout: 8 * time.Second}

	// ipwho.is is HTTPS and does not require an API key.
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://ipwho.is/"+ip, nil)
	if resp, err := client.Do(req); err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			var body struct {
				Success     bool   `json:"success"`
				Country     string `json:"country"`
				CountryCode string `json:"country_code"`
				Region      string `json:"region"`
				City        string `json:"city"`
				ISP         string `json:"isp"`
				Org         string `json:"org"`
				Connection  struct {
					ISP string `json:"isp"`
					Org string `json:"org"`
					ASN int    `json:"asn"`
				} `json:"connection"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&body); err == nil && body.Success {
				isp := firstNonEmptyStr(body.ISP, body.Connection.ISP)
				org := firstNonEmptyStr(body.Org, body.Connection.Org)
				asn := ""
				if body.Connection.ASN > 0 {
					asn = fmt.Sprintf("AS%d", body.Connection.ASN)
				}
				return geoInfo{
					IP: ip, Country: body.Country, CountryCode: body.CountryCode,
					Region: body.Region, City: body.City, ISP: isp, Org: org, ASN: asn,
				}, nil
			}
		}
	}

	// Fallback for networks where ipwho.is is blocked. ip-api.com allows
	// unauthenticated lookups over HTTP.
	req, _ = http.NewRequestWithContext(ctx, http.MethodGet,
		"http://ip-api.com/json/"+ip+"?fields=status,country,countryCode,regionName,city,isp,org,as,query", nil)
	resp, err := client.Do(req)
	if err != nil {
		return geoInfo{}, fmt.Errorf("IP owner lookup failed: %v", err)
	}
	defer resp.Body.Close()
	var body struct {
		Status      string `json:"status"`
		Message     string `json:"message"`
		Country     string `json:"country"`
		CountryCode string `json:"countryCode"`
		Region      string `json:"regionName"`
		City        string `json:"city"`
		ISP         string `json:"isp"`
		Org         string `json:"org"`
		AS          string `json:"as"`
		Query       string `json:"query"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return geoInfo{}, fmt.Errorf("IP owner lookup failed: %v", err)
	}
	if body.Status != "success" {
		msg := strings.TrimSpace(body.Message)
		if msg == "" {
			msg = "lookup service unavailable"
		}
		return geoInfo{}, fmt.Errorf("IP owner lookup failed: %s", msg)
	}
	return geoInfo{
		IP: firstNonEmptyStr(body.Query, ip), Country: body.Country, CountryCode: body.CountryCode,
		Region: body.Region, City: body.City, ISP: body.ISP, Org: body.Org, ASN: body.AS,
	}, nil
}

func firstNonEmptyStr(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
