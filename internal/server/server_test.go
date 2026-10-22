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
	return newTestServerWith(t, now, name, func(*config.Config) {})
}

func newTestServerWith(t *testing.T, now time.Time, name string, tweak func(*config.Config)) http.Handler {
	t.Helper()
	return newServer(t, now, name, tweak).Handler()
}

func newServer(t *testing.T, now time.Time, name string, tweak func(*config.Config)) *Server {
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
		Teaser:    config.DefaultTeaser,
		Location:  paris,
		OpenDays:  config.DefaultOpenDays,
		Halloween: true,
	}
	tweak(&cfg)
	s, err := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return now }
	s.typing = 0
	return s
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
	for _, want := range []string{"Chihiro", "July 20", "turning 30", "10 sleeps to go", config.DefaultTeaser, "Unlocks when the countdown ends · in 10 sleeps"} {
		if !strings.Contains(body, want) {
			t.Errorf("page does not contain %q", want)
		}
	}
	if !strings.Contains(body, `class="moon"`) {
		t.Error("Halloween touches missing")
	}
	for _, secret := range []string{config.DefaultMessage, "I.O.U.", "Victor"} {
		if strings.Contains(body, secret) {
			t.Errorf("the surprise leaks before the birthday: page contains %q", secret)
		}
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
	if !strings.Contains(body, config.DefaultMessage) {
		t.Error("the I.O.U. line is not revealed on the day")
	}
	if strings.Contains(body, config.DefaultTeaser) {
		t.Error("the teaser is still shown once unlocked")
	}
}

func TestIOUStaysOpenAfterTheDay(t *testing.T) {
	tests := []struct {
		name string
		now  time.Time
		open bool
	}{
		{"five days after", time.Date(2026, time.July, 25, 12, 0, 0, 0, time.UTC), true},
		{"last open day", time.Date(2026, time.August, 4, 12, 0, 0, 0, time.UTC), true},
		{"the day after that", time.Date(2026, time.August, 5, 12, 0, 0, 0, time.UTC), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := get(t, newTestServer(t, tt.now, "Chihiro"), "/").Body.String()
			if got := strings.Contains(body, config.DefaultMessage); got != tt.open {
				t.Errorf("I.O.U. shown = %v, want %v", got, tt.open)
			}
			if tt.open && !strings.Contains(body, "offer open until August 4") {
				t.Error("missing the last open day")
			}
			if strings.Contains(body, "Happy birthday") {
				t.Error("still celebrating after the day")
			}
		})
	}
}

func TestSecretOnlyOnceOpen(t *testing.T) {
	const code = "UBER-GIFT-7Q2X"
	withSecret := func(c *config.Config) {
		c.Secret = code
		c.SecretIntro = "And a ride to get there"
	}
	before := time.Date(2026, time.July, 10, 12, 0, 0, 0, time.UTC)
	for _, path := range []string{"/", "/api/countdown"} {
		if body := get(t, newTestServerWith(t, before, "Chihiro", withSecret), path).Body.String(); strings.Contains(body, code) {
			t.Errorf("GET %s leaks the secret before the birthday", path)
		}
	}

	onTheDay := time.Date(2026, time.July, 20, 8, 0, 0, 0, time.UTC)
	h := newTestServerWith(t, onTheDay, "Chihiro", withSecret)
	page := get(t, h, "/").Body.String()
	if !strings.Contains(page, code) || !strings.Contains(page, "And a ride to get there") {
		t.Error("the secret and its intro are not on the opened card")
	}
	if strings.Contains(page, "extra-open") {
		t.Error("a code that is not a link must not get an Open link")
	}
	if strings.Contains(get(t, h, "/api/countdown").Body.String(), code) {
		t.Error("the secret must never be in the JSON feed")
	}

	link := func(c *config.Config) { c.Secret = "https://r.uber.com/gift/abc" }
	if page := get(t, newTestServerWith(t, onTheDay, "Chihiro", link), "/").Body.String(); !strings.Contains(page, `href="https://r.uber.com/gift/abc"`) {
		t.Error("a link secret should get an Open link")
	}

	if page := get(t, newTestServer(t, onTheDay, "Chihiro"), "/").Body.String(); strings.Contains(page, `class="extra"`) {
		t.Error("no secret configured, but the extra block is rendered")
	}
}

func TestAssetsAreFingerprinted(t *testing.T) {
	body := get(t, newTestServer(t, time.Now(), "Chihiro"), "/").Body.String()
	if !strings.Contains(body, "/static/css/birthday.css?v=") || !strings.Contains(body, "/static/js/countdown.js?v=") {
		t.Error("asset URLs are not versioned")
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
	if got.Message != "" || got.From != "" || got.Open {
		t.Errorf("message = %q, from = %q, open = %v before the birthday, want them withheld", got.Message, got.From, got.Open)
	}
	if want := "2027-07-20T00:00:00+02:00"; got.Start.Format(time.RFC3339) != want {
		t.Errorf("start = %s, want %s", got.Start.Format(time.RFC3339), want)
	}
}

func TestCountdownJSONOnTheDay(t *testing.T) {
	now := time.Date(2026, time.July, 20, 8, 0, 0, 0, time.UTC)
	var got moment
	if err := json.NewDecoder(get(t, newTestServer(t, now, "Chihiro"), "/api/countdown").Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if !got.Today || !got.Open || got.Message != config.DefaultMessage {
		t.Errorf("moment = %+v, want today with the message", got)
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

func TestBuildupGrowsOverTheLastTenDays(t *testing.T) {
	for _, c := range []struct {
		day                        int
		pennants, sparkles, flecks int
	}{
		{9, 0, 0, 0},
		{10, 1, 1, 0},
		{15, 6, 6, 3},
		{19, 10, 10, 5},
		{20, 0, 0, 0}, // the day itself has its own party
	} {
		page := get(t, newTestServer(t, time.Date(2026, time.July, c.day, 12, 0, 0, 0, time.UTC), "Chihiro"), "/").Body.String()
		got := [3]int{strings.Count(page, `class="pennant `), strings.Count(page, "<i></i>"), strings.Count(page, `class="fleck"`)}
		if want := [3]int{c.pennants, c.sparkles, c.flecks}; got != want {
			t.Errorf("July %d: pennants, sparkles, flecks = %v, want %v", c.day, got, want)
		}
	}
}
