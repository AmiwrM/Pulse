package util

import (
	"fmt"
	"time"
)

// FaDigits is kept for compatibility with older formatting call sites. Pulse
// now uses Latin digits everywhere, including the Persian UI, so this is a
// no-op.
func FaDigits(s string) string {
	return s
}

// JalaliDate converts a Gregorian date to the Jalali calendar.
func JalaliDate(t time.Time) (int, int, int) {
	gy, gm, gd := t.Date()
	gDaysInMonth := [...]int{0, 31, 59, 90, 120, 151, 181, 212, 243, 273, 304, 334}
	gy2 := gy
	if gm > 2 {
		gy2++
	}
	days := 355666 + 365*gy + (gy2+3)/4 - (gy2+99)/100 + (gy2+399)/400 + gd + gDaysInMonth[gm-1]
	jy := -1595 + 33*(days/12053)
	days %= 12053
	jy += 4 * (days / 1461)
	days %= 1461
	if days > 365 {
		jy += (days - 1) / 365
		days = (days - 1) % 365
	}
	var jm, jd int
	if days < 186 {
		jm = 1 + days/31
		jd = 1 + days%31
	} else {
		jm = 7 + (days-186)/30
		jd = 1 + (days-186)%30
	}
	return jy, jm, jd
}

func two(n int) string { return fmt.Sprintf("%02d", n) }

// FormatDateTime formats a timestamp in Jalali with Latin digits.
func FormatDateTime(ts int64, loc *time.Location) string {
	if loc == nil {
		loc = time.UTC
	}
	if ts <= 0 {
		return "—"
	}
	t := time.Unix(ts, 0).In(loc)
	jy, jm, jd := JalaliDate(t)
	return FaDigits(fmt.Sprintf("%04d/%s/%s - %s:%s", jy, two(jm), two(jd), two(t.Hour()), two(t.Minute())))
}

func FormatDate(t time.Time) string {
	jy, jm, jd := JalaliDate(t)
	return FaDigits(fmt.Sprintf("%04d/%s/%s", jy, two(jm), two(jd)))
}

// HumanDuration renders a duration in a compact Persian form.
func HumanDuration(seconds int64) string {
	if seconds < 0 {
		seconds = 0
	}
	if seconds < 60 {
		return FaDigits(fmt.Sprintf("%d", seconds)) + " ثانیه"
	}
	minutes := seconds / 60
	if minutes < 60 {
		return FaDigits(fmt.Sprintf("%d", minutes)) + " دقیقه"
	}
	hours := minutes / 60
	minutes = minutes % 60
	if hours < 24 {
		if minutes == 0 {
			return FaDigits(fmt.Sprintf("%d", hours)) + " ساعت"
		}
		return FaDigits(fmt.Sprintf("%d ساعت و %d", hours, minutes)) + " دقیقه"
	}
	days := hours / 24
	hours = hours % 24
	if hours == 0 {
		return FaDigits(fmt.Sprintf("%d", days)) + " روز"
	}
	return FaDigits(fmt.Sprintf("%d روز و %d", days, hours)) + " ساعت"
}

func Percent(v float64) string {
	if v <= 0 {
		return "0٪"
	}
	return FaDigits(fmt.Sprintf("%.1f", v)) + "٪"
}
