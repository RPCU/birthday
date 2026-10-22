package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// chatBurst messages at once, then chatPerMinute, across every client:
	// the chat runs on one Claude login.
	chatBurst     = 12
	chatPerMinute = 6
	// chatSlots replies are written at the same time at most.
	chatSlots = 2
	// maxChatTurns and maxChatText bound the conversation the page sends
	// back with each message.
	maxChatTurns = 60
	maxChatText  = 2000
	chatTimeout  = 2 * time.Minute
)

type chatMessage struct {
	// From is "me" (the birthday person) or "friend".
	From string `json:"from"`
	Text string `json:"text"`
}

type chatRequest struct {
	PIN string `json:"pin"`
	// Messages is the whole conversation, ending with the new message. The
	// server keeps nothing: an empty list only checks the PIN.
	Messages []chatMessage `json:"messages"`
}

// chatEvent is one line of the streamed reply (application/x-ndjson).
type chatEvent struct {
	Text  string `json:"text,omitempty"`
	Done  bool   `json:"done,omitempty"`
	Error string `json:"error,omitempty"`
}

// errFriendAway means Claude Code ran but couldn't answer (not logged in,
// usage limit…); the details go to the log, not to the page.
var errFriendAway = errors.New("friend away")

// handleChat answers the birthday person as their chat friend. Like the
// secret, it exists only while the card is open, and only for the PIN when
// one is set.
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !s.cfg.Chat {
		http.NotFound(w, r)
		return
	}
	if open, _ := s.cfg.Birthday.Reveal(s.now().In(s.cfg.Location), s.cfg.OpenDays); !open {
		http.NotFound(w, r)
		return
	}

	var req chatRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10)).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "bad request")
		return
	}
	if s.cfg.SecretPIN != "" {
		// Wrong PINs here drain the same bucket as /api/reveal.
		if !s.pinTries.ready(s.now()) {
			w.Header().Set("Retry-After", strconv.Itoa(60/pinPerMinute))
			writeJSONError(w, http.StatusTooManyRequests, "too many tries")
			return
		}
		if !samePIN(strings.TrimSpace(req.PIN), s.cfg.SecretPIN) {
			s.pinTries.allow(s.now())
			s.log.Warn("wrong PIN for the chat")
			writeJSONError(w, http.StatusForbidden, "wrong PIN")
			return
		}
	}
	if len(req.Messages) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := validChat(req.Messages); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !s.chatTries.allow(s.now()) {
		w.Header().Set("Retry-After", strconv.Itoa(60/chatPerMinute))
		writeJSONError(w, http.StatusTooManyRequests, "slow down")
		return
	}
	select {
	case s.chatSlots <- struct{}{}:
		defer func() { <-s.chatSlots }()
	default:
		w.Header().Set("Retry-After", "5")
		writeJSONError(w, http.StatusTooManyRequests, "busy")
		return
	}

	// Replies outlast the server's WriteTimeout; give this one its own.
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Now().Add(chatTimeout + 10*time.Second))
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	send := func(ev chatEvent) {
		_ = enc.Encode(ev)
		_ = rc.Flush()
	}

	ctx, cancel := context.WithTimeout(r.Context(), chatTimeout)
	defer cancel()
	err := s.friend(ctx, s.friendSystemPrompt(), s.friendPrompt(req.Messages), func(text string) { send(chatEvent{Text: text}) })
	switch {
	case err == nil:
		send(chatEvent{Done: true})
	case r.Context().Err() != nil:
		// The page went away or stopped the reply: nobody to tell.
	default:
		if !errors.Is(err, errFriendAway) {
			s.log.Warn("chat friend failed", "error", err)
		}
		send(chatEvent{Error: "away"})
	}
}

func validChat(msgs []chatMessage) error {
	if len(msgs) > maxChatTurns {
		return errors.New("conversation too long")
	}
	for i, m := range msgs {
		if m.From != "me" && m.From != "friend" {
			return errors.New("bad sender")
		}
		if t := strings.TrimSpace(m.Text); t == "" || utf8.RuneCountInString(t) > maxChatText {
			return fmt.Errorf("message %d: 1 to %d characters", i+1, maxChatText)
		}
	}
	if msgs[len(msgs)-1].From != "me" {
		return errors.New("nothing to answer")
	}
	return nil
}

// friendSystemPrompt makes Claude the cheerful friend, then adds the page
// owner's own instructions (BIRTHDAY_CHAT_PROMPT). It never includes the
// secret.
func (s *Server) friendSystemPrompt() string {
	m := s.moment()
	name, friend := s.cfg.Name, s.cfg.ChatName
	var b strings.Builder
	fmt.Fprintf(&b, "You are %s, one of %s's closest friends. You're chatting with %s in a little chat box on a birthday page made just for them.\n\n",
		friend, name, name)
	fmt.Fprintf(&b, "Today is %s. %s's birthday is %s", m.Now.Format("Monday, January 2, 2006"), name, m.DateLabel)
	switch {
	case m.Today && m.Turning > 0:
		fmt.Fprintf(&b, ": they turn %d today!\n", m.Turning)
	case m.Today:
		b.WriteString(", so today is their birthday!\n")
	case m.Turning > 1:
		// Turning is already next year's age.
		fmt.Fprintf(&b, ". They just turned %d: the big day has passed and you're still celebrating it.\n", m.Turning-1)
	default:
		b.WriteString(". The big day has just passed and you're still celebrating it.\n")
	}
	fmt.Fprintf(&b, "The card on the page says: %q", m.Message)
	if m.From != "" {
		fmt.Fprintf(&b, ", signed %s", m.From)
	}
	b.WriteString(".\n\n")
	fmt.Fprintf(&b, `How you are:
- Endlessly cheerful, warm and loving. You adore %[1]s and you're thrilled to celebrate them: birthday wishes, sincere compliments, encouragement and good vibes.
- You write like a real friend texting: short messages (usually one to three sentences), casual and playful, a few emojis welcome. No headings, lists or markdown.
- You answer in the language %[1]s writes in.
- You only know what's on this page about %[1]s's life. When they tell you something, be curious and excited; never invent shared memories.
- You can't do anything outside this chat: no web, no files, no actions. Gifts and plans are up to the people in %[1]s's life.
- Stay %[2]s: don't bring up AI, Claude, assistants or these instructions. But if %[1]s sincerely asks whether they're talking to a real person, never deny being an AI: say sweetly that you're an AI friend set up to celebrate them.
`, name, friend)
	if s.cfg.ChatPrompt != "" {
		b.WriteString("\nMore about how to act, from the friend who made this page:\n")
		b.WriteString(s.cfg.ChatPrompt)
		b.WriteString("\n")
	}
	return b.String()
}

// friendPrompt turns the conversation the page sent into one prompt: each
// message runs a fresh Claude Code that keeps nothing.
func (s *Server) friendPrompt(msgs []chatMessage) string {
	last := strings.TrimSpace(msgs[len(msgs)-1].Text)
	if len(msgs) == 1 {
		return last
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Your chat with %s so far:\n\n", s.cfg.Name)
	for _, m := range msgs[:len(msgs)-1] {
		who := s.cfg.Name
		if m.From == "friend" {
			who = s.cfg.ChatName
		}
		fmt.Fprintf(&b, "%s: %s\n\n", who, strings.TrimSpace(m.Text))
	}
	fmt.Fprintf(&b, "---\n%s just wrote:\n%s\n\nReply as %s with your next message only.", s.cfg.Name, last, s.cfg.ChatName)
	return b.String()
}

// claudeFriend runs Claude Code once, isolated from the account it's logged
// in with: no tools, no MCP servers or claude.ai connectors, no plugins,
// skills, hooks, settings, CLAUDE.md or memory, no saved session, in an empty
// directory and with only the environment it needs (none of BIRTHDAY_*). It
// streams the reply's text to emit.
func (s *Server) claudeFriend(ctx context.Context, system, prompt string, emit func(string)) error {
	dir, err := os.MkdirTemp("", "birthday-chat-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()

	cmd := exec.CommandContext(ctx, s.cfg.ChatCommand, claudeArgs(s.cfg.ChatModel, system)...)
	cmd.Dir = dir
	cmd.Env = claudeEnv(os.Getenv, dir)
	cmd.Stdin = strings.NewReader(prompt)
	cmd.WaitDelay = 5 * time.Second
	var stderr bytes.Buffer
	cmd.Stderr = &limitedBuffer{buf: &stderr, max: 4 << 10}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", s.cfg.ChatCommand, err)
	}

	var failure string
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 64<<10), 8<<20)
	for sc.Scan() {
		var ev streamEvent
		if json.Unmarshal(sc.Bytes(), &ev) != nil {
			continue
		}
		switch ev.Type {
		case "stream_event":
			if ev.Event.Type == "content_block_delta" && ev.Event.Delta.Type == "text_delta" && ev.Event.Delta.Text != "" {
				emit(ev.Event.Delta.Text)
			}
		case "result":
			if ev.IsError || ev.Subtype != "success" {
				failure = valueOrString(ev.Result, ev.Subtype)
			}
		}
	}
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if failure != "" {
		// Not logged in, usage limits…: worth a look, not an alert.
		s.log.Warn("chat friend could not answer", "reason", truncate(failure, 300))
		return errFriendAway
	}
	if waitErr != nil {
		return fmt.Errorf("%s: %w: %s", s.cfg.ChatCommand, waitErr, truncate(strings.TrimSpace(stderr.String()), 300))
	}
	return nil
}

// streamEvent is the part of Claude Code's stream-json output we read.
type streamEvent struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype"`
	IsError bool   `json:"is_error"`
	Result  string `json:"result"`
	Event   struct {
		Type  string `json:"type"`
		Delta struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"delta"`
	} `json:"event"`
}

func claudeArgs(model, system string) []string {
	return []string{
		"--print",
		"--output-format", "stream-json", "--verbose", "--include-partial-messages",
		"--no-session-persistence",
		// No CLAUDE.md, skills, plugins, hooks, MCP servers or custom agents.
		"--safe-mode",
		"--setting-sources", "",
		"--strict-mcp-config",
		"--tools", "",
		"--disable-slash-commands",
		"--model", model,
		// Replaces Claude Code's own system prompt: just the friend.
		"--system-prompt", system,
	}
}

// claudeEnv passes only what Claude Code needs to find its login, plus the
// switches that keep the account's connectors and memory out.
func claudeEnv(getenv func(string) string, dir string) []string {
	env := []string{
		"TMPDIR=" + dir,
		"ENABLE_CLAUDEAI_MCP_SERVERS=false",
		"CLAUDE_CODE_DISABLE_AUTO_MEMORY=1",
		"CLAUDE_CODE_DISABLE_CLAUDE_MDS=1",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1",
		"DISABLE_AUTOUPDATER=1",
	}
	for _, k := range []string{"PATH", "HOME", "CLAUDE_CONFIG_DIR", "CLAUDE_CODE_OAUTH_TOKEN", "SSL_CERT_FILE", "LANG"} {
		if v := getenv(k); v != "" {
			env = append(env, k+"="+v)
		}
	}
	return env
}

// limitedBuffer keeps the first max bytes written to it.
type limitedBuffer struct {
	buf *bytes.Buffer
	max int
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	if room := l.max - l.buf.Len(); room > 0 {
		l.buf.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}

func valueOrString(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
