package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"pulse/internal/cloudflare"
	"pulse/internal/config"
	"pulse/internal/model"
)

var errCFAccountMissing = errors.New("حساب Cloudflare تنظیم نشده است؛ ابتدا یک حساب اضافه کنید")

var validDNSRecordTypes = map[string]bool{
	"A": true, "AAAA": true, "CNAME": true, "TXT": true, "MX": true, "NS": true, "SRV": true, "CAA": true,
}

// cfClient resolves an account by id, or picks the first enabled account when
// id is zero. Tokens are read from the cf_accounts table, never from the
// browser.
func (a *App) cfClient(accountID int64) (*cloudflare.Client, model.CFAccount, error) {
	var acc *model.CFAccount
	var err error
	if accountID > 0 {
		acc, err = a.db.GetCFAccount(accountID)
		if err != nil {
			return nil, model.CFAccount{}, err
		}
		if acc == nil {
			return nil, model.CFAccount{}, fmt.Errorf("حساب Cloudflare پیدا نشد")
		}
	} else {
		acc, err = a.db.FirstCFAccount()
		if err != nil {
			return nil, model.CFAccount{}, err
		}
		if acc == nil {
			return nil, model.CFAccount{}, errCFAccountMissing
		}
	}
	if !acc.Enabled {
		return nil, *acc, fmt.Errorf("حساب Cloudflare «%s» غیرفعال است", acc.Name)
	}
	if strings.TrimSpace(acc.Token) == "" {
		return nil, *acc, fmt.Errorf("توکن حساب Cloudflare «%s» خالی است", acc.Name)
	}
	return cloudflare.New(acc.Token), *acc, nil
}

func (a *App) migrateCFAccounts(conf *config.Settings) {
	if v, _ := a.db.GetSetting("cf_accounts_migrated"); v == "1" {
		return
	}
	count, err := a.db.CountCFAccounts()
	if err != nil {
		a.logFor("cloudflare").Warn("could not check Cloudflare accounts", "error", err)
		return
	}
	if count == 0 {
		if token := strings.TrimSpace(conf.CloudflareToken); token != "" {
			acc := &model.CFAccount{Name: "Default", Token: token, Enabled: true}
			if err := a.db.CreateCFAccount(acc); err != nil {
				a.logFor("cloudflare").Warn("legacy Cloudflare token migration failed", "error", err)
				return
			}
			a.logFor("cloudflare").Info("legacy Cloudflare token migrated", "account_id", acc.ID)
		}
	}
	_ = a.db.SetSetting("cf_accounts_migrated", "1")
}

func (a *App) validateCFAccount(in *monitorInput) error {
	if !in.CFEnabled {
		return nil
	}
	if in.CFAccountID > 0 {
		acc, err := a.db.GetCFAccount(in.CFAccountID)
		if err != nil {
			return fmt.Errorf("بررسی حساب Cloudflare ناموفق بود")
		}
		if acc == nil {
			return fmt.Errorf("حساب Cloudflare انتخاب‌شده پیدا نشد")
		}
		if !acc.Enabled {
			return fmt.Errorf("حساب Cloudflare انتخاب‌شده غیرفعال است")
		}
		return nil
	}
	acc, err := a.db.FirstCFAccount()
	if err != nil {
		return fmt.Errorf("بررسی حساب Cloudflare ناموفق بود")
	}
	if acc == nil {
		return errCFAccountMissing
	}
	in.CFAccountID = acc.ID
	return nil
}

func queryAccountID(r *http.Request) int64 {
	id, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("account_id")), 10, 64)
	return id
}

func (a *App) handleCFStatus(w http.ResponseWriter, r *http.Request) {
	conf := a.config()
	accounts, err := a.db.ListCFAccounts()
	if err != nil {
		writeErr(w, 500, "دریافت حساب‌های Cloudflare ناموفق بود")
		return
	}
	out := map[string]any{
		"configured": len(accounts) > 0,
		"enabled":    conf.CloudflareEnabled,
		"connected":  false,
		"accounts":   accounts,
	}
	id := queryAccountID(r)
	if id == 0 {
		for _, acc := range accounts {
			if acc.Enabled {
				id = acc.ID
				break
			}
		}
	}
	if id == 0 && len(accounts) > 0 {
		id = accounts[0].ID
	}
	out["account_id"] = id
	if id == 0 {
		writeData(w, out)
		return
	}
	client, acc, err := a.cfClient(id)
	if err != nil {
		out["error"] = err.Error()
		out["account_name"] = acc.Name
		writeData(w, out)
		return
	}
	out["account_name"] = acc.Name
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	verify, err := client.Verify(ctx)
	if err != nil {
		out["error"] = err.Error()
		writeData(w, out)
		return
	}
	out["connected"] = verify.Status == "active"
	out["token_status"] = verify.Status
	writeData(w, out)
}

func (a *App) handleCFAccounts(w http.ResponseWriter, r *http.Request) {
	accounts, err := a.db.ListCFAccounts()
	if err != nil {
		writeErr(w, 500, "دریافت حساب‌های Cloudflare ناموفق بود")
		return
	}
	writeData(w, map[string]any{"accounts": accounts})
}

func (a *App) handleCFCreateAccount(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name    string `json:"name"`
		Token   string `json:"token"`
		Enabled *bool  `json:"enabled"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	body.Token = strings.TrimSpace(body.Token)
	if body.Name == "" {
		body.Name = "Cloudflare"
	}
	if len([]rune(body.Name)) > 80 {
		writeErr(w, 400, "نام حساب طولانی است")
		return
	}
	if body.Token == "" {
		writeErr(w, 400, "توکن API الزامی است")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	verify, err := cloudflare.New(body.Token).Verify(ctx)
	if err != nil {
		writeErr(w, 400, "تأیید توکن Cloudflare ناموفق بود: "+err.Error())
		return
	}
	if verify.Status != "active" {
		writeErr(w, 400, "توکن Cloudflare فعال نیست (وضعیت: "+verify.Status+")")
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	acc := &model.CFAccount{Name: body.Name, Token: body.Token, Enabled: enabled}
	if err := a.db.CreateCFAccount(acc); err != nil {
		writeErr(w, 500, "ذخیره حساب Cloudflare ناموفق بود")
		return
	}
	a.db.AddAudit("admin", "cloudflare.account.create", acc.Name)
	a.logFor("cloudflare").Info("cloudflare account created", "account_id", acc.ID, "name", acc.Name)
	writeData(w, map[string]any{"account": acc})
}

func (a *App) handleCFUpdateAccount(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	acc, err := a.db.GetCFAccount(id)
	if err != nil {
		writeErr(w, 500, "دریافت حساب Cloudflare ناموفق بود")
		return
	}
	if acc == nil {
		writeErr(w, 404, "حساب Cloudflare پیدا نشد")
		return
	}
	var body struct {
		Name    string `json:"name"`
		Token   string `json:"token"`
		Enabled *bool  `json:"enabled"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if name := strings.TrimSpace(body.Name); name != "" {
		if len([]rune(name)) > 80 {
			writeErr(w, 400, "نام حساب طولانی است")
			return
		}
		acc.Name = name
	}
	if token := strings.TrimSpace(body.Token); token != "" {
		ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
		verify, err := cloudflare.New(token).Verify(ctx)
		cancel()
		if err != nil {
			writeErr(w, 400, "تأیید توکن Cloudflare ناموفق بود: "+err.Error())
			return
		}
		if verify.Status != "active" {
			writeErr(w, 400, "توکن Cloudflare فعال نیست (وضعیت: "+verify.Status+")")
			return
		}
		acc.Token = token
	}
	if body.Enabled != nil {
		acc.Enabled = *body.Enabled
	}
	if err := a.db.UpdateCFAccount(acc); err != nil {
		writeErr(w, 500, "ذخیره حساب Cloudflare ناموفق بود")
		return
	}
	a.db.AddAudit("admin", "cloudflare.account.update", acc.Name)
	a.logFor("cloudflare").Info("cloudflare account updated", "account_id", acc.ID, "name", acc.Name, "enabled", acc.Enabled)
	writeData(w, map[string]any{"account": acc})
}

func (a *App) handleCFDeleteAccount(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	acc, _ := a.db.GetCFAccount(id)
	if acc == nil {
		writeErr(w, 404, "حساب Cloudflare پیدا نشد")
		return
	}
	if err := a.db.DeleteCFAccount(id); err != nil {
		writeErr(w, 500, "حذف حساب Cloudflare ناموفق بود")
		return
	}
	a.db.AddAudit("admin", "cloudflare.account.delete", acc.Name)
	a.logFor("cloudflare").Info("cloudflare account deleted", "account_id", id, "name", acc.Name)
	writeData(w, map[string]any{"ok": true})
}

func (a *App) handleCFZones(w http.ResponseWriter, r *http.Request) {
	client, _, err := a.cfClient(queryAccountID(r))
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	zones, err := client.ListZones(ctx)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeData(w, map[string]any{"zones": zones})
}

func (a *App) handleCFRecords(w http.ResponseWriter, r *http.Request) {
	zoneID := strings.TrimSpace(r.URL.Query().Get("zone_id"))
	if zoneID == "" {
		writeErr(w, 400, "شناسه Zone الزامی است")
		return
	}
	client, _, err := a.cfClient(queryAccountID(r))
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	recordType := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("type")))
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	records, err := client.ListRecords(ctx, zoneID, recordType, search)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeData(w, map[string]any{"records": records})
}

type cfRecordInput struct {
	AccountID int64  `json:"account_id"`
	ZoneID    string `json:"zone_id"`
	Type      string `json:"type"`
	Name      string `json:"name"`
	Content   string `json:"content"`
	TTL       int    `json:"ttl"`
	Proxied   bool   `json:"proxied"`
	Priority  int    `json:"priority"`
}

func (in *cfRecordInput) normalize() error {
	in.ZoneID = strings.TrimSpace(in.ZoneID)
	in.Type = strings.ToUpper(strings.TrimSpace(in.Type))
	in.Name = strings.TrimSpace(in.Name)
	in.Content = strings.TrimSpace(in.Content)
	if in.ZoneID == "" {
		return fmt.Errorf("شناسه Zone الزامی است")
	}
	if in.Type == "" || !validDNSRecordTypes[in.Type] {
		return fmt.Errorf("نوع رکورد DNS پشتیبانی نمی‌شود")
	}
	if in.Name == "" || in.Content == "" {
		return fmt.Errorf("نام و مقدار رکورد الزامی است")
	}
	if in.TTL <= 0 {
		in.TTL = 1
	}
	if in.Priority < 0 {
		in.Priority = 0
	}
	switch in.Type {
	case "A", "AAAA", "CNAME":
	default:
		in.Proxied = false
	}
	return nil
}

func (in cfRecordInput) record() model.DNSRecord {
	return model.DNSRecord{
		ZoneID: in.ZoneID, Type: in.Type, Name: in.Name, Content: in.Content,
		TTL: in.TTL, Proxied: in.Proxied, Priority: in.Priority,
	}
}

func (a *App) handleCFCreateRecord(w http.ResponseWriter, r *http.Request) {
	var in cfRecordInput
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := in.normalize(); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	client, acc, err := a.cfClient(in.AccountID)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	rec, err := client.CreateRecord(ctx, in.ZoneID, in.record())
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	a.db.AddAudit("admin", "cloudflare.record.create", rec.Name+" → "+rec.Content)
	a.logFor("cloudflare").Info("cloudflare record created", "account", acc.Name, "record", rec.Name, "type", rec.Type, "content", rec.Content)
	a.hub.broadcast("cloudflare.updated", map[string]any{"record": rec})
	writeData(w, map[string]any{"record": rec})
}

func (a *App) handleCFUpdateRecord(w http.ResponseWriter, r *http.Request) {
	recordID := strings.TrimSpace(r.PathValue("id"))
	if recordID == "" {
		writeErr(w, 400, "شناسه رکورد الزامی است")
		return
	}
	var in cfRecordInput
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := in.normalize(); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	client, acc, err := a.cfClient(in.AccountID)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	rec, err := client.UpdateRecord(ctx, in.ZoneID, recordID, in.record())
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	a.db.AddAudit("admin", "cloudflare.record.update", rec.Name+" → "+rec.Content)
	a.logFor("cloudflare").Info("cloudflare record updated", "account", acc.Name, "record", rec.Name, "type", rec.Type, "content", rec.Content)
	a.hub.broadcast("cloudflare.updated", map[string]any{"record": rec})
	writeData(w, map[string]any{"record": rec})
}

func (a *App) handleCFDeleteRecord(w http.ResponseWriter, r *http.Request) {
	recordID := strings.TrimSpace(r.PathValue("id"))
	if recordID == "" {
		writeErr(w, 400, "شناسه رکورد الزامی است")
		return
	}
	zoneID := strings.TrimSpace(r.URL.Query().Get("zone_id"))
	if zoneID == "" {
		writeErr(w, 400, "شناسه Zone الزامی است")
		return
	}
	client, acc, err := a.cfClient(queryAccountID(r))
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := client.DeleteRecord(ctx, zoneID, recordID); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	a.db.AddAudit("admin", "cloudflare.record.delete", recordID)
	a.logFor("cloudflare").Info("cloudflare record deleted", "account", acc.Name, "record_id", recordID, "zone_id", zoneID)
	a.hub.broadcast("cloudflare.updated", map[string]any{"record_id": recordID})
	writeData(w, map[string]any{"ok": true})
}

// handleCFUpdate is the legacy content-only endpoint. It remains for older
// clients and for the monitor detail screen.
func (a *App) handleCFUpdate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AccountID int64  `json:"account_id"`
		ZoneID    string `json:"zone_id"`
		RecordID  string `json:"record_id"`
		Content   string `json:"content"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	body.Content = strings.TrimSpace(body.Content)
	if body.ZoneID == "" || body.RecordID == "" || body.Content == "" {
		writeErr(w, 400, "شناسه Zone، رکورد و مقدار جدید الزامی است")
		return
	}
	client, acc, err := a.cfClient(body.AccountID)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	rec, err := client.UpdateRecordContent(ctx, body.ZoneID, body.RecordID, body.Content)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	a.db.AddAudit("admin", "cloudflare.update", rec.Name+" → "+rec.Content)
	a.logFor("cloudflare").Info("cloudflare record updated manually", "account", acc.Name, "record", rec.Name, "content", rec.Content)
	a.hub.broadcast("cloudflare.updated", map[string]any{"record": rec})
	writeData(w, map[string]any{"record": rec})
}

func (a *App) applyCFFailover(m model.Monitor, mode string) {
	conf := a.config()
	if !conf.CloudflareEnabled {
		return
	}
	if !m.CFEnabled || m.CFZoneID == "" || m.CFRecordID == "" {
		return
	}
	client, _, err := a.cfClient(m.CFAccountID)
	if err != nil {
		a.logCFAction(m, false, "حساب Cloudflare در دسترس نیست: "+err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	record, err := client.GetRecord(ctx, m.CFZoneID, m.CFRecordID)
	if err != nil {
		a.logCFAction(m, false, "دریافت رکورد Cloudflare ناموفق بود: "+err.Error())
		return
	}
	current := strings.TrimSpace(record.Content)
	var target, label string
	switch mode {
	case "failover":
		target = strings.TrimSpace(m.CFFailover)
		label = "IP پشتیبان"
	case "restore":
		target = strings.TrimSpace(m.CFPrimary)
		label = "IP اصلی"
	default:
		return
	}
	if target == "" {
		return
	}
	if sameContent(current, target) {
		a.logCFAction(m, true, "رکورد DNS از قبل روی "+label+" تنظیم شده است")
		return
	}
	safeSource := m.CFPrimary
	if mode == "restore" {
		safeSource = m.CFFailover
	}
	if safeSource != "" && !sameContent(current, safeSource) {
		msg := "مقدار فعلی رکورد (" + current + ") با مقدار مورد انتظار (" + safeSource + ") هم‌خوانی ندارد؛ برای جلوگیری از تغییر ناخواسته، سوییچ انجام نشد"
		a.logCFAction(m, false, msg)
		return
	}
	updated, err := client.UpdateRecordContent(ctx, m.CFZoneID, m.CFRecordID, target)
	if err != nil {
		a.logCFAction(m, false, "تغییر رکورد Cloudflare ناموفق بود: "+err.Error())
		return
	}
	a.logCFAction(m, true, "رکورد "+updated.Name+" با موفقیت به "+target+" ("+label+") تغییر کرد")
}

func (a *App) logCFAction(m model.Monitor, ok bool, message string) {
	if ok {
		a.logFor("cloudflare").Info("cloudflare DNS action", "monitor", m.Name, "message", message)
	} else {
		a.logFor("cloudflare").Warn("cloudflare DNS action failed", "monitor", m.Name, "message", message)
	}
	now := nowUnix()
	_ = a.db.UpdateMonitorCFAction(m.ID, message, now)
	_ = a.db.InsertNotification(&model.NotificationLog{
		At: now, Channel: "cloudflare", RuleName: "سوییچ خودکار DNS", Event: "cf_failover",
		Title: m.Name, Body: message, OK: ok, Error: map[bool]string{true: "", false: message}[ok],
	})
	a.db.AddAudit("system", "cloudflare.failover", m.Name+": "+message)
	a.hub.broadcast("cf.action", map[string]any{"monitor_id": m.ID, "ok": ok, "message": message})
}

func sameContent(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(strings.TrimSuffix(a, ".")), strings.TrimSpace(strings.TrimSuffix(b, ".")))
}
