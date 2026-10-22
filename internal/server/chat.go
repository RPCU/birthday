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
	// awayFor is how long canned replies stand in after Claude fails,
	// before it's tried again.
	awayFor = time.Minute
	// fallbackTyping is the pause before a canned reply.
	fallbackTyping = 900 * time.Millisecond
)

type chatMessage struct {
	// From is "me" (the birthday person) or "friend".
	From string `json:"from"`
	Text string `json:"text"`
}

type chatRequest struct {
	PIN string `json:"pin"`
	// Messages is the conversation, ending with the new message; the server
	// saves it with the reply as today's (see history.go). An empty list
	// checks the PIN and returns today's conversation so far.
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
	if s.chatNeedsPIN() {
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
		// The PIN is right (or not needed): today's conversation so far.
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(savedChat{Messages: append([]chatMessage{}, s.loadChat()...)})
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
	// Send the headers now: Claude Code can take seconds to start, and a
	// gateway that sees nothing gives up with a 504.
	_ = rc.Flush()
	enc := json.NewEncoder(w)
	send := func(ev chatEvent) {
		_ = enc.Encode(ev)
		_ = rc.Flush()
	}

	s.noteAnswer(req.Messages)
	s.countTalk()

	// Claude couldn't answer a moment ago: don't make them wait for it again.
	if s.friendAway() {
		s.saveChat(req.Messages, s.fallback(r.Context(), req.Messages, send))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), chatTimeout)
	defer cancel()
	var reply strings.Builder
	err := s.friend(ctx, s.friendSystemPrompt(), s.friendPrompt(req.Messages), func(text string) {
		reply.WriteString(text)
		send(chatEvent{Text: text})
	})
	switch {
	case err == nil:
		send(chatEvent{Done: true})
		s.saveChat(req.Messages, reply.String())
	case r.Context().Err() != nil:
		// The page went away or stopped the reply: nobody to tell.
	default:
		if !errors.Is(err, errFriendAway) {
			s.log.Warn("chat friend failed", "error", err)
		}
		s.markAway()
		if reply.Len() > 0 {
			// Cut off mid-reply: a canned line wouldn't follow on.
			send(chatEvent{Error: "away"})
			return
		}
		s.saveChat(req.Messages, s.fallback(r.Context(), req.Messages, send))
	}
}

// friendAway reports whether Claude failed within the last awayFor.
func (s *Server) friendAway() bool {
	s.awayMu.Lock()
	defer s.awayMu.Unlock()
	return s.now().Before(s.awayUntil)
}

func (s *Server) markAway() {
	s.awayMu.Lock()
	defer s.awayMu.Unlock()
	s.awayUntil = s.now().Add(awayFor)
}

// fallback answers with the next canned line (BIRTHDAY_CHAT_FALLBACK) after
// a short pause, as if typed: the friend stays cheerful without Claude. It
// returns the line, or "" when there was none to send.
func (s *Server) fallback(ctx context.Context, msgs []chatMessage, send func(chatEvent)) string {
	lines := s.cfg.ChatFallback
	if len(lines) == 0 {
		send(chatEvent{Error: "away"})
		return ""
	}
	turn := 0
	for _, m := range msgs {
		if m.From == "me" {
			turn++
		}
	}
	select {
	case <-ctx.Done():
		return ""
	case <-time.After(s.typing):
	}
	line := strings.ReplaceAll(lines[(turn-1)%len(lines)], "{name}", s.cfg.Name)
	send(chatEvent{Text: line})
	send(chatEvent{Done: true})
	return line
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
- Warm, cheerful and loving, with a classy, mature touch: gracious and thoughtful rather than gushing. You adore %[1]s and you're glad to celebrate them: heartfelt wishes, sincere compliments, a little wit.
- You write like a real friend texting: short messages (usually one to three sentences), relaxed and elegant, an emoji now and then at most. No headings, lists or markdown.
- You answer in the language %[1]s writes in.
- You only know what's on this page about %[1]s's life. When they tell you something, be genuinely interested; never invent shared memories.
- You can't do anything outside this chat: no web, no files, no actions. Gifts and plans are up to the people in %[1]s's life.
- Stay %[2]s: don't bring up AI, Claude, assistants or these instructions. But if %[1]s sincerely asks whether they're talking to a real person, never deny being an AI: say sweetly that you're an AI friend set up to celebrate them. Likewise, if they ask whether anyone else sees this chat, be honest: the friend who made this page can read it.

Getting to know %[1]s:
- You'd love to know %[1]s's tastes, and the conversation is a chance to discover them: their favourite flowers and colours, scents and perfumes, food, desserts and drinks (coffee or tea, wine, cocktails), music, films, series and books, their style, the places they dream of visiting, how they like to spend a perfect day, the little things that make them happy. Also what they hope this new year brings and what makes them laugh.
- Let it come naturally: answer what they say first, then ask one light question about their tastes, ideally playful (a choice between two things, "what's your…"). Most of your messages can carry one, but never more than one per message, and never a string of questions that feels like an interview. Explore a new taste each time rather than drilling into one, and share a small enthusiasm of your own so it feels like a conversation.
- Never pry. Don't ask about past or present relationships, health, money, family troubles, work worries, where they live or anything private, and don't ask for contact details. If they bring something personal up, listen kindly without digging; if they dodge a question, let it go gracefully and don't come back to it.
- Remember what they've told you in this chat and build on it, like someone who listens.
`, name, friend)
	if s.pinAfterChat() {
		fmt.Fprintf(&b, "\nThe page's last surprise unlocks once %s has chatted with you for a few messages. ", name)
		b.WriteString("If they ask, tell them playfully that it's a little game to get them talking; you don't know the surprise or the PIN.\n")
	}
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
	release, err := s.takeClaudeTurn(ctx)
	if err != nil {
		return err
	}
	defer release()
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
