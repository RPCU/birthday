// Package server serves the countdown page, its JSON feed and static assets.
package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
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
	return &Server{cfg: cfg, log: log, page: page, static: static, now: time.Now}, nil
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
	Name    string    `json:"name"`
	Message string    `json:"message"`
	From    string    `json:"from,omitempty"`
	Start   time.Time `json:"start"`
	End     time.Time `json:"end"`
	Today   bool      `json:"today"`
	Turning int       `json:"turning,omitempty"`
	// Now lets the browser correct for a skewed local clock.
	Now time.Time `json:"now"`

	DaysLeft  int    `json:"-"`
	Halloween bool   `json:"-"`
	DateLabel string `json:"-"`
	TimeZone  string `json:"-"`
}

func (s *Server) moment() moment {
	now := s.now().In(s.cfg.Location)
	st := s.cfg.Birthday.At(now)
	return moment{
		Name:      s.cfg.Name,
		Message:   s.cfg.Message,
		From:      s.cfg.From,
		Start:     st.Start,
		End:       st.End,
		Today:     st.Today,
		Turning:   st.Turning,
		Now:       now,
		DaysLeft:  st.DaysUntil(now),
		Halloween: s.cfg.Halloween,
		DateLabel: st.Start.Format("January 2"),
		TimeZone:  s.cfg.Location.String(),
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
