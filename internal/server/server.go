// Package server serves the countdown page, its JSON feed and static assets.
package server

import (
	"bytes"
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
	return &Server{cfg: cfg, log: log, page: page, static: static, assetVersion: version, now: time.Now}, nil
}

// isLink reports whether the secret is a single web link, so the page can
// offer to open it.
func isLink(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && !strings.ContainsAny(s, " \n")
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
	mux.HandleFunc("GET /healthz", handleHealth)
	mux.Handle("GET /static/", http.StripPrefix("/static/", cacheFor(time.Hour, http.FileServerFS(s.static))))
	return s.recoverPanics(securityHeaders(s.logRequests(mux)))
}

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
	// Secret is set only while Open. It is rendered in the page, never in
	// the JSON feed.
	Secret       string `json:"-"`
	SecretURL    bool   `json:"-"`
	SecretIntro  string `json:"-"`
	DaysLeft     int    `json:"-"`
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
	if open {
		message, from, secret = s.cfg.Message, s.cfg.From, s.cfg.Secret
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
		SecretIntro:  s.cfg.SecretIntro,
		DaysLeft:     st.DaysUntil(now),
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
