package config

import (
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(env(map[string]string{
		"BIRTHDAY_NAME": "  Chihiro ",
		"BIRTHDAY_DATE": "07-20",
	}))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Name != "Chihiro" {
		t.Errorf("Name = %q, want trimmed %q", cfg.Name, "Chihiro")
	}
	if cfg.Message != DefaultMessage {
		t.Errorf("Message = %q, want default", cfg.Message)
	}
	if cfg.Port != "8080" || cfg.Host != "" {
		t.Errorf("listen = %q:%q, want :8080", cfg.Host, cfg.Port)
	}
	if !cfg.Halloween {
		t.Error("Halloween = false, want on by default")
	}
	if cfg.Secret != "" || cfg.SecretIntro != DefaultSecretIntro {
		t.Errorf("Secret = %q, SecretIntro = %q, want none and the default intro", cfg.Secret, cfg.SecretIntro)
	}
	if cfg.Teaser != DefaultTeaser {
		t.Errorf("Teaser = %q, want default", cfg.Teaser)
	}
	if cfg.OpenDays != DefaultOpenDays {
		t.Errorf("OpenDays = %d, want %d", cfg.OpenDays, DefaultOpenDays)
	}
	if cfg.Location != time.UTC {
		t.Errorf("Location = %v, want UTC", cfg.Location)
	}
	if cfg.Birthday.Month != time.July || cfg.Birthday.Day != 20 {
		t.Errorf("Birthday = %+v", cfg.Birthday)
	}
}

func TestLoadCustom(t *testing.T) {
	cfg, err := Load(env(map[string]string{
		"BIRTHDAY_NAME":       "Haku",
		"BIRTHDAY_DATE":       "1996-02-29",
		"BIRTHDAY_MESSAGE":    "Dinner is on me",
		"BIRTHDAY_FROM":       "Victor",
		"BIRTHDAY_TIMEZONE":   "Asia/Tokyo",
		"BIRTHDAY_PORT":       "9000",
		"BIRTHDAY_HALLOWEEN":  "false",
		"BIRTHDAY_OPEN_DAYS":  "3",
		"BIRTHDAY_TEASER":     "Guess what?",
		"BIRTHDAY_SECRET":     " UBER-1234-ABCD ",
		"BIRTHDAY_SECRET_PIN": " 482916 ",
	}))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Message != "Dinner is on me" || cfg.From != "Victor" || cfg.Port != "9000" {
		t.Errorf("cfg = %+v", cfg)
	}
	if cfg.Halloween {
		t.Error("Halloween = true, want false")
	}
	if cfg.SecretPIN != "482916" {
		t.Errorf("SecretPIN = %q, want it trimmed", cfg.SecretPIN)
	}
	if cfg.Secret != "UBER-1234-ABCD" {
		t.Errorf("Secret = %q, want it trimmed", cfg.Secret)
	}
	if cfg.Teaser != "Guess what?" {
		t.Errorf("Teaser = %q", cfg.Teaser)
	}
	if cfg.OpenDays != 3 {
		t.Errorf("OpenDays = %d, want 3", cfg.OpenDays)
	}
	if cfg.Location.String() != "Asia/Tokyo" {
		t.Errorf("Location = %v", cfg.Location)
	}
}

func TestLoadReportsEveryError(t *testing.T) {
	_, err := Load(env(map[string]string{
		"BIRTHDAY_DATE":       "20/07",
		"BIRTHDAY_TIMEZONE":   "Mars/Olympus",
		"BIRTHDAY_PORT":       "http",
		"BIRTHDAY_HALLOWEEN":  "maybe",
		"BIRTHDAY_OPEN_DAYS":  "-1",
		"BIRTHDAY_SECRET_PIN": "12",
		"BIRTHDAY_MESSAGE":    strings.Repeat("x", maxMessageLen+1),
	}))
	if err == nil {
		t.Fatal("Load() error = nil, want errors")
	}
	for _, want := range []string{"BIRTHDAY_NAME", "BIRTHDAY_DATE", "BIRTHDAY_TIMEZONE", "BIRTHDAY_PORT", "BIRTHDAY_MESSAGE", "BIRTHDAY_HALLOWEEN", "BIRTHDAY_OPEN_DAYS", "BIRTHDAY_SECRET_PIN"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}
