package util

import (
	"strings"
	"testing"
	"time"
)

func TestJalaliDate(t *testing.T) {
	tm := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	jy, jm, jd := JalaliDate(tm)
	if jy != 1405 || jm != 8 || jd != 28 {
		t.Fatalf("got %d/%d/%d, want 1405/08/28", jy, jm, jd)
	}
	if out := FormatDateTime(tm.Unix(), time.UTC); !strings.Contains(out, "1405/08/28") {
		t.Errorf("unexpected formatted date: %s", out)
	}
}

func TestHumanDuration(t *testing.T) {
	if got := HumanDuration(45); !strings.Contains(got, "45 ثانیه") {
		t.Errorf("unexpected: %s", got)
	}
	if got := HumanDuration(3700); !strings.Contains(got, "1 ساعت") || !strings.Contains(got, "1 دقیقه") {
		t.Errorf("unexpected: %s", got)
	}
	if got := HumanDuration(90000); !strings.Contains(got, "1 روز") {
		t.Errorf("unexpected: %s", got)
	}
}
