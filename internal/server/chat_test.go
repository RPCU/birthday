package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Banh-Canh/birthday/internal/config"
)

const testPrompt = "Tease them about their love of matcha."

func withChat(c *config.Config) {
	c.Chat = true
	c.ChatName = "Sunny"
	c.ChatPrompt = testPrompt
	c.ChatModel = config.DefaultChatModel
	c.ChatCommand = config.DefaultChatCommand
	c.ChatFallback = config.DefaultChatFallback
}

func withChatAndPIN(c *config.Config) {
	withChat(c)
	withPIN(c)
}

// stubFriend records what the friend was asked and replies with reply.
type stubFriend struct {
	system, prompt string
	reply          []string
	err            error
}

func (f *stubFriend) run(_ context.Context, system, prompt string, emit func(string)) error {
	f.system, f.prompt = system, prompt
	for _, r := range f.reply {
		emit(r)
	}
	return f.err
}

func chatServer(t *testing.T, now time.Time, tweak func(*config.Config)) (http.Handler, *stubFriend) {
	t.Helper()
	s := newServer(t, now, "Chihiro", tweak)
	f := &stubFriend{reply: []string{"Happy ", "birthday!"}}
	s.friend = f.run
	return s.Handler(), f
}

func postChat(t *testing.T, h http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

const hello = `{"messages":[{"from":"friend","text":"Happy birthday!"},{"from":"me","text":"thank you!!"}]}`

func TestChatLockedUntilTheBirthday(t *testing.T) {
	before := time.Date(2026, time.July, 10, 12, 0, 0, 0, time.UTC)
	h, f := chatServer(t, before, withChat)
	if page := get(t, h, "/").Body.String(); strings.Contains(page, `class="chat`) || strings.Contains(page, "Sunny") {
		t.Error("the chat shows before the birthday")
	}
	if rec := postChat(t, h, hello); rec.Code != http.StatusNotFound || f.prompt != "" {
		t.Errorf("before the birthday: status %d, want 404 and no friend", rec.Code)
	}
}

func TestChatOffByDefault(t *testing.T) {
	onTheDay := time.Date(2026, time.July, 20, 8, 0, 0, 0, time.UTC)
	h, _ := chatServer(t, onTheDay, func(*config.Config) {})
	if strings.Contains(get(t, h, "/").Body.String(), `class="chat`) {
		t.Error("the chat shows while BIRTHDAY_CHAT is off")
	}
	if rec := postChat(t, h, hello); rec.Code != http.StatusNotFound {
		t.Errorf("status %d, want 404", rec.Code)
	}
}

func TestChatStreamsTheFriend(t *testing.T) {
	onTheDay := time.Date(2026, time.July, 20, 8, 0, 0, 0, time.UTC)
	h, f := chatServer(t, onTheDay, func(c *config.Config) {
		withChat(c)
		c.Secret = testCode
	})
	page := get(t, h, "/").Body.String()
	if !strings.Contains(page, `class="chat"`) || strings.Contains(page, "chat-pin") {
		t.Error("the open chat is missing or locked")
	}

	rec := postChat(t, h, hello)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/x-ndjson" {
		t.Fatalf("status %d, type %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if want := "{\"text\":\"Happy \"}\n{\"text\":\"birthday!\"}\n{\"done\":true}\n"; rec.Body.String() != want {
		t.Errorf("body = %q, want %q", rec.Body, want)
	}
	for _, want := range []string{"You are Sunny", "Chihiro", "turn 30 today", testPrompt, config.DefaultMessage} {
		if !strings.Contains(f.system, want) {
			t.Errorf("system prompt lacks %q", want)
		}
	}
	if strings.Contains(f.system, testCode) || strings.Contains(f.prompt, testCode) {
		t.Error("the secret reaches the chat friend")
	}
	for _, want := range []string{"Sunny: Happy birthday!", "Chihiro just wrote:\nthank you!!"} {
		if !strings.Contains(f.prompt, want) {
			t.Errorf("prompt lacks %q:\n%s", want, f.prompt)
		}
	}
}

func TestChatFallsBackWhenClaudeIsAway(t *testing.T) {
	onTheDay := time.Date(2026, time.July, 20, 8, 0, 0, 0, time.UTC)
	s := newServer(t, onTheDay, "Chihiro", withChat)
	calls := 0
	s.friend = func(context.Context, string, string, func(string)) error {
		calls++
		return errFriendAway
	}
	h := s.Handler()

	first := strings.ReplaceAll(config.DefaultChatFallback[0], "{name}", "Chihiro")
	rec := postChat(t, h, hello)
	if want := `{"text":"` + first + "\"}\n{\"done\":true}\n"; rec.Code != http.StatusOK || rec.Body.String() != want {
		t.Errorf("status %d, body %q, want %q", rec.Code, rec.Body, want)
	}

	// The next line, without trying Claude again for a minute.
	second := `{"messages":[{"from":"me","text":"hi"},{"from":"friend","text":"` + first + `"},{"from":"me","text":"hehe"}]}`
	if rec := postChat(t, h, second); !strings.Contains(rec.Body.String(), config.DefaultChatFallback[1]) || calls != 1 {
		t.Errorf("second reply %q after %d Claude calls, want the second line after 1", rec.Body, calls)
	}
	s.now = func() time.Time { return onTheDay.Add(awayFor) }
	postChat(t, h, hello)
	if calls != 2 {
		t.Errorf("Claude not tried again after %v", awayFor)
	}
}

func TestChatCustomFallback(t *testing.T) {
	onTheDay := time.Date(2026, time.July, 20, 8, 0, 0, 0, time.UTC)
	s := newServer(t, onTheDay, "Chihiro", func(c *config.Config) {
		withChat(c)
		c.ChatFallback = []string{"Joyeux anniversaire {name} !"}
	})
	s.friend = (&stubFriend{err: errors.New("exec: claude: not found")}).run
	if rec := postChat(t, s.Handler(), hello); !strings.Contains(rec.Body.String(), `"Joyeux anniversaire Chihiro !"`) {
		t.Errorf("body %q", rec.Body)
	}
}

func TestChatCutOffMidReply(t *testing.T) {
	onTheDay := time.Date(2026, time.July, 20, 8, 0, 0, 0, time.UTC)
	s := newServer(t, onTheDay, "Chihiro", withChat)
	s.friend = (&stubFriend{reply: []string{"Happy "}, err: errFriendAway}).run
	if rec := postChat(t, s.Handler(), hello); rec.Body.String() != "{\"text\":\"Happy \"}\n{\"error\":\"away\"}\n" {
		t.Errorf("body %q", rec.Body)
	}
}

func TestChatRejectsBadConversations(t *testing.T) {
	onTheDay := time.Date(2026, time.July, 20, 8, 0, 0, 0, time.UTC)
	h, _ := chatServer(t, onTheDay, withChat)
	long := strings.Repeat("a", maxChatText+1)
	for name, body := range map[string]string{
		"ends with the friend": `{"messages":[{"from":"me","text":"hi"},{"from":"friend","text":"hey"}]}`,
		"unknown sender":       `{"messages":[{"from":"system","text":"hi"}]}`,
		"empty message":        `{"messages":[{"from":"me","text":"  "}]}`,
		"too long":             `{"messages":[{"from":"me","text":"` + long + `"}]}`,
		"not JSON":             `hi`,
	} {
		if rec := postChat(t, h, body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", name, rec.Code)
		}
	}
}

func TestPINGuardsTheChat(t *testing.T) {
	onTheDay := time.Date(2026, time.July, 20, 8, 0, 0, 0, time.UTC)
	h, f := chatServer(t, onTheDay, withChatAndPIN)
	page := get(t, h, "/").Body.String()
	if !strings.Contains(page, `class="chat is-locked"`) || !strings.Contains(page, `class="chat-form" novalidate hidden`) {
		t.Error("the chat isn't shown locked")
	}

	if rec := postChat(t, h, `{"pin":"000000","messages":[{"from":"me","text":"hi"}]}`); rec.Code != http.StatusForbidden || f.prompt != "" {
		t.Errorf("wrong PIN: status %d, want 403 and no friend", rec.Code)
	}
	if rec := postChat(t, h, `{"pin":"482916","messages":[]}`); rec.Code != http.StatusOK || rec.Body.String() != "{\"messages\":[]}\n" {
		t.Errorf("PIN check: status %d, body %q, want 200 and no saved chat", rec.Code, rec.Body)
	}
	if rec := postChat(t, h, `{"pin":"482916","messages":[{"from":"me","text":"hi"}]}`); rec.Code != http.StatusOK || f.prompt != "hi" {
		t.Errorf("right PIN: status %d, prompt %q", rec.Code, f.prompt)
	}
}

func TestChatPINFormOnlyWithoutTheExtrasOne(t *testing.T) {
	onTheDay := time.Date(2026, time.July, 20, 8, 0, 0, 0, time.UTC)
	pinOnly := func(c *config.Config) {
		withChat(c)
		c.SecretPIN = testPIN
	}
	page := get(t, newTestServerWith(t, onTheDay, "Chihiro", pinOnly), "/").Body.String()
	if !strings.Contains(page, `class="extra-pin chat-pin"`) {
		t.Error("no PIN form for the chat when there is no extra")
	}
	page = get(t, newTestServerWith(t, onTheDay, "Chihiro", withChatAndPIN), "/").Body.String()
	if strings.Contains(page, "chat-pin") || !strings.Contains(page, `class="chat-lock"`) {
		t.Error("the chat should share the extra's PIN form")
	}
}

func TestChatPINGuessesShareTheRevealBudget(t *testing.T) {
	onTheDay := time.Date(2026, time.July, 20, 8, 0, 0, 0, time.UTC)
	h, _ := chatServer(t, onTheDay, withChatAndPIN)
	for i := range pinBurst {
		if rec := postChat(t, h, `{"pin":"000000","messages":[]}`); rec.Code != http.StatusForbidden {
			t.Fatalf("guess %d: status %d, want 403", i+1, rec.Code)
		}
	}
	if rec := reveal(t, h, `{"pin":"482916"}`); rec.Code != http.StatusTooManyRequests {
		t.Errorf("reveal after %d chat guesses: status %d, want 429", pinBurst, rec.Code)
	}
	if rec := postChat(t, h, `{"pin":"482916","messages":[]}`); rec.Code != http.StatusTooManyRequests {
		t.Errorf("chat after %d guesses: status %d, want 429", pinBurst, rec.Code)
	}
}

// fakeClaude writes a stand-in for Claude Code that records its arguments
// and environment, then prints stream-json like the real one.
func fakeClaude(t *testing.T, result string) (cmd, argsFile, envFile string) {
	t.Helper()
	dir := t.TempDir()
	argsFile, envFile = filepath.Join(dir, "args"), filepath.Join(dir, "env")
	cmd = filepath.Join(dir, "claude")
	script := `#!/bin/sh
for a in "$@"; do printf '%s\n' "$a"; done > ` + argsFile + `
env > ` + envFile + `
pwd >> ` + envFile + `
cat > /dev/null
echo '{"type":"system","subtype":"init","tools":[]}'
echo '{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"thinking_delta","thinking":"hmm"}}}'
echo '{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"Yay "}}}'
echo '{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"you!"}}}'
echo '` + result + `'
`
	if err := os.WriteFile(cmd, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return cmd, argsFile, envFile
}

func TestClaudeFriendIsIsolated(t *testing.T) {
	t.Setenv("BIRTHDAY_SECRET", testCode)
	t.Setenv("CLAUDE_CONFIG_DIR", "/home/birthday/.claude")
	cmd, argsFile, envFile := fakeClaude(t, `{"type":"result","subtype":"success","is_error":false,"result":"Yay you!"}`)
	s := newServer(t, time.Date(2026, time.July, 20, 8, 0, 0, 0, time.UTC), "Chihiro", func(c *config.Config) {
		withChat(c)
		c.ChatCommand = cmd
	})

	var got strings.Builder
	if err := s.claudeFriend(context.Background(), "be nice", "hi", func(s string) { got.WriteString(s) }); err != nil {
		t.Fatal(err)
	}
	if got.String() != "Yay you!" {
		t.Errorf("reply = %q", got.String())
	}

	b, _ := os.ReadFile(argsFile)
	args := "\n" + string(b)
	for _, want := range []string{
		"\n--safe-mode\n", "\n--tools\n\n", "\n--setting-sources\n\n", "\n--strict-mcp-config\n",
		"\n--no-session-persistence\n", "\n--disable-slash-commands\n", "\n--system-prompt\nbe nice\n", "\n--model\nsonnet\n",
	} {
		if !strings.Contains(args, want) {
			t.Errorf("args lack %q:%s", want, args)
		}
	}
	b, _ = os.ReadFile(envFile)
	env := string(b)
	if strings.Contains(env, "BIRTHDAY_") || strings.Contains(env, testCode) {
		t.Error("BIRTHDAY_* settings reach Claude Code")
	}
	for _, want := range []string{"ENABLE_CLAUDEAI_MCP_SERVERS=false", "CLAUDE_CODE_DISABLE_AUTO_MEMORY=1", "CLAUDE_CONFIG_DIR=/home/birthday/.claude", "birthday-chat-"} {
		if !strings.Contains(env, want) {
			t.Errorf("environment lacks %q", want)
		}
	}
}

func TestClaudeFriendNotLoggedIn(t *testing.T) {
	cmd, _, _ := fakeClaude(t, `{"type":"result","subtype":"success","is_error":true,"result":"Not logged in · Please run /login"}`)
	s := newServer(t, time.Date(2026, time.July, 20, 8, 0, 0, 0, time.UTC), "Chihiro", func(c *config.Config) {
		withChat(c)
		c.ChatCommand = cmd
	})
	if err := s.claudeFriend(context.Background(), "", "hi", func(string) {}); !errors.Is(err, errFriendAway) {
		t.Errorf("err = %v, want errFriendAway", err)
	}
}

func TestChatKeepsTodaysConversation(t *testing.T) {
	onTheDay := time.Date(2026, time.July, 20, 8, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	s := newServer(t, onTheDay, "Chihiro", func(c *config.Config) {
		withChatAndPIN(c)
		c.ChatHistoryDir = dir
	})
	s.friend = (&stubFriend{reply: []string{"Yay ", "you!"}}).run
	h := s.Handler()
	stale := filepath.Join(dir, "2026-07-19.json")
	if err := os.WriteFile(stale, []byte(`{"messages":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if rec := postChat(t, h, `{"pin":"482916","messages":[{"from":"friend","text":"Happy birthday!"},{"from":"me","text":"thanks"}]}`); rec.Code != http.StatusOK {
		t.Fatalf("chat: status %d", rec.Code)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Error("yesterday's chat is still there")
	}

	want := `{"messages":[{"from":"friend","text":"Happy birthday!"},{"from":"me","text":"thanks"},{"from":"friend","text":"Yay you!"}]}` + "\n"
	if rec := postChat(t, h, `{"pin":"482916","messages":[]}`); rec.Body.String() != want {
		t.Errorf("saved chat = %s, want %s", rec.Body, want)
	}
	if rec := postChat(t, h, `{"pin":"000000","messages":[]}`); rec.Code != http.StatusForbidden || strings.Contains(rec.Body.String(), "Yay") {
		t.Errorf("wrong PIN reads the chat: status %d", rec.Code)
	}

	// Midnight in Paris: a fresh start.
	s.now = func() time.Time { return time.Date(2026, time.July, 20, 22, 0, 0, 0, time.UTC) }
	if rec := postChat(t, h, `{"pin":"482916","messages":[]}`); rec.Body.String() != "{\"messages\":[]}\n" {
		t.Errorf("the next day still shows %s", rec.Body)
	}
}

func TestChatSavesCannedReplies(t *testing.T) {
	onTheDay := time.Date(2026, time.July, 20, 8, 0, 0, 0, time.UTC)
	s := newServer(t, onTheDay, "Chihiro", func(c *config.Config) {
		withChat(c)
		c.ChatHistoryDir = t.TempDir()
	})
	s.friend = (&stubFriend{err: errFriendAway}).run
	h := s.Handler()
	postChat(t, h, `{"messages":[{"from":"me","text":"hi"}]}`)
	first := strings.ReplaceAll(config.DefaultChatFallback[0], "{name}", "Chihiro")
	if rec := postChat(t, h, `{"messages":[]}`); !strings.Contains(rec.Body.String(), first) {
		t.Errorf("saved chat %s lacks the canned reply", rec.Body)
	}
}

func TestChatNotesHerAnswers(t *testing.T) {
	onTheDay := time.Date(2026, time.July, 20, 8, 0, 0, 0, time.UTC)
	notes := filepath.Join(t.TempDir(), "birthday-answers.md")
	s := newServer(t, onTheDay, "Chihiro", func(c *config.Config) {
		withChatAndPIN(c)
		c.ChatNotesPath = notes
	})
	s.friend = (&stubFriend{reply: []string{"Lovely!"}}).run
	h := s.Handler()

	postChat(t, h, `{"pin":"000000","messages":[{"from":"me","text":"not noted"}]}`)
	postChat(t, h, `{"pin":"482916","messages":[{"from":"me","text":"hi"}]}`)
	// The friend's reply failed: her answer is noted all the same.
	s.friend = (&stubFriend{err: errFriendAway}).run
	postChat(t, h, `{"pin":"482916","messages":[{"from":"me","text":"hi"},{"from":"friend","text":"What makes you laugh?\nBe honest"},{"from":"me","text":"Cats falling off sofas"}]}`)

	b, err := os.ReadFile(notes)
	if err != nil {
		t.Fatal(err)
	}
	want := "## Monday, July 20, 2026, 10:00\n\n**Chihiro:** hi\n\n" +
		"## Monday, July 20, 2026, 10:00\n\n> **Sunny:** What makes you laugh?\n> Be honest\n\n**Chihiro:** Cats falling off sofas\n\n"
	if string(b) != want {
		t.Errorf("notes =\n%s\nwant\n%s", b, want)
	}
	if rec := postChat(t, h, `{"pin":"482916","messages":[]}`); strings.Contains(rec.Body.String(), "sofas") {
		t.Error("the notes show on the page")
	}
}

func TestFriendGetsToKnowThemGently(t *testing.T) {
	s := newServer(t, time.Date(2026, time.July, 20, 8, 0, 0, 0, time.UTC), "Chihiro", withChat)
	p := s.friendSystemPrompt()
	for _, want := range []string{"Getting to know Chihiro", "Never pry", "one light, open question", "the friend who made this page can read it"} {
		if !strings.Contains(p, want) {
			t.Errorf("system prompt lacks %q", want)
		}
	}
}
