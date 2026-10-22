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

func TestLoadFallsBackWithoutNameAndDate(t *testing.T) {
	cfg, err := Load(env(map[string]string{}))
	if err != nil {
		t.Fatalf("Load() with no settings: error = %v, want defaults", err)
	}
	if cfg.Name != DefaultName {
		t.Errorf("Name = %q, want %q", cfg.Name, DefaultName)
	}
	if !cfg.DateDefaulted || cfg.Birthday.Month != time.January || cfg.Birthday.Day != 1 {
		t.Errorf("Birthday = %+v (defaulted %v), want January 1", cfg.Birthday, cfg.DateDefaulted)
	}
	if cfg.Message != DefaultMessage || cfg.Teaser != DefaultTeaser || cfg.SecretIntro != DefaultSecretIntro {
		t.Errorf("texts = %q / %q / %q, want the defaults", cfg.Message, cfg.Teaser, cfg.SecretIntro)
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
	for _, want := range []string{"BIRTHDAY_DATE", "BIRTHDAY_TIMEZONE", "BIRTHDAY_PORT", "BIRTHDAY_MESSAGE", "BIRTHDAY_HALLOWEEN", "BIRTHDAY_OPEN_DAYS", "BIRTHDAY_SECRET_PIN"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}

func TestLoadChat(t *testing.T) {
	cfg, err := Load(env(map[string]string{}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Chat || cfg.ChatName != DefaultChatName || cfg.ChatModel != DefaultChatModel || cfg.ChatCommand != DefaultChatCommand {
		t.Errorf("chat defaults: %+v", cfg)
	}

	cfg, err = Load(env(map[string]string{
		"BIRTHDAY_CHAT":        "true",
		"BIRTHDAY_CHAT_NAME":   " Pip ",
		"BIRTHDAY_CHAT_PROMPT": "Loves puns.",
		"BIRTHDAY_CHAT_MODEL":  "opus",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Chat || cfg.ChatName != "Pip" || cfg.ChatPrompt != "Loves puns." || cfg.ChatModel != "opus" {
		t.Errorf("chat settings: %+v", cfg)
	}

	_, err = Load(env(map[string]string{
		"BIRTHDAY_CHAT":        "maybe",
		"BIRTHDAY_CHAT_PROMPT": strings.Repeat("a", maxPromptLen+1),
		"BIRTHDAY_CHAT_MODEL":  "--dangerously-skip-permissions",
	}))
	for _, want := range []string{"BIRTHDAY_CHAT ", "BIRTHDAY_CHAT_PROMPT", "BIRTHDAY_CHAT_MODEL"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("error %v does not mention %s", err, want)
		}
	}
}

func TestLoadChatFallback(t *testing.T) {
	cfg, err := Load(env(map[string]string{}))
	if err != nil || len(cfg.ChatFallback) != len(DefaultChatFallback) {
		t.Fatalf("default fallback: %v, %q", err, cfg.ChatFallback)
	}
	cfg, err = Load(env(map[string]string{"BIRTHDAY_CHAT_FALLBACK": "  Joyeux anniversaire !\n\n  Bisous {name}  \n"}))
	if err != nil || len(cfg.ChatFallback) != 2 || cfg.ChatFallback[1] != "Bisous {name}" {
		t.Errorf("custom fallback: %v, %q", err, cfg.ChatFallback)
	}
	if _, err := Load(env(map[string]string{"BIRTHDAY_CHAT_FALLBACK": strings.Repeat("a", maxMessageLen+1)})); err == nil {
		t.Error("an over-long fallback line is accepted")
	}
}

func TestLoadChatHistoryDir(t *testing.T) {
	for _, c := range []struct {
		env  map[string]string
		want string
	}{
		{map[string]string{"CLAUDE_CONFIG_DIR": "/home/birthday/.claude"}, "/home/birthday/.claude/birthday-chat"},
		{map[string]string{"HOME": "/home/me"}, "/home/me/.claude/birthday-chat"},
		{map[string]string{"CLAUDE_CONFIG_DIR": "/c", "BIRTHDAY_CHAT_HISTORY": "false"}, ""},
	} {
		cfg, err := Load(env(c.env))
		if err != nil || cfg.ChatHistoryDir != c.want {
			t.Errorf("%v: ChatHistoryDir = %q, %v, want %q", c.env, cfg.ChatHistoryDir, err, c.want)
		}
	}
	if _, err := Load(env(map[string]string{"BIRTHDAY_CHAT_HISTORY": "maybe"})); err == nil {
		t.Error("BIRTHDAY_CHAT_HISTORY=maybe accepted")
	}
}

func TestLoadChatLockPath(t *testing.T) {
	cfg, err := Load(env(map[string]string{"CLAUDE_CONFIG_DIR": "/home/birthday/.claude", "BIRTHDAY_CHAT_HISTORY": "false"}))
	if err != nil || cfg.ChatLockPath != "/home/birthday/.claude/birthday-claude.lock" {
		t.Errorf("ChatLockPath = %q, %v", cfg.ChatLockPath, err)
	}
}

func TestLoadChatNotesPath(t *testing.T) {
	for _, c := range []struct {
		env  map[string]string
		want string
	}{
		{map[string]string{"CLAUDE_CONFIG_DIR": "/home/birthday/.claude"}, "/home/birthday/.claude/birthday-answers.md"},
		{map[string]string{"CLAUDE_CONFIG_DIR": "/c", "BIRTHDAY_CHAT_NOTES": "false"}, ""},
	} {
		cfg, err := Load(env(c.env))
		if err != nil || cfg.ChatNotesPath != c.want {
			t.Errorf("%v: ChatNotesPath = %q, %v, want %q", c.env, cfg.ChatNotesPath, err, c.want)
		}
	}
	if _, err := Load(env(map[string]string{"BIRTHDAY_CHAT_NOTES": "maybe"})); err == nil {
		t.Error("BIRTHDAY_CHAT_NOTES=maybe accepted")
	}
}

func TestLoadChatBeforePIN(t *testing.T) {
	cfg, err := Load(env(map[string]string{"CLAUDE_CONFIG_DIR": "/c"}))
	if err != nil || cfg.ChatBeforePIN != DefaultChatBeforePIN || cfg.ChatTalkedPath != "/c/birthday-talked" {
		t.Errorf("ChatBeforePIN = %d, ChatTalkedPath = %q, %v", cfg.ChatBeforePIN, cfg.ChatTalkedPath, err)
	}
	if cfg, err := Load(env(map[string]string{"BIRTHDAY_CHAT_BEFORE_PIN": "0"})); err != nil || cfg.ChatBeforePIN != 0 {
		t.Errorf("BIRTHDAY_CHAT_BEFORE_PIN=0: %d, %v", cfg.ChatBeforePIN, err)
	}
	for _, v := range []string{"-1", "51", "five"} {
		if _, err := Load(env(map[string]string{"BIRTHDAY_CHAT_BEFORE_PIN": v})); err == nil {
			t.Errorf("BIRTHDAY_CHAT_BEFORE_PIN=%s accepted", v)
		}
	}
}
