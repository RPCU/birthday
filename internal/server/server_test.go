package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Banh-Canh/birthday/internal/config"
	"github.com/Banh-Canh/birthday/internal/countdown"
)

func newTestServer(t *testing.T, now time.Time, name string) http.Handler {
	t.Helper()
	paris, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		Name:      name,
		Message:   config.DefaultMessage,
		From:      "Victor",
		Birthday:  countdown.Birthday{Year: 1996, Month: time.July, Day: 20},
		Location:  paris,
		Halloween: true,
	}
	s, err := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return now }
	return s.Handler()
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestIndexCountingDown(t *testing.T) {
	h := newTestServer(t, time.Date(2026, time.July, 10, 12, 0, 0, 0, time.UTC), "Chihiro")
	rec := get(t, h, "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Chihiro", "I owe you a birthday drink", "Victor", "July 20", "turning 30", "10 sleeps to go"} {
		if !strings.Contains(body, want) {
			t.Errorf("page does not contain %q", want)
		}
	}
	if !strings.Contains(body, `class="moon"`) {
		t.Error("Halloween touches missing")
	}
	if strings.Contains(body, "Happy birthday") {
		t.Error("page celebrates before the day")
	}
}

func TestIndexOnTheDay(t *testing.T) {
	h := newTestServer(t, time.Date(2026, time.July, 20, 8, 0, 0, 0, time.UTC), "Chihiro")
	body := get(t, h, "/").Body.String()
	if !strings.Contains(body, "Happy birthday") {
		t.Error("page does not celebrate on the day")
	}
}

func TestIndexEscapesName(t *testing.T) {
	h := newTestServer(t, time.Date(2026, time.July, 10, 0, 0, 0, 0, time.UTC), `<script>alert(1)</script>`)
	body := get(t, h, "/").Body.String()
	if strings.Contains(body, "<script>alert(1)</script>") {
		t.Error("name is not HTML-escaped")
	}
}

func TestCountdownJSON(t *testing.T) {
	now := time.Date(2026, time.October, 21, 9, 0, 0, 0, time.UTC)
	rec := get(t, newTestServer(t, now, "Chihiro"), "/api/countdown")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	var got moment
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Name != "Chihiro" || got.Today || got.Turning != 31 {
		t.Errorf("moment = %+v", got)
	}
	if want := "2027-07-20T00:00:00+02:00"; got.Start.Format(time.RFC3339) != want {
		t.Errorf("start = %s, want %s", got.Start.Format(time.RFC3339), want)
	}
}

func TestHeadersAndRoutes(t *testing.T) {
	h := newTestServer(t, time.Now(), "Chihiro")

	rec := get(t, h, "/")
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "script-src 'self'") {
		t.Errorf("CSP = %q", csp)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("missing nosniff")
	}

	tests := map[string]int{
		"/healthz":                 http.StatusOK,
		"/static/css/birthday.css": http.StatusOK,
		"/static/js/countdown.js":  http.StatusOK,
		"/static/favicon.svg":      http.StatusOK,
		"/nope":                    http.StatusNotFound,
	}
	for path, want := range tests {
		if got := get(t, h, path).Code; got != want {
			t.Errorf("GET %s = %d, want %d", path, got, want)
		}
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST / = %d, want 405", rec.Code)
	}
}
