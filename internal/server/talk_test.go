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
		c.ChatCountReset = true
		c.SecretIntro = "And a ride to get there"
	}
}

func TestCountCanStayReached(t *testing.T) {
	onTheDay := time.Date(2026, time.July, 20, 8, 0, 0, 0, time.UTC)
	s := newServer(t, onTheDay, "Chihiro", func(c *config.Config) {
		withChatBeforePIN(filepath.Join(t.TempDir(), "birthday-talked"))(c)
		c.ChatCountReset = false
	})
	s.friend = (&stubFriend{reply: []string{"Yay!"}}).run
	h := s.Handler()
	for range 3 {
		postChat(t, h, `{"pin":"482916","messages":[{"from":"me","text":"hi"}]}`)
	}
	postChat(t, h, `{"pin":"482916","messages":[]}`)
	if rec := reveal(t, h, `{"pin":"482916"}`); rec.Code != http.StatusOK {
		t.Errorf("reveal on the next visit: status %d", rec.Code)
	}
	if page := get(t, h, "/").Body.String(); strings.Contains(page, `class="grow"`) {
		t.Error("the pumpkin comes back")
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
	if strings.Contains(page, "a ride") {
		t.Error("the intro shows before the pumpkin is ripe")
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
	if rec := reveal(t, h, `{"pin":"482916"}`); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"intro":"And a ride to get there"`) {
		t.Errorf("reveal after 3 messages: status %d, %s", rec.Code, rec.Body)
	}
	// The reveal spent the count, on every replica sharing the volume.
	if rec := reveal(t, h, `{"pin":"482916"}`); rec.Code != http.StatusConflict {
		t.Errorf("a second reveal: status %d, want 409", rec.Code)
	}
	other := newServer(t, onTheDay, "Chihiro", withChatBeforePIN(talked))
	if left := other.messagesLeft(); left != 3 {
		t.Errorf("another server has %d messages left after the reveal, want 3", left)
	}

	// Another replica (or a restart) reads the count off the volume.
	postChat(t, h, `{"pin":"482916","messages":[{"from":"me","text":"hi"}]}`)
	if left := other.messagesLeft(); left != 2 {
		t.Errorf("another server has %d messages left, want 2", left)
	}

	// The next visit: the pumpkin again, and the PIN starts the count over.
	if page := get(t, h, "/").Body.String(); !strings.Contains(page, `data-left="3"`) {
		t.Error("the next visit skips the pumpkin")
	}
	if rec := postChat(t, h, `{"pin":"482916","messages":[]}`); rec.Code != http.StatusOK {
		t.Fatalf("unlock: status %d", rec.Code)
	}
	if left := other.messagesLeft(); left != 3 {
		t.Errorf("another server has %d messages left after the PIN, want 3", left)
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
