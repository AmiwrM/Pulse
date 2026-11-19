package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"

	"pulse/internal/model"
)

var ErrGone = errors.New("push subscription expired")

func httpClient() *http.Client {
	return &http.Client{Timeout: 20 * time.Second}
}

// SendTelegram delivers an HTML formatted message.
func SendTelegram(ctx context.Context, token, chatID, text string) error {
	if strings.TrimSpace(token) == "" || strings.TrimSpace(chatID) == "" {
		return fmt.Errorf("توکن ربات یا Chat ID تنظیم نشده است")
	}
	if len([]rune(text)) > 4000 {
		text = string([]rune(text)[:4000]) + "…"
	}
	body, _ := json.Marshal(map[string]any{
		"chat_id":                  chatID,
		"text":                     text,
		"parse_mode":               "HTML",
		"disable_web_page_preview": true,
	})
	url := "https://api.telegram.org/bot" + token + "/sendMessage"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 32*1024))
	var tr struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	_ = json.Unmarshal(data, &tr)
	if resp.StatusCode != http.StatusOK || !tr.OK {
		desc := tr.Description
		if desc == "" {
			desc = strings.TrimSpace(string(data))
		}
		return fmt.Errorf("تلگرام: %s", desc)
	}
	return nil
}

// SendWebhook posts a signed JSON event.
func SendWebhook(ctx context.Context, rawURL, secret string, payload map[string]any) error {
	if strings.TrimSpace(rawURL) == "" {
		return fmt.Errorf("آدرس وبهوک تنظیم نشده است")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Pulse-Monitor/1.0")
	if secret != "" {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		req.Header.Set("X-Pulse-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	resp, err := httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 8*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("وبهوک کد %d برگرداند", resp.StatusCode)
	}
	return nil
}

type PushSender struct {
	PublicKey  string
	PrivateKey string
	Subject    string
}

type pushPayload struct {
	Title     string `json:"title"`
	Body      string `json:"body"`
	Tag       string `json:"tag"`
	URL       string `json:"url"`
	Timestamp int64  `json:"timestamp"`
	Icon      string `json:"icon"`
	Badge     string `json:"badge"`
}

func (p *PushSender) Configured() bool {
	return p.PublicKey != "" && p.PrivateKey != ""
}

// SendPush delivers a web push message. ErrGone means the subscription must be removed.
func (p *PushSender) SendPush(ctx context.Context, sub model.PushSub, title, body, tag, targetURL string) error {
	if !p.Configured() {
		return fmt.Errorf("کلیدهای Web Push هنوز ساخته نشده‌اند")
	}
	payload, _ := json.Marshal(pushPayload{
		Title:     title,
		Body:      body,
		Tag:       tag,
		URL:       targetURL,
		Timestamp: time.Now().Unix(),
		Icon:      "/icons/icon-192.png",
		Badge:     "/icons/badge-72.png",
	})
	subject := p.Subject
	if subject == "" {
		subject = "mailto:admin@example.com"
	}
	resp, err := webpush.SendNotificationWithContext(ctx, payload, &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys: webpush.Keys{
			Auth:   sub.Auth,
			P256dh: sub.P256DH,
		},
	}, &webpush.Options{
		Subscriber:      subject,
		VAPIDPublicKey:  p.PublicKey,
		VAPIDPrivateKey: p.PrivateKey,
		TTL:             3600,
		Urgency:         webpush.UrgencyHigh,
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 8*1024))
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		return ErrGone
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("سرویس پوش کد %d برگرداند", resp.StatusCode)
	}
	return nil
}
