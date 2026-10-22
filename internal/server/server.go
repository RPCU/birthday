// Package server serves the countdown page, its JSON feed and static assets.
package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Banh-Canh/birthday/internal/config"
	"github.com/Banh-Canh/birthday/web"
)

// contentSecurityPolicy allows only same-origin scripts (no inline ones) plus
// Google Fonts for the typefaces.
const contentSecurityPolicy = "default-src 'none'; script-src 'self'; style-src 'self' https://fonts.googleapis.com; " +
	"font-src https://fonts.gstatic.com; img-src 'self' data:; connect-src 'self'; manifest-src 'self'; " +
	"base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

// Server renders the countdown for one configured birthday.
type Server struct {
	cfg    config.Config
	log    *slog.Logger
	page   *template.Template
	static fs.FS
	// assetVersion fingerprints the CSS and JS so a deploy never serves new
	// HTML with stale cached assets.
	assetVersion string
	// pinTries throttles PIN guesses to /api/reveal and /api/chat.
	pinTries *limiter
	// chatTries and chatSlots keep the chat friend from running away with
	// the Claude login; friend writes the replies (swapped in tests).
	chatTries *limiter
	chatSlots chan struct{}
	friend    func(ctx context.Context, system, prompt string, emit func(string)) error
	// awayUntil is when to try Claude again after it failed; until then the
	// friend answers with canned lines after typing.
	awayMu    sync.Mutex
	awayUntil time.Time
	typing    time.Duration
	history   chatHistory
	notes     sync.Mutex
	// talked counts the birthday person's messages toward the PIN (see
	// talk.go), with talkMu.
	talkMu sync.Mutex
	talked int
	// claudeTurn lets one Claude Code run at a time in this process (see
	// takeClaudeTurn).
	claudeTurn chan struct{}
	// now is swapped in tests.
	now func() time.Time
}

// New parses the embedded template and prepares the static file tree.
func New(cfg config.Config, log *slog.Logger) (*Server, error) {
	page, err := template.ParseFS(web.FS, "templates/index.html")
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}
	static, err := fs.Sub(web.FS, "static")
	if err != nil {
		return nil, fmt.Errorf("static assets: %w", err)
	}
	version, err := fingerprint(static, "css/birthday.css", "js/countdown.js")
	if err != nil {
		return nil, fmt.Errorf("static assets: %w", err)
	}
	s := &Server{
		cfg:          cfg,
		log:          log,
		page:         page,
		static:       static,
		assetVersion: version,
		pinTries:     newLimiter(pinBurst, pinPerMinute),
		chatTries:    newLimiter(chatBurst, chatPerMinute),
		chatSlots:    make(chan struct{}, chatSlots),
		typing:       fallbackTyping,
		claudeTurn:   make(chan struct{}, 1),
		now:          time.Now,
	}
	s.friend = s.claudeFriend
	return s, nil
}

// isLink reports whether the secret is a single web link, so the page can
// offer to open it.
func isLink(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && !strings.ContainsAny(s, " \n")
}

// initial is the first letter of name, for the chat avatar.
func initial(name string) string {
	for _, r := range name {
		return strings.ToUpper(string(r))
	}
	return ""
}

// fingerprint returns a short content hash of the named files.
func fingerprint(fsys fs.FS, names ...string) (string, error) {
	h := sha256.New()
	for _, name := range names {
		b, err := fs.ReadFile(fsys, name)
		if err != nil {
			return "", err
		}
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil))[:12], nil
}

// Handler returns the routes wrapped in the common middleware.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /api/countdown", s.handleCountdown)
	// Same-origin only: rejects cross-site form posts and fetches.
	mux.Handle("POST /api/reveal", http.NewCrossOriginProtection().Handler(http.HandlerFunc(s.handleReveal)))
	mux.Handle("POST /api/chat", http.NewCrossOriginProtection().Handler(http.HandlerFunc(s.handleChat)))
	mux.HandleFunc("GET /healthz", handleHealth)
	mux.Handle("GET /static/", http.StripPrefix("/static/", cacheFor(time.Hour, http.FileServerFS(s.static))))
	return s.recoverPanics(securityHeaders(s.logRequests(mux)))
}

// buildupDays is how long before the birthday the countdown page starts
// to dress up.
const buildupDays = 10

// partyFlecks is how many confetti flecks drift by once the card is open.
const partyFlecks = 8

// moment is everything the page and the JSON feed say about "now".
type moment struct {
	Name string `json:"name"`
	// Message and From are empty unless Open, so the surprise can't be read
	// early from the page source or this feed.
	Message string `json:"message,omitempty"`
	// Open is true from the start of the birthday until OpenUntil.
	Open      bool      `json:"open"`
	OpenUntil time.Time `json:"openUntil,omitzero"`
	From      string    `json:"from,omitempty"`
	Start     time.Time `json:"start"`
	End       time.Time `json:"end"`
	Today     bool      `json:"today"`
	Turning   int       `json:"turning,omitempty"`
	// Now lets the browser correct for a skewed local clock.
	Now time.Time `json:"now"`

	Teaser string `json:"-"`
	// Secret is set only while Open and when no PIN guards it. It is
	// rendered in the page, never in the JSON feed. SecretLocked means a
	// secret exists but needs the PIN (POST /api/reveal).
	Secret       string `json:"-"`
	SecretLocked bool   `json:"-"`
	SecretURL    bool   `json:"-"`
	SecretIntro  string `json:"-"`
	// Chat is set only while Open and the chat is on; ChatLocked means it
	// needs the PIN first. SecretWaits is how many more messages to the
	// friend the secret waits for after the PIN, out of ChatBeforePIN.
	Chat        bool   `json:"-"`
	ChatLocked  bool   `json:"-"`
	ChatName    string `json:"-"`
	ChatInitial string `json:"-"`
	// ChatGreeting is the friend's opening line, with the name filled in.
	ChatGreeting string `json:"-"`
	SecretWaits  int    `json:"-"`
	ChatBefore   int    `json:"-"`
	DaysLeft     int    `json:"-"`
	// Buildup is the party building up over the last buildupDays before
	// the birthday: 1 ten days before, up to 10 the day before, and all of
	// it while Open. Flecks is how many slow confetti flecks drift by (half
	// as many, more while Open). Party adds balloons while Open.
	Buildup      int    `json:"-"`
	Flecks       int    `json:"-"`
	Party        bool   `json:"-"`
	Halloween    bool   `json:"-"`
	DateLabel    string `json:"-"`
	LastOpenDay  string `json:"-"`
	TimeZone     string `json:"-"`
	AssetVersion string `json:"-"`
}

func (s *Server) moment() moment {
	now := s.now().In(s.cfg.Location)
	st := s.cfg.Birthday.At(now)
	open, until := s.cfg.Birthday.Reveal(now, s.cfg.OpenDays)
	var message, from, secret, lastOpenDay string
	var locked, chat bool
	var waits, buildup int
	flecks := 0
	switch days := st.DaysUntil(now); {
	case open:
		buildup, flecks = buildupDays, partyFlecks
	case days >= 1 && days <= buildupDays:
		buildup = buildupDays + 1 - days
		flecks = buildup / 2
	}
	if open {
		chat = s.cfg.Chat
		message, from = s.cfg.Message, s.cfg.From
		if s.cfg.SecretPIN == "" {
			secret = s.cfg.Secret
		} else {
			locked = s.cfg.Secret != ""
		}
		switch {
		case !s.pinAfterChat():
		case s.cfg.ChatCountReset:
			// The PIN starts a fresh count every time.
			waits = s.cfg.ChatBeforePIN
		default:
			waits = s.messagesLeft()
		}
		lastOpenDay = until.AddDate(0, 0, -1).Format("January 2")
	}
	return moment{
		Name:         s.cfg.Name,
		Message:      message,
		Open:         open,
		OpenUntil:    until,
		From:         from,
		Start:        st.Start,
		End:          st.End,
		Today:        st.Today,
		Turning:      st.Turning,
		Now:          now,
		Teaser:       s.cfg.Teaser,
		Secret:       secret,
		SecretURL:    isLink(secret),
		SecretLocked: locked,
		SecretIntro:  s.cfg.SecretIntro,
		Chat:         chat,
		ChatLocked:   chat && s.cfg.SecretPIN != "",
		ChatName:     s.cfg.ChatName,
		ChatInitial:  initial(s.cfg.ChatName),
		ChatGreeting: strings.ReplaceAll(s.cfg.ChatGreeting, "{name}", s.cfg.Name),
		SecretWaits:  waits,
		ChatBefore:   s.cfg.ChatBeforePIN,
		DaysLeft:     st.DaysUntil(now),
		Buildup:      buildup,
		Flecks:       flecks,
		Party:        open,
		Halloween:    s.cfg.Halloween,
		DateLabel:    st.Start.Format("January 2"),
		LastOpenDay:  lastOpenDay,
		TimeZone:     s.cfg.Location.String(),
		AssetVersion: s.assetVersion,
	}
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	// Render into a buffer so a template error becomes a clean 500 instead
	// of a half-written page.
	var buf bytes.Buffer
	if err := s.page.Execute(&buf, s.moment()); err != nil {
		s.log.Error("render page", "error", err)
		http.Error(w, "something went wrong", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = buf.WriteTo(w)
}

func (s *Server) handleCountdown(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(s.moment()); err != nil {
		s.log.Warn("write countdown", "error", err)
	}
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte("ok\n"))
}
