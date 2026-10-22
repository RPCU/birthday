package server

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Banh-Canh/birthday/internal/config"
)

func withChatBeforePIN(path string) func(*config.Config) {
	return func(c *config.Config) {
		withChatAndPIN(c)
		c.ChatBeforePIN = 3
		c.ChatTalkedPath = path
	}
}

func TestPINWaitsForTheChat(t *testing.T) {
	onTheDay := time.Date(2026, time.July, 20, 8, 0, 0, 0, time.UTC)
	talked := filepath.Join(t.TempDir(), "birthday-talked")
	s := newServer(t, onTheDay, "Chihiro", withChatBeforePIN(talked))
	s.friend = (&stubFriend{reply: []string{"Yay!"}}).run
	h := s.Handler()

	page := get(t, h, "/").Body.String()
	for _, want := range []string{
		`class="grow" data-left="3" data-total="3" data-friend="Sunny" hidden`,
		`<form class="extra-pin" novalidate data-then="chat">`,
		"Enter your PIN to chat with Sunny",
		"chat is-locked",
		"3 more messages",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q", want)
		}
	}

	// The chat is still behind the PIN.
	if rec := postChat(t, h, `{"messages":[{"from":"me","text":"hi"}]}`); rec.Code != http.StatusForbidden {
		t.Fatalf("chat without the PIN: status %d", rec.Code)
	}
	for i := range 3 {
		if rec := reveal(t, h, `{"pin":"482916"}`); rec.Code != http.StatusConflict {
			t.Fatalf("reveal after %d messages: status %d, want 409", i, rec.Code)
		}
		if rec := postChat(t, h, `{"pin":"482916","messages":[{"from":"me","text":"hi"}]}`); rec.Code != http.StatusOK {
			t.Fatalf("chat: status %d", rec.Code)
		}
	}
	if rec := reveal(t, h, `{"pin":"482916"}`); rec.Code != http.StatusOK {
		t.Errorf("reveal after 3 messages: status %d", rec.Code)
	}
	if page := get(t, h, "/").Body.String(); strings.Contains(page, `class="grow"`) || strings.Contains(page, "data-then") {
		t.Error("the pumpkin is still in the way")
	}

	// Another replica (or a restart) reads the count off the volume.
	other := newServer(t, onTheDay, "Chihiro", withChatBeforePIN(talked))
	if left := other.messagesLeft(); left != 0 {
		t.Errorf("another server has %d messages left", left)
	}
}

func TestNoPumpkinWithoutTheSecret(t *testing.T) {
	s := newServer(t, time.Date(2026, time.July, 20, 8, 0, 0, 0, time.UTC), "Chihiro", func(c *config.Config) {
		withChatBeforePIN("")(c)
		c.Secret = ""
	})
	if s.pinAfterChat() || strings.Contains(get(t, s.Handler(), "/").Body.String(), `class="grow"`) {
		t.Error("a pumpkin grows with nothing to unlock")
	}
}

func TestFriendKnowsAboutTheGame(t *testing.T) {
	s := newServer(t, time.Date(2026, time.July, 20, 8, 0, 0, 0, time.UTC), "Chihiro", withChatBeforePIN(""))
	if p := s.friendSystemPrompt(); !strings.Contains(p, "a little game") || strings.Contains(p, testPIN) || strings.Contains(p, testCode) {
		t.Errorf("system prompt: %s", p)
	}
}
