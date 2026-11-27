# پالس — Pulse

**مرکز فرمان و پایش لحظه‌ای زیرساخت** — سرورهای VPS، دامنه‌ها، گواهی‌های SSL، رکوردهای DNS و کانفیگ‌های پروکسی؛ با **هندشیک واقعی Xray**، نمودارهای زنده، موتور قوانین، اعلان هوشمند در تلگرام و Web Push و ترمیم خودکار DNS با Cloudflare.

[![CI](https://github.com/AmiwrM/Pulse/actions/workflows/ci.yml/badge.svg)](https://github.com/AmiwrM/Pulse/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/AmiwrM/Pulse?label=release)](https://github.com/AmiwrM/Pulse/releases)
[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)](https://go.dev/)

پالس یک باینری Go است: رابط کاربری، API، دیتابیس SQLite و ایجنت سرور همه در یک فایل. بدون Node، بدون دیتابیس جداگانه و بدون وابستگی خارجی برای اجرا. فونت ایران‌سنس، PWA و طراحی ریسپانسیو داخل خود برنامه جاسازی شده‌اند.

**ریپازیتوری:** <https://github.com/AmiwrM/Pulse>

### خلاصهٔ نسخهٔ 1.6.1

- نصب‌کنندهٔ پنل و ایجنت حالا منتظر قفل apt/dpkg می‌ماند (تا ۱۰ دقیقه) و عملیات را دوباره تلاش می‌کند؛ اجرای هم‌زمان `unattended-upgrades` یا `cloud-init` دیگر نصب را متوقف نمی‌کند.
- نصب‌کننده نسخهٔ Caddy را بررسی می‌کند و در صورت قدیمی‌بودن، Caddy 2.6+ را از مخزن رسمی نصب می‌کند؛ اگر Caddy قدیمی `h2c` را نپذیرد، کانفیگ WebSocket جایگزین نوشته می‌شود تا HTTPS و اتصال ایجنت از کار نیفتد.
- پشتیبانی از Ubuntu 20.04 و Debian 11 به فهرست سیستم‌های پشتیبانی‌شده اضافه شد.

### امکانات کلیدی نسخهٔ 1.6

- **زبان انگلیسی** با دکمهٔ EN/FA، چیدمان خودکار LTR/RTL، تاریخ/عدد انگلیسی و ترجمهٔ کل رابط. انتخاب زبان در مرورگر ذخیره می‌شود.
- **لاگ‌های تفکیک‌شده**: صفحهٔ لاگ با دسته‌های پنل، زمان‌بند، مانیتور، Xray، کلادفلر، اعلان‌ها، ایجنت، امنیت و API؛ ذخیره در SQLite، حفظ ۱۴ روز/۲۰٬۰۰۰ رکورد و فرمان `pulse logs` با فیلتر.
- **CLI یکدست و کامل** برای پنل و ایجنت: start/stop/restart/status/logs/update/uninstall/version و `pulse port` برای تغییر پورت پنل.
- **عدم تداخل با Xray موجود**: نصب‌کننده نسخهٔ اختصاصی Pulse را در `/usr/local/lib/pulse/xray` می‌گذارد و به `/usr/local/bin/xray` یا سرویس Xray دست نمی‌زند؛ هسته‌های تست فقط با پورت تصادفی loopback و پوشهٔ موقت اجرا می‌شوند.
- **پورت پنل قابل تنظیم** با `--port` / `PULSE_PORT` و دستور `pulse port`؛ گزینهٔ `--behind-proxy` برای نودهایی که از قبل Xray/VPN و reverse proxy دارند.
- **مودال‌های موبایل سبک‌تر**: انیمیشن کوتاه‌تر، بدون سایه/بلور سنگین و جلوگیری از بسته‌شدن اتفاقی با کلیک اطراف.
- **اتصال زندهٔ ایجنت** با gRPC (و fallback خودکار به WebSocket و سپس REST)؛ گزارش منابع هر **۱ ثانیه** و نمایش فوری وضعیت اتصال/قطع.
- **تست از ایجنت انتخابی**: هر مانیتور می‌تواند هندشیک کانفیگ، TCP، ICMP، HTTP، DNS یا SSL را از شبکهٔ یک سرور دیگر اجرا کند؛ اگر ایجنت آفلاین باشد، وضعیت «هشدار» ثبت می‌شود و رخداد کاذب باز نمی‌شود.
- ایجنت قابلیت تست و وضعیت Xray خود را اعلام می‌کند؛ نصب‌کنندهٔ ایجنت Xray رسمی را هم نصب می‌کند تا هندشیک واقعی از همان سرور انجام شود.
- نصب ایجنت با **یک دستور** از داخل پنل؛ توکن در `/etc/pulse/agent.env`؛ مدیریت با `pulse-agent` / `pulse-a`.
- **تم روشن و تیره**، آیکون‌های یکدست، هاله‌های نرم‌تر، هدر تمام‌عرض و رابط ریسپانسیو.
- اصلاحات پایدارسازی: ماندگاری انتخاب ایجنت در مانیتور، پاک‌سازی ارجاع‌ها هنگام حذف ایجنت، و جلوگیری از رخداد کاذب هنگام خطای زیرساخت تست.

---

## فهرست

- [ویژگی‌ها](#ویژگیها)
- [نصب سریع روی VPS](#نصب-سریع-روی-vps)
- [دستورهای مدیریت VPS](#دستورهای-مدیریت-vps)
- [شروع سریع با Docker](#شروع-سریع-با-docker)
- [اجرای مستقیم و ساخت از سورس](#اجرای-مستقیم-و-ساخت-از-سورس)
- [HTTPS و دامنه](#https-و-دامنه)
- [پیکربندی مانیتورها](#پیکربندی-مانیتورها)
- [اعلان‌ها](#اعلانها)
- [موتور قوانین](#موتور-قوانین)
- [ترمیم خودکار DNS با Cloudflare](#ترمیم-خودکار-dns-با-cloudflare)
- [ایجنت سرور](#ایجنت-سرور)
- [پایش کانفیگ‌های پروکسی](#پایش-کانفیگهای-پروکسی)
- [تست واقعی پروکسی با Xray](#تست-واقعی-پروکسی-با-xray)
- [لاگ‌ها](#لاگها)
- [زبان](#زبان)
- [API](#api)
- [پشتیبان‌گیری](#پشتیبانگیری)
- [امنیت](#امنیت)
- [تنظیمات محیطی](#تنظیمات-محیطی)
- [توسعه و تست](#توسعه-و-تست)
- [عیب‌یابی](#عیبیابی)
- [ساختار پروژه](#ساختار-پروژه)
- [محدودیت‌های آگاهانه](#محدودیتهای-آگاهانه)

---

## ویژگی‌ها

| بخش | امکانات |
|---|---|
| **پایش سرویس** | HTTP/HTTPS (کد وضعیت، کلیدواژه، هدر، بدنه، Basic Auth، دنبال‌کردن ریدایرکت)، TCP، ICMP Ping، DNS (A/AAAA/CNAME/MX/TXT/NS)، گواهی SSL و کانفیگ پروکسی |
| **تست از ایجنت** | اجرای هر نوع بررسی از شبکهٔ ایجنت انتخابی (مثلاً سرور ایران) برای تست کانفیگ، پینگ، TCP، HTTP، DNS و SSL؛ با تشخیص خطای زیرساخت و بدون رخداد کاذب |
| **هندشیک واقعی** | موتور Xray v26.11.9 برای VMess/VLESS/Trojan/Shadowsocks/WireGuard؛ اجرای هستهٔ موقت + SOCKS محلی + درخواست HTTP واقعی از داخل تونل؛ بدون دست‌زدن به Xray سیستمی |
| **لاگ‌ها** | لاگ ساختاریافتهٔ SQLite با دسته‌بندی پنل/مانیتور/Xray/کلادفلر/اعلان/ایجنت/امنیت/API، صفحهٔ اختصاصی و فرمان `pulse logs` |
| **زبان** | فارسی و انگلیسی، چیدمان خودکار RTL/LTR، تاریخ و اعداد بومی، ذخیرهٔ انتخاب در مرورگر |
| **مدیریت** | CLI یکدست پنل/ایجنت: start/stop/restart/status/logs/update/uninstall/version و تغییر پورت پنل |
| **آپتایم و نمودار** | آپتایم ۲۴ ساعته و ۳۰ روزه، نمودار تأخیر، رخدادها، Sparkline، روند کلی ۲۴ ساعته، تاریخ شمسی و اعداد فارسی |
| **قوانین هوشمند** | رویدادمحور با آستانه، محدوده، محدودیت زمانی، کانال‌های تلگرام/Web Push/وب‌هوک و ساعات سکوت |
| **اعلان‌ها** | تلگرام (HTML)، Web Push (PWA روی اندروید و iPhone با iOS 16.4+)، وب‌هوک امضاشده با HMAC-SHA256، سابقه ارسال |
| **Cloudflare** | بررسی توکن، لیست Zone و رکوردها، تغییر دستی رکورد، سوییچ خودکار A/AAAA در قطعی و بازگردانی پس از بازیابی با محافظت از تغییرات دستی |
| **سرورها** | ایجنت سبک Go با اتصال زندهٔ gRPC/WebSocket و fallback REST؛ گزارش هر ۱ ثانیه از CPU، RAM، Disk، Load، شبکه، آپتایم، وضعیت سرویس‌های systemd و همتایان WireGuard |
| **امنیت** | حساب مدیر با bcrypt، نشست امضاشده، محدودیت تلاش ورود، CSRF، CSP، هدرهای امنیتی، توکن یک‌بارمصرف ایجنت |
| **PWA** | نصب روی موبایل/دسکتاپ، آفلاین‌سازی پوسته، Service Worker، اعلان Push، آیکون‌های اختصاصی |
| **رابط کاربری** | فارسی و RTL، فونت ایران‌سنس، **تم تیره و روشن**، هاله‌های نرم و سایه‌های لایه‌ای، آیکون‌های یکدست، انیمیشن، نمودارهای Canvas بدون کتابخانه خارجی، کاملاً ریسپانسیو |

---

## نصب سریع روی VPS

روی **Debian 11 / 12 و Ubuntu 20.04 / 22.04 / 24.04** با دسترسی root. پیشنهاد می‌شود اسکریپت را اول دانلود و بعد اجرا کنید:

```bash
curl -fsSL https://raw.githubusercontent.com/AmiwrM/Pulse/main/install.sh -o /tmp/pulse-install.sh
sudo bash /tmp/pulse-install.sh
```

اسکریپت تعاملی می‌پرسد آیا **HTTPS خودکار با دامنه** می‌خواهید یا نه. برای نصب کاملاً غیرتعاملی:

```bash
# با SSL خودکار (Caddy + Let's Encrypt)
sudo bash /tmp/pulse-install.sh \
  --domain status.example.com \
  --email you@example.com \
  --yes

# بدون SSL؛ فقط برای شبکهٔ خصوصی یا وقتی خودتان TLS را مدیریت می‌کنید
sudo bash /tmp/pulse-install.sh --no-ssl --yes

# روی نودی که از قبل Xray/VPN panel و reverse proxy دارد:
# فقط روی loopback گوش می‌دهد و به Caddy، پورت ۴۴۳ و فایروال دست نمی‌زند.
sudo bash /tmp/pulse-install.sh \
  --behind-proxy https://status.example.com \
  --port 8080 \
  --yes

# تغییر پورت داخلی پنل
sudo bash /tmp/pulse-install.sh --port 8090 --domain status.example.com --yes
```

نصب یک‌خطی (اسکریپت برای سؤال SSL از `/dev/tty` استفاده می‌کند):

```bash
curl -fsSL https://raw.githubusercontent.com/AmiwrM/Pulse/main/install.sh | sudo bash
```

این نصب‌کننده همه‌چیز را از **منبع رسمی** روی خود سرور دانلود می‌کند و هیچ باینری‌ای داخل ریپو نگه‌داری نمی‌شود:

| جزء | منبع |
|---|---|
| باینری Pulse | GitHub Releases همین ریپو؛ در صورت نبود ریلیز، ساخت از سورس با Go |
| Go (در صورت ساخت از سورس) | `https://go.dev/dl` |
| Xray-core | ریلیز رسمی `XTLS/Xray-core` |
| Caddy + SSL | مخزن رسمی Caddy |

نصب‌کننده به‌صورت خودکار:

- وابستگی‌های لازم (curl، unzip، ping، CA certificates) را نصب می‌کند؛
- Pulse را در `/usr/local/bin/pulse` و سرویس systemd با کاربر اختصاصی `pulse` نصب می‌کند؛
- **Xray** را برای هندشیک واقعی نصب می‌کند (پیش‌فرض نسخهٔ تست‌شده `v26.11.9`؛ قابل تغییر با `--xray-version latest`)؛
- یک **حساب مدیر** با رمز تصادفی می‌سازد و مشخصات را در ترمینال و فایل `/root/pulse-admin-credentials.txt` (مجوز 600) می‌نویسد؛
- در صورت تأیید، Caddy و **SSL خودکار Let's Encrypt** را برای دامنه تنظیم می‌کند؛ Caddyfile موجود حفظ می‌شود و کانفیگ Pulse در `conf.d/pulse.caddy` اضافه می‌شود؛
- در صورت فعال بودن UFW/firewalld، پورت‌های لازم را باز می‌کند و در پایان خلاصهٔ نصب را چاپ می‌کند.

### انتشار ریلیز برای نصب سریع‌تر

اولین نصب اگر هنوز Release نداشته باشید، به‌صورت خودکار از سورس می‌سازد. بعد از اولین push، یک تگ بسازید تا GitHub Actions باینری‌های لینوکس را برای نصب‌های بعدی آماده کند:

```bash
git tag v1.6.1
git push origin v1.6.1
```

خروجی workflow: `pulse-linux-amd64`، `pulse-linux-arm64` و `SHA256SUMS`. نصب‌کننده از این فایل‌ها استفاده می‌کند. اگر ریلیز موجود نباشد یا GitHub در دسترس نباشد، با `--source` یا `--binary`/`--binary-url` می‌توانید مسیر دیگری انتخاب کنید.

> اولین نصب قبل از انتشار Release، از سورس ساخته می‌شود و به Go 1.26+ و دسترسی به `go.dev` نیاز دارد. برای نصب سریع‌تر، ابتدا یک تگ بسازید تا GitHub Actions باینری‌ها را منتشر کند.

### گزینه‌های مهم نصب‌کننده

```bash
sudo bash /tmp/pulse-install.sh --help

# نسخهٔ مشخص از ریلیز
sudo bash /tmp/pulse-install.sh --release v1.6.1 --domain status.example.com --yes

# ساخت از سورس (مثلاً قبل از انتشار اولین ریلیز)
sudo bash /tmp/pulse-install.sh --source --domain status.example.com --yes

# باینری آفلاین یا از میرور
sudo bash /tmp/pulse-install.sh --binary /root/pulse-linux-amd64 --domain status.example.com --yes
```

### نکته‌های SSL

- رکورد `A` دامنه باید قبل از نصب به IP سرور اشاره کند.
- پورت‌های **80 و 443** باید از سمت فایروال ارائه‌دهنده نیز باز باشند.
- اگر دامنه در Cloudflare با ابر نارنجی (Proxied) است، برای صدور گواهی اولیه آن را روی **DNS only** بگذارید؛ بعد از صدور می‌توانید Proxied کنید (Caddy از HTTP-01 استفاده می‌کند).
- اگر نصب Caddy ممکن نشود، اسکریپت در حالت تعاملی اجازه می‌دهد بدون HTTPS ادامه دهید؛ در حالت غیرتعاملی متوقف می‌شود تا پنل ناخواسته روی HTTP عمومی قرار نگیرد.
- نصب‌کننده Caddy 2.6+ را برای مسیر gRPC با `h2c` ترجیح می‌دهد. اگر Caddy قدیمی‌تر باشد و بلوک `h2c` را نپذیرد، کانفیگ WebSocket نوشته می‌شود و اتصال ایجنت با WebSocket/REST ادامه می‌یابد.
- روی نصب موجود، اگر دامنه را عوض کردید، آدرس عمومی را در **تنظیمات** هم به‌روز کنید؛ مقدار ذخیره‌شده در پنل بر مقدار اولیهٔ محیط اولویت دارد.

> نصب مجدد/ارتقا با همان اسکریپت انجام می‌شود. قبل از جایگزینی باینری از دیتابیس قبلی در `/var/backups` نسخهٔ پشتیبان گرفته می‌شود و حساب مدیریت موجود بازنویسی نمی‌شود. برای ریسک کمتر، از دکمهٔ پشتیبان‌گیری پنل هم استفاده کنید.

---

## دستورهای مدیریت VPS

از نسخهٔ `1.6.0` پنل و ایجنت یک مجموعهٔ دستور یکدست دارند. `pulse-agent` و `pulse-a` همان باینری هستند و `pulse agent ...` هم ایجنت را هدف می‌گیرد:

```bash
# پنل
pulse start | stop | restart | status
pulse logs [--lines N] [--follow] [--category monitor] [--level warn]
pulse update [--release v1.6.1] [--no-restart]
pulse port                       # نمایش پورت فعلی
pulse port 8090                  # تغییر پورت پنل + Caddy و ری‌استارت
pulse uninstall [--purge] [--yes]
pulse version

# ایجنت
pulse-agent start | stop | restart | status
pulse-agent logs [--lines N] [--follow] [--journal]
pulse-agent update
pulse-agent uninstall [--purge] [--yes]
pulse-agent version
```

- `pulse logs` پیش‌فرض از دیتابیس لاگ برنامه می‌خواند؛ `--journal` آن را به journalctl برمی‌گرداند. `pulse-agent logs` همیشه journalctl است چون ایجنت دیتابیس محلی ندارد.
- `uninstall` سرویس و یونیت را حذف می‌کند ولی دیتابیس/تنظیمات را نگه می‌دارد؛ `--purge` آن‌ها را هم پاک می‌کند. باینری `/usr/local/bin/pulse` عمداً حذف نمی‌شود تا اگر ایجنت روی همان هاست است از کار نیفتد.
- `pulse port 8090` یونیت systemd و بلوک Caddy پالس را به‌روزرسانی می‌کند، بکاپ می‌گیرد و سرویس‌ها را ری‌استارت/ری‌لود می‌کند. برای تغییر فقط با نصب‌کننده: `install.sh --port 8090`.

`pulse update` و `pulse-agent update` باینری را از آخرین Release رسمی دانلود می‌کنند، SHA256 را بررسی می‌کنند، فایل را اتمیک جایگزین می‌کنند و سرویس را ری‌استارت می‌کنند. برای نسخهٔ مشخص:

```bash
pulse update --release v1.6.1
pulse-agent update --release v1.6.1
```

> برای اولین آپدیت از نسخهٔ قدیمی(source) که هنوز این دستور را ندارد، از بخش [آپدیت دستی](#آپدیت-دستی) استفاده کن. اگر هنوز Release نساخته‌ای، `pulse update` خطا می‌دهد و باید نسخه را با installer/سورس نصب کنی.

### آپدیت دستی

برای پنل، بعد از ساخته شدن Release:

```bash
curl -fL https://github.com/AmiwrM/Pulse/releases/latest/download/pulse-linux-amd64 -o /tmp/pulse
sudo install -m 0755 /tmp/pulse /usr/local/bin/pulse
sudo systemctl restart pulse
```

برای ایجنت:

```bash
curl -fL https://github.com/AmiwrM/Pulse/releases/latest/download/pulse-linux-amd64 -o /tmp/pulse
sudo install -m 0755 /tmp/pulse /usr/local/bin/pulse
sudo ln -sfn /usr/local/bin/pulse /usr/local/bin/pulse-agent
sudo ln -sfn /usr/local/bin/pulse /usr/local/bin/pulse-a
sudo systemctl restart pulse-agent
```

بعد از این آپدیت، از دفعهٔ بعد `pulse update` یا `pulse-agent update` کافی است. اگر هنوز Release نداری، نصب مجدد با installer از سورس انجام می‌شود:

```bash
curl -fsSL https://raw.githubusercontent.com/AmiwrM/Pulse/main/install.sh -o /tmp/pulse-install.sh
sudo bash /tmp/pulse-install.sh --help
```

---

## شروع سریع با Docker

پیش‌نیاز: Docker و Docker Compose.

```bash
docker compose up -d --build
```

سپس باز کنید:

```
http://IP-SERVER:8080
```

در اولین ورود، صفحهٔ **راه‌اندازی اولیه** باز می‌شود و یک حساب مدیر با رمز قوی می‌سازید.

تصویر Docker به‌صورت پیش‌فرض **Xray-core v26.11.9** را برای تست واقعی پروکسی‌ها همراه دارد. اگر دانلود Xray از GitHub در شبکهٔ شما مسدود است، آدرس آینهٔ فایل را بدهید:

```bash
XRAY_DOWNLOAD_URL="https://your-mirror.example/Xray-linux-64.zip" docker compose build --no-cache
docker compose up -d
```

پس از بالا آمدن، در **تنظیمات → هستهٔ Xray** وضعیت و نسخه را ببینید و «بررسی مجدد» را بزنید.

اگر دانلود ماژول‌های Go در شبکه شما کند است، از آینه استفاده کنید:

```bash
GOPROXY=https://mirrors.aliyun.com/goproxy/,direct docker compose build --no-cache
docker compose up -d
```

دستورهای روزمره:

```bash
docker compose logs -f --tail=100
docker compose restart
docker compose down
```

داده‌ها در volume با نام `pulse-data` ذخیره می‌شوند. برای پشتیبان‌گیری:

```bash
docker run --rm -v pulse-data:/data -v "$PWD":/backup alpine \
  tar czf /backup/pulse-data.tar.gz -C /data .
```

> برای پایش ICMP، سرویس با `cap_add: NET_RAW` اجرا می‌شود. بدون این قابلیت، مانیتورهای Ping خطای دسترسی می‌دهند.

---

## اجرای مستقیم و ساخت از سورس

نصب‌کنندهٔ اصلی `install.sh` سرویس systemd، کاربر و دیتابیس را می‌سازد و در صورت تأیید SSL، Caddy را هم تنظیم می‌کند. اگر می‌خواهید خودتان باینری را بسازید:

```bash
git clone https://github.com/AmiwrM/Pulse.git
cd Pulse
make linux
sudo bash install.sh --binary dist/pulse-linux-amd64
```

یا فقط باینری را اجرا کنید:

```bash
go run ./cmd/pulse --addr 127.0.0.1:8080 --db ./data/pulse.db
```

فلگ‌ها: `--addr`، `--port`، `--db`، `--debug`، `--version`. مقادیر از محیط هم خوانده می‌شوند (`PULSE_ADDR`، `PULSE_PORT`، `PULSE_DB_PATH`). `--port` پورت داخل `--addr` را بازنویسی می‌کند.

سرویس نصب‌شده روی `127.0.0.1:8080` گوش می‌دهد تا Caddy در جلوی آن قرار بگیرد. برای مدیریت:

```bash
sudo systemctl status pulse --no-pager
sudo journalctl -u pulse -f
```

**Xray** در نصب‌کننده به‌صورت خودکار از ریلیز رسمی نصب می‌شود. در اجرای دستی می‌توانید از همان گزینه‌های نصب‌کننده استفاده کنید:

```bash
sudo bash /tmp/pulse-install.sh --force-xray --xray-version latest
```

سپس در **تنظیمات → هستهٔ Xray** دکمهٔ «بررسی مجدد» را بزنید. مسیر پیش‌فرض `xray` در PATH است و `/usr/local/bin/xray` خودکار پیدا می‌شود.

---

## HTTPS و دامنه

Web Push و کوکی امن به HTTPS نیاز دارند. پیشنهاد: یک زیردامنه مثل `status.example.com` بسازید و آن را به IP سرور وصل کنید.

### Caddy (ساده‌ترین)

نصب‌کننده این کار را خودکار انجام می‌دهد. اگر Caddy را دستی مدیریت می‌کنید، این بلوک را در Caddyfile اضافه کنید:

```caddyfile
status.example.com {
	encode gzip

	# gRPC live agent link (Caddy terminates TLS, h2c to Pulse)
	@pulse_grpc {
		path /pulse.agent.v1.AgentLink/*
		header Content-Type application/grpc*
	}
	handle @pulse_grpc {
		reverse_proxy 127.0.0.1:8080 {
			transport http {
				versions h2c 2
			}
		}
	}

	# UI, REST and the WebSocket agent fallback
	handle {
		reverse_proxy 127.0.0.1:8080 {
			flush_interval -1
		}
	}
	header {
		-Server
	}
}
```

`flush_interval -1` برای زنده‌ماندن SSE ضروری است. اگر بلوک `@pulse_grpc` را اضافه نکنید، ایجنت‌ها به‌صورت خودکار از WebSocket استفاده می‌کنند و قابلیت تست از راه دور از دست نمی‌رود؛ فقط transport از gRPC به WebSocket تغییر می‌کند. اگر دامنه پشت Cloudflare است، برای gRPC باید HTTP/2 و gRPC در Cloudflare مجاز باشد؛ در غیر این صورت WebSocket fallback فعال می‌شود.

### Nginx

```nginx
server {
    listen 443 ssl http2;
    server_name status.example.com;

    ssl_certificate     /etc/letsencrypt/live/status.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/status.example.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # SSE / live updates
        proxy_buffering off;
        proxy_cache off;
        proxy_read_timeout 3600s;

        # WebSocket fallback for the agent live link
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

برای gRPC روی Nginx می‌توانید مسیر ایجنت را جدا کنید (نسخهٔ Nginx باید HTTP/2 و `grpc_pass` داشته باشد):

```nginx
location /pulse.agent.v1.AgentLink/ {
    grpc_pass grpc://127.0.0.1:8080;
    grpc_read_timeout 3600s;
    grpc_send_timeout 3600s;
}
```

اگر این بلوک را اضافه نکنید، ایجنت از WebSocket استفاده می‌کند.

بعد از فعال‌سازی HTTPS، آدرس عمومی را در **تنظیمات** وارد کنید؛ همین آدرس برای VAPID و ساخت فرمان نصب ایجنت استفاده می‌شود.

---

## پیکربندی مانیتورها

از دکمهٔ **مانیتور جدید** استفاده کنید. انواع بررسی:

| نوع | مثال هدف | تنظیمات مهم |
|---|---|---|
| HTTP | `https://example.com/health` | کد انتظار (`2xx` پیش‌فرض)، کلیدواژه، متد، هدرهای JSON، بدنه، دنبال‌کردن ریدایرکت، نادیده‌گرفتن TLS |
| TCP | `example.com:443` | مهلت بررسی |
| ICMP | `1.1.1.1` | نصب بودن `ping` و دسترسی `NET_RAW` |
| DNS | `example.com` | نوع رکورد، مقدار انتظار (چند مقدار با کاما) و DNS Server سفارشی |
| SSL | `example.com` | SNI، آستانه هشدار روزهای باقی‌مانده |
| Proxy | نام ساب | متن/URL سابسکریپشن، حداقل کانفیگ سالم |

- **اجرای تست از:** برای هر نوع مانیتور می‌توانید یک ایجنت را انتخاب کنید. بررسی از همان سرور و با همان مسیر شبکه انجام می‌شود؛ مناسب برای تست کانفیگ از یک نود ایران، بررسی دسترسی از یک منطقهٔ دیگر یا تفکیک خطای مقصد از خطای پنل. گزینهٔ پیش‌فرض «پنل اصلی» است. اگر ایجنت انتخاب‌شده آفلاین باشد یا اتصال زنده نداشته باشد، نتیجه به‌صورت **هشدار (degraded)** با خطای «اتصال زندهٔ ایجنت برقرار نیست» ثبت می‌شود و برای مقصد رخداد کاذب باز نمی‌شود.
- برای تست کانفیگ از ایجنت، Xray باید روی همان ایجنت نصب باشد. نصب‌کنندهٔ جدید ایجنت این کار را به‌صورت پیش‌فرض انجام می‌دهد؛ در صورت نصب با `--no-xray`، فقط بررسی سریع TCP/TLS انجام می‌شود.
- **آستانه خطا** تعیین می‌کند پس از چند بررسی ناموفق، وضعیت «قطع» شود. **آستانه بازیابی** هم تعداد بررسی موفق لازم برای «سالم» شدن است؛ این دو از هشدارهای زودگذر جلوگیری می‌کنند.
- برای هر مانیتور می‌توانید سوییچ خودکار DNS را فعال کنید (بخش Cloudflare).
- گواهی SSL به‌صورت خودکار برای مانیتورهای HTTP و SSL ثبت می‌شود و در کارت‌ها هشدار انقضا نمایش داده می‌شود.
- وضعیت‌ها: `سالم`، `قطع`، `در انتظار`، `متوقف` (غیرفعال‌شده).

---

## اعلان‌ها

### تلگرام

1. در تلگرام به [@BotFather](https://t.me/BotFather) پیام دهید و با `/newbot` ربات بسازید.
2. توکن ربات (`123456789:AA...`) را در صفحهٔ **اعلان‌ها** وارد کنید.
3. برای Chat ID:
   - به ربات خود پیام دهید؛
   - از [@userinfobot](https://t.me/userinfobot) یا `https://api.telegram.org/bot<TOKEN>/getUpdates` استفاده کنید.
4. برای کانال، ربات را ادمین کانال کنید و Chat ID را با `@channelusername` یا شناسهٔ عددی وارد کنید.
5. دکمهٔ **ذخیره** و سپس **تست** را بزنید.

### Web Push (اندروید / آیفون / دسکتاپ)

- پنل باید با **HTTPS** باز شود (لوکال‌هاست استثناست).
- در **اعلان‌ها** دکمهٔ **فعال‌سازی** را بزنید و اجازهٔ اعلان را بدهید.
- در اندروید/کروم، از منوی مرورگر **Install app / Add to Home Screen** را بزنید.
- در **iPhone/iPad** (iOS 16.4+): پنل را در Safari باز کنید، دکمهٔ Share → **Add to Home Screen**، سپس اپ را از صفحهٔ اصلی باز کنید و اعلان را فعال کنید. Web Push در تب معمولی Safari کار نمی‌کند.
- اعلان تست فقط به اشتراک‌های همین سامانه ارسال می‌شود.

### وب‌هوک

هر رویداد به‌صورت JSON POST می‌شود:

```json
{
  "event": "down",
  "severity": "critical",
  "title": "🔴 قطع سرویس: سایت اصلی",
  "body": "...",
  "at": 1795088590,
  "monitor": { "...": "..." }
}
```

اگر **Secret** تنظیم شود، هدر `X-Pulse-Signature: sha256=...` با HMAC-SHA256 امضا می‌شود.

### ساعات سکوت

اعلان‌های غیرحیاتی در بازهٔ تعیین‌شده ارسال نمی‌شوند. به‌صورت پیش‌فرض هشدارهای قطعی (`down` و آفلاین‌شدن ایجنت) در ساعات سکوت هم می‌رسند؛ این رفتار قابل تغییر است. فاصلهٔ تکرار هشدار قطعی را هم می‌توانید تعیین کنید.

---

## موتور قوانین

هر قانون = **رویداد + محدوده + آستانه + کانال + فاصلهٔ زمانی**.

| رویداد | کاربرد |
|---|---|
| `down` / `up` | قطع و بازیابی سرویس؛ برای هر گذر وضعیت ارسال می‌شود |
| `cert_expiring` | باقی‌ماندن کمتر از N روز تا انقضای SSL (پیش‌فرض ۱۴) |
| `cpu_high` / `ram_high` / `disk_high` | عبور مصرف سرور از آستانه (پیش‌فرض ۹۰٪) |
| `agent_offline` | قطع شدن گزارش ایجنت |
| `dns_mismatch` | تفاوت مقدار واقعی DNS با مقدار انتظار |

کانال «همه کانال‌های فعال» به هر کانالی که در صفحهٔ اعلان‌ها تنظیم شده باشد می‌فرستد. هشت قانون پیش‌فرض در اولین راه‌اندازی ساخته می‌شوند و بعد از آن، حذف/ویرایش شما دست‌نخورده می‌ماند.

---

## ترمیم خودکار DNS با Cloudflare

1. در Cloudflare → My Profile → **API Tokens** یک توکن بسازید با دسترسی‌های:
   - `Zone : Zone : Read`
   - `Zone : DNS : Edit`
2. توکن را در صفحهٔ **کلادفلر** ذخیره و اتصال را بررسی کنید.
3. هنگام ساخت/ویرایش مانیتور، بخش **سوییچ خودکار DNS** را باز کنید:
   - Zone و رکورد `A`/`AAAA` را انتخاب کنید؛
   - IP اصلی و IP پشتیبان را وارد کنید.
4. اگر مانیتور پس از رسیدن به آستانهٔ خطا قطع شود، پالس مقدار رکورد را از IP اصلی به IP پشتیبان تغییر می‌دهد. پس از بازیابی، رکورد به IP اصلی برمی‌گردد.
5. **محافظت:** قبل از هر تغییر، مقدار فعلی رکورد با مقدار مورد انتظار مقایسه می‌شود. اگر رکورد را دستی تغییر داده باشید، پالس آن را بازنویسی نمی‌کند و فقط در سابقه ثبت می‌کند.

تمام تغییرات در **سابقهٔ اعلان‌ها** و audit log ثبت می‌شوند. تغییر دستی رکورد نیز از همان صفحهٔ کلادفلر ممکن است.

---

## ایجنت سرور

برای دیدن CPU/RAM/Disk، وضعیت سرویس‌ها و WireGuard، ایجنت را روی هر VPS نصب کنید. ایجنت همان باینری پالس است و در حالت `--agent` اجرا می‌شود. از نسخهٔ `1.5.0` ارتباط اصلی یک **stream دوطرفهٔ gRPC** است که آمار را هر ۱ ثانیه می‌فرستد و دستور «اجرای تست» را از پنل می‌گیرد. اگر مسیر شبکه/reverse proxy از HTTP/2 پشتیبانی نکند، ایجنت به‌صورت خودکار **WebSocket** و در نهایت **REST** را انتخاب می‌کند؛ بنابراین پایش منابع در قطعی موقت یا پروکسی‌های ناسازگار متوقف نمی‌شود. ایجنت‌های قدیمی‌تر (نسخهٔ ۱.۴) فقط REST هستند و می‌توانند منابع را گزارش کنند، اما برای «تست از ایجنت» باید به ۱.۵ به‌روزرسانی شوند.

### ۱. ساخت توکن

در صفحهٔ **سرورها** → **افزودن سرور** یک نام وارد کنید. توکن و **دستور نصب یک‌خطی** فقط یک‌بار نمایش داده می‌شوند.

### ۲. نصب

اسکریپت ایجنت را روی VPS دانلود و اجرا کنید (باینری از GitHub Releases همین ریپو دانلود می‌شود):

در پنجرهٔ ساخت ایجنت، دستور **نصب یک‌خطی** آماده است. همان را روی VPS مقصد اجرا کن. معادل دستی آن:

```bash
curl -fsSL https://raw.githubusercontent.com/AmiwrM/Pulse/main/deploy/install-agent.sh | \
  sudo bash -s -- \
    --server https://status.example.com \
    --token TOKEN_FROM_PANEL \
    --name vps-frankfurt-1 \
    --services "xray,wireguard"
```

نصب‌کننده به‌صورت پیش‌فرض آخرین Xray رسمی را هم نصب می‌کند تا تست واقعی کانفیگ از همان ایجنت انجام شود. برای صرفه‌جویی در فضا یا اگر Xray را خودتان مدیریت می‌کنید، `--no-xray` را اضافه کنید. برای مسیر سفارشی، `PULSE_XRAY_BIN=/path/to/xray` را در `/etc/pulse/agent.env` قرار دهید.

در اتصال زنده، فاصلهٔ گزارش **۱ ثانیه** است (حتی اگر در یونیت قدیمی `--interval 5s` مانده باشد)؛ مقدار `--interval` برای fallback REST و سازگاری نگه داشته شده است.

`pulse-agent update` فقط باینری ایجنت را به‌روزرسانی می‌کند و Xray نصب نمی‌کند. برای اضافه‌شدن Xray به یک ایجنت موجود، همان دستور نصب یک‌خطی را با `--server` و `--token` فعلی دوباره اجرا کنید (توکن در `/etc/pulse/agent.env` است) یا Xray را جداگانه نصب کنید.

مدیریت ایجنت:

```bash
pulse-agent status
pulse-agent restart
pulse-agent logs
pulse-agent update

# همین دستورها با نام کوتاه pulse-a هم کار می‌کنند
pulse-a restart
```

اگر هنوز ریلیزی منتشر نشده است، با `--binary /path/to/pulse-linux-amd64` از باینری ساخته‌شدهٔ خودتان استفاده کنید.

توکن در `/etc/pulse/agent.env` با مجوز 600 نگه‌داری می‌شود و داخل فایل systemd قرار نمی‌گیرد. اجرای تک‌باره برای تست:

```bash
sudo bash -c 'set -a; . /etc/pulse/agent.env; set +a; /usr/local/bin/pulse --agent --once'
```

پس از نصب، کارت سرور در صفحهٔ **سرورها** ظاهر می‌شود و نمودار ۲۴ ساعته/۶ ساعته/۳ روزه در دسترس است. لینک دانلود ایجنت در تنظیمات، به‌صورت پیش‌فرض به ریلیز رسمی همین ریپو برای معماری پنل اشاره می‌کند و برای معماری‌های دیگر یا میرور قابل تغییر است.

### چه چیزی ارسال می‌شود؟

- CPU، RAM، Disk، Load 1m، Uptime و نرخ دریافت/ارسال شبکه — هر ۱ ثانیه
- وضعیت سرویس‌های systemd که با `--services "nginx,xray"` مشخص کرده‌اید
- اطلاعات WireGuard با فلگ `--wg` (پیش‌فرض روشن): همتاها، handshake و ترافیک
- قابلیت اجرای تست و وضعیت Xray (برای انتخاب ایجنت به‌عنوان محل تست)

ایجنت در بازهٔ پیش‌فرض **۱ ثانیه** گزارش REST می‌دهد و UI از طریق SSE همان لحظه به‌روز می‌شود. تاریخچهٔ نمودار هر ۱۰ ثانیه ذخیره می‌شود تا دیتابیس با گزارش‌های یک‌ثانیه‌ای سنگین نشود. اگر بیش از `Agent offline after` ثانیه گزارش نرسد، قانون `agent_offline` اجرا می‌شود.

> توکن ایجنت در `/etc/pulse/agent.env` با مجوز `600` ذخیره می‌شود و داخل فایل unit قرار نمی‌گیرد. در صورت لو رفتن، از پنل **توکن جدید** بسازید.

---

## پایش کانفیگ‌های پروکسی

نوع **کانفیگ پروکسی** این ورودی‌ها را می‌پذیرد:

- لینک تکی: `vmess://`، `vless://`، `trojan://`، `ss://`، `hysteria2://`، `hy2://`، `tuic://`
- متن چندخطی شامل چند لینک
- لینک ساب‌سکریپشن HTTP(S) (Base64، Clash YAML ساده یا لیست URI)
- متن کامل کانفیگ WireGuard (`[Interface]` + `Endpoint`)
- خطوط سادهٔ `host:port`

برای هر کانفیگ ابتدا یک بررسی سریع دسترسی انجام می‌شود؛ سپس اگر هستهٔ Xray فعال باشد، برای پروتکل‌های پشتیبانی‌شده **هندشیک واقعی** انجام می‌شود: یک هستهٔ موقت Xray با SOCKS محلی بالا می‌آید و یک درخواست HTTP/HTTPS واقعی از داخل تونل به URL تست زده می‌شود. نتیجهٔ هر کانفیگ شامل روش تست (Xray/TCP/TLS/UDP)، تأخیر، وضعیت و خطای دقیق در جزئیات مانیتور نمایش داده می‌شود. در نسخهٔ جدید اگر بررسی سریع TCP/TLS یک کانفیگ Reality/CDN/WebSocket را رد کند، تست واقعی Xray همچنان روی آن انجام می‌شود و نتیجهٔ نهایی را تعیین می‌کند.

**نکته‌ها:**
- VMess، VLESS، Trojan، Shadowsocks، WireGuard و SOCKS با Xray تست واقعی می‌شوند.
- Hysteria2/TUIC و پلاگین‌های Shadowsocks در این نسخهٔ پالس هندشیک واقعی Xray ندارند و برای آن‌ها فقط بررسی دسترسی انجام می‌شود.
- برای WireGuard واقعی، Xray در حالت userspace و بدون TUN تست می‌کند؛ PrivateKey، Address و Peer لازم است.
- Reality با کلید عمومی (`pbk`) و ShortID واقعی تست می‌شود؛ بررسی اولیهٔ TCP فقط برای رد کردن سریع کانفیگ‌های مرده است.
- بررسی UDP در حالت fallback تقریبی است: timeout به معنی «رد نشدن فعال» در نظر گرفته می‌شود.
- دکمهٔ **بررسی فوری** حالا تا پایان بررسی صبر می‌کند و نتیجهٔ تازه را نشان می‌دهد؛ برای کانفیگ‌های بزرگ تا حدود ۹۰ ثانیه ممکن است طول بکشد.

---

## تست واقعی پروکسی با Xray

### چرا Xray؟

فقط `TCP connect` یا TLS دستی ثابت نمی‌کند که UUID/رمز درست است، پروتکل کار می‌کند و خروجی تونل واقعاً به اینترنت وصل می‌شود. پالس یک هستهٔ موقت Xray برای هر کانفیگ اجرا می‌کند، SOCKS آن را روی `127.0.0.1` بالا می‌آورد و از طریق آن یک درخواست واقعی به URL تست می‌فرستد. بنابراین این موارد بررسی می‌شوند:

- دست‌دهی واقعی پروتکل و احراز هویت (VMess/VLESS/Trojan/...)
- عبور واقعی ترافیک و خروج از سرور پروکسی
- تأخیر واقعی درخواست از داخل تونل
- خطاهای دقیق در صورت اشتباه بودن UUID، رمز، Reality، SNI یا شبکه

### نصب و به‌روزرسانی

- نصب‌کنندهٔ اصلی، Xray را از ریلیز رسمی `XTLS/Xray-core` نصب می‌کند (پیش‌فرض نسخهٔ تست‌شده `v26.11.9`).
- به‌روزرسانی روی هاست:

```bash
curl -fsSL https://raw.githubusercontent.com/AmiwrM/Pulse/main/install.sh -o /tmp/pulse-install.sh
sudo bash /tmp/pulse-install.sh --force-xray --xray-version latest
# یا نسخهٔ مشخص:
sudo bash /tmp/pulse-install.sh --force-xray --xray-version v26.11.9
```

- تصویر Docker به‌صورت پیش‌فرض **Xray v26.11.9** را نصب می‌کند.
- مسیر باینری در **تنظیمات → هستهٔ Xray** قابل تغییر است (پیش‌فرض `xray` در PATH).
- نسخهٔ نصب‌شده، مسیر و خطا در همان صفحه و در جزئیات مانیتور نمایش داده می‌شود.

### تنظیمات مانیتور

در فرم مانیتور پروکسی:

- **اجرای تست از** را انتخاب کنید: پنل اصلی یا یک ایجنت آنلاین. برای تست از شبکهٔ ایران، ایجنت همان سرور را انتخاب کنید.
- **هندشیک واقعی با Xray** روشن باشد (پیش‌فرض).
- **آدرس تست اتصال** را وارد کنید؛ پیش‌فرض `https://www.gstatic.com/generate_204` است. این آدرس باید از سمت سرور پروکسی قابل دسترس باشد و کد 2xx/3xx برگرداند.
- **حداکثر کانفیگ برای تست واقعی** را برای سابسکریپشن‌های بزرگ محدود کنید (پیش‌فرض ۱۰ کانفیگ).
- مهلت بررسی پروکسی حداقل ۹۰ ثانیه در نظر گرفته می‌شود؛ هر کانفیگ در نهایت حداکثر ۱۴ ثانیه فرصت دارد.

اگر Xray نصب نباشد یا اجرا نشود، مانیتور به‌جای نمایش سبز کاذب، وضعیت **هشدار** می‌گیرد و در جزئیات، دلیل عدم انجام تست واقعی و روش fallback را نشان می‌دهد.

> تست واقعی برای هر کانفیگ یک پروسهٔ Xray موقت اجرا می‌کند. برای سابسکریپشن‌های بزرگ، فاصلهٔ بررسی را بیشتر و تعداد کانفیگ‌های تست واقعی را محدود نگه دارید.

> **بدون تداخل با Xray سیستمی:** نصب‌کنندهٔ پالس نسخهٔ اختصاصی خودش را در `/usr/local/lib/pulse/xray` نگه می‌دارد، `/usr/local/bin/xray` را بازنویسی نمی‌کند و هیچ سرویس Xray یا پنل VPN را متوقف/ری‌استارت نمی‌کند. هر تست فقط یک هستهٔ موقت با پورت تصادفی `127.0.0.1`، پوشهٔ موقت اختصاصی و همان پروسهٔ فرزند را اجرا/پایان می‌دهد. روی نودی که Xray از قبل روی ۴۴۳ است، از `--behind-proxy` استفاده کنید تا Caddy و پورت ۴۴۳ دست نخورند.

---

## لاگ‌ها

پالس لاگ‌های ساختاریافته را در همان دیتابیس SQLite (جدول `app_logs`) نگه می‌دارد. صفحهٔ **لاگ‌ها** در منوی سیستم، دسته‌های زیر را جدا نشان می‌دهد:

| دسته | نمونه |
|---|---|
| `panel` | شروع/توقف پنل، تغییر تنظیمات |
| `scheduler` | خطاهای زمان‌بند |
| `monitor` | قطع/بازیابی، تغییر مانیتور، خطای probe |
| `xray` | وضعیت هسته، تست واقعی، خطای پروکسی |
| `cloudflare` | تغییر رکورد، failover و خطاهای DNS |
| `notify` | ارسال موفق/ناموفق تلگرام، Web Push و وب‌هوک |
| `agent` | اتصال/قطع لینک، ساخت/حذف/چرخش توکن، تست از راه دور |
| `security` | ورود موفق/ناموفق، محدودیت تلاش و CSRF |
| `api` | درخواست‌های تغییردهنده و خطاهای API |

- صفحهٔ لاگ فیلتر دسته، سطح (`debug/info/warn/error`) و جستجو دارد و هر ۴ ثانیه به‌روز می‌شود.
- لاگ‌ها ۱۴ روز یا حداکثر ۲۰٬۰۰۰ رکورد نگه داشته می‌شوند.
- در CLI:

```bash
pulse logs --category xray --level warn --lines 100
pulse logs --follow
pulse logs --journal          # journalctl خام
```

- لاگ ایجنت جداگانه در journal همان سرور است: `pulse-agent logs --follow`.

---

## زبان

دکمهٔ **EN/FA** در نوار بالا زبان را عوض می‌کند. در حالت انگلیسی، جهت صفحه LTR می‌شود، تاریخ‌ها میلادی/انگلیسی و اعداد لاتین نمایش داده می‌شوند. انتخاب در `localStorage` مرورگر ذخیره می‌شود و نیازی به تغییر تنظیمات سرور نیست.

---

## API

تمام پاسخ‌ها JSON با ساختار `{"ok": true, "data": ...}` یا `{"ok": false, "error": "..."}` هستند. احراز هویت با کوکی نشست انجام می‌شود، به‌جز endpoint ایجنت.

| متد | مسیر | توضیح |
|---|---|---|
| `GET` | `/api/health` | سلامت عمومی |
| `GET` | `/api/auth/state` | وضعیت ورود/راه‌اندازی |
| `POST` | `/api/auth/setup` | ساخت حساب اولیه |
| `POST` | `/api/auth/login` / `logout` | ورود/خروج |
| `GET` | `/api/summary` | خلاصه داشبورد، روند ۲۴ ساعته |
| `GET/POST` | `/api/monitors` | فهرست/ساخت مانیتور |
| `GET/PUT/DELETE` | `/api/monitors/{id}` | جزئیات/ویرایش/حذف |
| `POST` | `/api/monitors/{id}/check?wait=1` / `toggle` | بررسی فوری و انتظار برای نتیجه / توقف |
| `GET` | `/api/monitors/{id}/checks` | تاریخچه بررسی |
| `GET` | `/api/incidents` | رخدادها (`?active=1`) |
| `POST` | `/api/incidents/{id}/resolve` | بستن دستی رخداد |
| `GET/POST` | `/api/rules` | قوانین |
| `PUT/DELETE` | `/api/rules/{id}` | ویرایش/حذف قانون |
| `GET/POST` | `/api/agents` | سرورها |
| `PUT/DELETE` | `/api/agents/{id}` | ویرایش/حذف ایجنت |
| `POST` | `/api/agents/{id}/rotate` | توکن جدید |
| `GET` | `/api/agents/{id}/metrics` | نمودار منابع (`?hours=24`) |
| `GET` | `/api/agent/hello` | هندشیک REST ایجنت — هدر `X-Agent-Token` یا `Authorization: Bearer` |
| `POST` | `/api/agent/report` | گزارش ایجنت — REST/JSON با همان هدر توکن (fallback) |
| `GET` | `/api/agent/link` | ارتقای WebSocket برای stream زندهٔ ایجنت (fallback) |
| `POST` | `/pulse.agent.v1.AgentLink/Connect` | stream دوطرفهٔ gRPC ایجنت (JSON codec، هدر توکن در metadata) |
| `GET` | `/api/cloudflare/status` / `zones` / `records` | وضعیت و داده‌های Cloudflare |
| `POST` | `/api/cloudflare/update` | تغییر رکورد |
| `GET/PUT` | `/api/settings` | تنظیمات (توکن‌ها بازخوانی نمی‌شوند) |
| `POST` | `/api/settings/test/telegram` / `webhook` | تست کانال |
| `GET` | `/api/xray/status` | وضعیت و نسخهٔ هستهٔ Xray |
| `POST` | `/api/xray/detect` | شناسایی مجدد باینری Xray |
| `GET` | `/api/push/config` | کلید عمومی VAPID |
| `POST` | `/api/push/subscribe` / `unsubscribe` / `test` | مدیریت Web Push |
| `GET` | `/api/notifications` | سابقهٔ اعلان |
| `GET` | `/api/logs` | لاگ‌های ساختاریافته (`category`, `level`, `q`, `after`, `limit`) |
| `GET` | `/api/audit` | آخرین فعالیت‌ها |
| `GET` | `/api/backup` | خروجی JSON |
| `GET` | `/api/stream` | SSE رویدادهای زنده |

نمونه ساخت مانیتور:

```bash
curl -X POST http://127.0.0.1:8080/api/monitors \
  -H "Content-Type: application/json" \
  -H "Cookie: pulse_session=..." \
  -d '{
    "name": "سایت اصلی",
    "type": "http",
    "target": "https://example.com",
    "group": "عمومی",
    "interval_sec": 60,
    "timeout_ms": 12000,
    "probe_agent_id": 0,
    "enabled": true,
    "config": {"expected_status": "2xx", "follow_redirects": true},
    "fail_threshold": 2,
    "recover_threshold": 1
  }'
```

`probe_agent_id: 0` یعنی تست از خود پنل انجام شود؛ برای اجرای تست از یک ایجنت، شناسهٔ آن را از `/api/agents` بگذارید. اگر ایجنت آفلاین باشد، نتیجه به‌صورت `degraded` با خطای اتصال ثبت می‌شود.

---

## پشتیبان‌گیری

- **پشتیبان JSON:** تنظیمات → دانلود نسخهٔ پشتیبان. شامل مانیتورها، قوانین، ایجنت‌ها (بدون توکن)، رخدادها و تنظیمات غیرحساس است. توکن‌ها و رمزها در خروجی نیستند.
- **پشتیبان کامل:** فایل `pulse.db` (و در صورت وجود، `-wal` و `-shm`) را همراه با کلیدهای VAPID از volume کپی کنید. برای یک نسخهٔ سازگار، سرویس را متوقف یا از دستور `sqlite3 pulse.db ".backup backup.db"` استفاده کنید.
- پشتیبان JSON برای بازگردانی خودکار در نسخهٔ فعلی نیست؛ برای مهاجرت از همان دیتابیس استفاده کنید.

---

## امنیت

- پنل را فقط پشت HTTPS و ترجیحاً روی یک زیردامنه قرار دهید؛ پورت ۸۰۸۰ را عمومی نکنید.
- رمز مدیر قوی و منحصربه‌فرد انتخاب کنید.
- توکن Cloudflare را با حداقل دسترسی (`Zone Read` + `DNS Edit`) بسازید.
- توکن‌ها در SQLite ذخیره می‌شوند؛ دسترسی فایل دیتابیس را محدود کنید (`chmod 600`).
- توکن ایجنت فقط یک‌بار نمایش داده می‌شود و به‌صورت hash ذخیره می‌شود.
- نشست‌ها با HMAC امضا می‌شوند؛ تغییر رمز، نشست‌های قبلی را باطل نمی‌کند — برای باطل‌کردن، راز نشست را از دیتابیس (`settings.session_secret`) تغییر دهید و سرویس را ری‌استارت کنید.
- در Docker، کانتینر با root داخلی و `NET_RAW` اجرا می‌شود تا Ping کار کند؛ isolation را در سطح Docker/Caddy تنظیم کنید.
- تست واقعی Xray برای هر کانفیگ یک فایل موقت حاوی اطلاعات اتصال می‌سازد (مجوز 0600، پس از تست حذف می‌شود) و SOCKS موقت فقط روی `127.0.0.1` گوش می‌دهد.
- CSP، `X-Frame-Options: DENY`، `nosniff`، `Referrer-Policy`، HSTS (در HTTPS) و CSRF origin-check فعال‌اند.
- محدودیت ۸ تلاش ورود در ۱۰ دقیقه بر اساس IP.

---

## تنظیمات محیطی

| متغیر | پیش‌فرض | توضیح |
|---|---|---|
| `PULSE_ADDR` | `:8080` | آدرس شنود کامل (host:port) |
| `PULSE_PORT` | `8080` | پورت پنل؛ اگر `PULSE_ADDR` تنظیم نشده باشد استفاده می‌شود |
| `PULSE_DB_PATH` | `./data/pulse.db` | مسیر SQLite |
| `PULSE_TELEGRAM_TOKEN` | — | توکن ربات؛ بر تنظیمات پنل اولویت دارد |
| `PULSE_TELEGRAM_CHAT` | — | Chat ID |
| `PULSE_CF_TOKEN` | — | توکن Cloudflare |
| `PULSE_WEBHOOK_URL` | — | آدرس وب‌هوک |
| `PULSE_PUBLIC_BASE_URL` | — | آدرس عمومی اولیه؛ اگر تنظیمات پنل خالی باشد استفاده می‌شود |
| `PULSE_XRAY_BIN` | `xray` | مسیر باینری Xray؛ بر تنظیمات پنل اولویت دارد |

متغیرهای ایجنت:

| متغیر | پیش‌فرض | توضیح |
|---|---|---|
| `PULSE_SERVER` | — | آدرس عمومی پنل |
| `PULSE_TOKEN` | — | توکن ایجنت |
| `PULSE_AGENT_NAME` | hostname | نام نمایشی |
| `PULSE_AGENT_SERVICES` | — | سرویس‌های systemd با کاما جدا‌شده |
| `PULSE_AGENT_WG` | `true` | جمع‌آوری WireGuard |
| `PULSE_XRAY_BIN` | `xray` | مسیر باینری Xray برای تست کانفیگ از ایجنت |

فلگ‌های خط فرمان بر محیط اولویت دارند (`--addr`, `--db`).

متغیرهای ساخت Docker (اختیاری):

| متغیر | پیش‌فرض | توضیح |
|---|---|---|
| `PULSE_VERSION` | `1.6.1` | نسخهٔ تصویر |
| `XRAY_VERSION` | `v26.11.9` | نسخهٔ Xray داخل تصویر |
| `XRAY_DOWNLOAD_URL` | — | آدرس آینهٔ فایل ZIP برای دور زدن محدودیت GitHub |
| `GOPROXY` | `https://proxy.golang.org,direct` | آینهٔ ماژول‌های Go |

---

## توسعه و تست

```bash
# بک‌اند و تست‌ها
go test ./...
go vet ./...
go run ./cmd/pulse --addr 127.0.0.1:8080

# باینری‌های لینوکس (amd64 + arm64)
make linux
make release        # dist/ + SHA256SUMS

# بررسی سینتکس نصب‌کننده‌ها
make installer-check

# Docker
docker compose up -d --build
```

نیازمندی‌ها: Go 1.26+ (دیتابیس SQLite به‌صورت pure Go کامپایل می‌شود). برای UI نیازی به Node نیست. تست واقعی Xray به باینری `xray` نیاز دارد؛ نصب‌کننده یا Docker آن را نصب می‌کند.

برای اجرای تست یکپارچهٔ واقعی Xray (در صورت داشتن باینری):

```bash
XRAY_BIN=/usr/local/bin/xray go test ./internal/xray -run TestRealHandshake -v
```

### انتشار نسخه

با push یک تگ `v*`، workflow موجود در `.github/workflows/release.yml` باینری‌های لینوکس amd64/arm64 و فایل `SHA256SUMS` را می‌سازد و به GitHub Release اضافه می‌کند. نصب‌کننده همان فایل‌ها را روی سرور دانلود می‌کند:

```bash
git tag v1.6.1
git push origin v1.6.1
```

---

## عیب‌یابی

| مشکل | راه‌حل |
|---|---|
| نصب با خطای `Could not get lock /var/lib/dpkg/lock-frontend` متوقف می‌شود | نسخهٔ 1.6.1+ خودش تا ۱۰ دقیقه منتظر قفل می‌ماند و دوباره تلاش می‌کند. اگر باز هم تکرار شد: `sudo systemctl stop unattended-upgrades`، سپس `sudo dpkg --configure -a` و `sudo apt-get -f install` را اجرا و نصب را دوباره شروع کنید. |
| Ping خطای دسترسی می‌دهد | در Docker: `cap_add: NET_RAW`. در systemd: `AmbientCapabilities=CAP_NET_RAW` یا `sudo setcap cap_net_raw+ep $(which ping)`. |
| Web Push روی آیفون کار نمی‌کند | پنل باید HTTPS باشد و از صفحهٔ اصلی (PWA) باز شود، نه تب Safari. iOS 16.4+. |
| تلگرام پیام نمی‌فرستد | توکن را تست کنید؛ ربات را در کانال ادمین کنید؛ Chat ID را بررسی کنید. طبق قوانین تلگرام، ربات تا زمانی که کاربر به آن پیام نداده نمی‌تواند شروع کند. |
| Cloudflare خطای 403 می‌دهد | توکن باید `Zone:Read` و `DNS:Edit` داشته باشد و به Zone موردنظر دسترسی داشته باشد. |
| رکورد DNS تغییر نمی‌کند | مطمئن شوید رکورد در مانیتور انتخاب شده، IP اصلی/پشتیبان درست است و مقدار فعلی رکورد با IP اصلی یا پشتیبان هم‌خوان است. پالس برای محافظت، رکورد تغییر‌یافتهٔ دستی را بازنویسی نمی‌کند. |
| «database is locked» می‌بینید | پالس SQLite را با یک اتصال هم‌زمان اجرا می‌کند؛ این خطا در نسخه‌های فعلی نباید رخ دهد. فایل `.db-wal` را حذف نکنید و سرویس را ری‌استارت کنید. |
| اعلان مرورگر مسدود است | از تنظیمات سایت، Notification را Allow کنید و دوباره «فعال‌سازی» را بزنید. |
| فرمان نصب ایجنت آدرس اشتباه دارد | آدرس عمومی را در تنظیمات وارد کنید یا پنل را با Host صحیح باز کنید. |
| Caddy با خطای `address already in use` روی 443 متوقف می‌شود | با `ss -ltnp \| grep ':443'` ببینید چه سرویسی پورت را گرفته است (مثلاً Xray). اگر Xray باید روی 443 بماند، در Caddyfile گزینهٔ `https_port 8443` را اضافه کنید و پورت 8443 را باز کنید؛ وگرنه سرویس اشغال‌کننده را جابه‌جا/متوقف کنید. |
| Xray شناسایی نمی‌شود | با `xray version` از اجرای باینری مطمئن شوید، مسیر را در تنظیمات وارد کنید و «بررسی مجدد» را بزنید. در Docker، نسخهٔ Xray داخل تصویر نصب می‌شود. |
| تست واقعی پروکسی ناموفق است | UUID/رمز/Reality، SNI و شبکه را بررسی کنید. URL تست باید از سمت سرور پروکسی در دسترس باشد. اگر پنل بدون Xray اجرا شود، مانیتور به‌جای سبز کاذب، وضعیت هشدار می‌گیرد. |
| تست واقعی زمان‌بر است | تعداد «حداکثر کانفیگ برای تست واقعی» یا فاصلهٔ بررسی را کم/زیاد کنید؛ هر کانفیگ یک پروسهٔ Xray موقت دارد. |
| مانیتور با «اتصال زندهٔ ایجنت برقرار نیست» هشدار می‌دهد | در صفحهٔ سرورها transport ایجنت را ببینید؛ ایجنت باید نسخهٔ ۱.۵+ و آنلاین باشد. اگر REST است، gRPC/WebSocket در reverse proxy مسدود است؛ بلوک gRPC در Caddyfile یا WebSocket headers در Nginx را اضافه کنید. |
| «تست از ایجنت» انجام می‌شود ولی هندشیک Xray ندارد | روی ایجنت `xray version` را اجرا کنید و `PULSE_XRAY_BIN` را تنظیم کنید. نصب‌کنندهٔ جدید ایجنت Xray رسمی را نصب می‌کند؛ برای ایجنت قدیمی با همان دستور نصب دوباره اجرا کنید. |
| آمار زنده با تأخیر می‌رسد | ترافیک باید هر ۱ ثانیه باشد؛ transport کارت سرور را بررسی کنید. اگر REST است، ایجنت به fallback رفته و اتصال gRPC/WebSocket را برقرار کنید. |
| ساخت Docker در دانلود Xray خطا می‌دهد | `XRAY_DOWNLOAD_URL` را به یک آینهٔ معتبر از فایل ZIP همان معماری تنظیم کنید، یا پالس را بدون Docker روی هاستی با Xray نصب‌شده اجرا کنید. |
| نصب‌کننده ریلیز پیدا نمی‌کند | نصب‌کننده به‌صورت خودکار از سورس با Go رسمی می‌سازد. برای نصب سریع‌تر یک تگ بسازید یا از `--release`/`--binary`/`--binary-url` استفاده کنید. |
| دانلود از GitHub یا go.dev مسدود است | از `--binary-url` با یک میرور معتبر، `GOPROXY` مناسب، یا `XRAY_DOWNLOAD_URL` استفاده کنید. |
| Caddy نصب نمی‌شود | مخزن رسمی Caddy یا پورت‌های 80/443 را بررسی کنید. در حالت غیرتعاملی، نصب برای جلوگیری از HTTP عمومی متوقف می‌شود؛ با `--no-ssl` فقط در شبکهٔ خصوصی ادامه دهید. |
| صفحه سفید / خطای CSP | فایل‌های UI داخل باینری جاسازی شده‌اند؛ بعد از تغییر فرانت باید دوباره `go build` کنید. کش Service Worker را پاک کنید. |

---

## ساختار پروژه

```
install.sh              نصب‌کنندهٔ خودکار VPS (Pulse + Xray + Caddy/SSL)
cmd/pulse/              main (master + agent mode)
internal/
  agent/                ایجنت: جمع‌آوری CPU/RAM/Disk/سرویس/WireGuard
  app/                  HTTP API، scheduler، قوانین، اعلان‌ها، احراز هویت، SSE
  checker/              HTTP, TCP, Ping, DNS, SSL, Proxy
  cloudflare/           کلاینت Cloudflare API
  config/               تنظیمات و ذخیره‌سازی
  model/                مدل‌های داده
  notify/               تلگرام، Web Push، وب‌هوک
  store/                SQLite، migration و کوئری‌ها
  util/                 تاریخ شمسی و فرمت‌ها
  xray/                 موتور تست واقعی پروکسی با Xray-core (config + process + SOCKS)
web/                    SPA، CSS، فونت ایران‌سنس، PWA، Service Worker
deploy/                 نصب‌کنندهٔ ایجنت سرور
.github/workflows/      CI و ساخت GitHub Release برای لینوکس
```

پوشه‌های `bin/`، `dist/`، `data/` و فایل‌های `.db` محلی هستند و در `.gitignore` قرار دارند. هیچ باینری‌ای داخل ریپو نگه‌داری نمی‌شود؛ نصب‌کننده آن را از Release رسمی می‌گیرد.

---

## محدودیت‌های آگاهانه

- **یک حساب مدیر:** برای تیم‌های چندنفره، SSO/نقش‌ها در نسخهٔ بعدی قابل افزودن است.
- **هستهٔ Xray خارجی است:** برای تست واقعی باید باینری Xray نصب یا در Docker موجود باشد. اگر نباشد، پالس fallback دسترسی را انجام می‌دهد و هشدار می‌دهد.
- **Hysteria2/TUIC و پلاگین SS:** در این نسخهٔ پالس هندشیک واقعی Xray برای آن‌ها انجام نمی‌شود و فقط دسترسی بررسی می‌شود.
- **Clash YAML ساده:** فقط لیست سادهٔ کانفیگ‌ها خوانده می‌شود؛ برای تست واقعی از لینک‌های URI یا ساب‌سکریپشن Base64 استفاده کنید.
- **URL تست:** اگر خروجی تونل به اینترنت دسترسی نداشته باشد یا مقصد تست مسدود باشد، تست واقعی ناموفق می‌شود؛ آدرس تست را در فرم مانیتور تغییر دهید.
- **منابع تست واقعی:** هر کانفیگ یک پروسهٔ Xray موقت دارد؛ برای ساب‌سکریپشن‌های بزرگ تعداد کانفیگ‌ها و فاصلهٔ بررسی را تنظیم کنید.
- Cloudflare فقط رکوردهای A/AAAA را برای سوییچ خودکار پیشنهاد می‌کند.
- ساعات سکوت رویدادها را دور نمی‌ریزد؛ برای هشدار قطعی گزینهٔ عبور از سکوت فعال است.
- برای iOS، Web Push فقط بعد از نصب PWA فعال می‌شود.
- تاریخچهٔ بررسی‌ها و نمودار ایجنت طبق تنظیمات نگه‌داری پاک می‌شود (پیش‌فرض ۱۴ و ۳۰ روز).

---

## اعتبارها

- فونت **IRANSans / IRANSans FaNum** برای رابط فارسی استفاده شده است.
- طراحی و پیاده‌سازی: پروژهٔ Pulse.
- ساخته‌شده با Go، SQLite، Canvas و بدون فریم‌ورک فرانت‌اند.

اگر این پروژه را مفید دیدید، با دادن ستاره و ارسال بازخورد به بهتر شدنش کمک کنید. ❤️
