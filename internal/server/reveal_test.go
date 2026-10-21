package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Banh-Canh/birthday/internal/config"
)

const (
	testCode = "UBER-GIFT-7Q2X"
	testPIN  = "482916"
)

func withPIN(c *config.Config) {
	c.Secret = testCode
	c.SecretPIN = testPIN
}

func reveal(t *testing.T, h http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/reveal", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestPINGuardsTheSecret(t *testing.T) {
	onTheDay := time.Date(2026, time.July, 20, 8, 0, 0, 0, time.UTC)
	h := newTestServerWith(t, onTheDay, "Chihiro", withPIN)

	page := get(t, h, "/").Body.String()
	if strings.Contains(page, testCode) {
		t.Fatal("the page contains the PIN-guarded secret")
	}
	if !strings.Contains(page, `class="extra-pin"`) {
		t.Error("the page has no PIN form")
	}

	if rec := reveal(t, h, `{"pin":"000000"}`); rec.Code != http.StatusForbidden || strings.Contains(rec.Body.String(), testCode) {
		t.Errorf("wrong PIN: status %d, body %s", rec.Code, rec.Body)
	}

	rec := reveal(t, h, `{"pin":" 482916 "}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("right PIN: status %d", rec.Code)
	}
	var got revealResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil || got.Secret != testCode {
		t.Errorf("right PIN: got %+v, %v", got, err)
	}
}

func TestRevealOnlyWhileOpen(t *testing.T) {
	before := time.Date(2026, time.July, 10, 12, 0, 0, 0, time.UTC)
	if rec := reveal(t, newTestServerWith(t, before, "Chihiro", withPIN), `{"pin":"482916"}`); rec.Code != http.StatusNotFound {
		t.Errorf("before the birthday: status %d, want 404", rec.Code)
	}
}

func TestRevealWithoutPINIsNotAnEndpoint(t *testing.T) {
	onTheDay := time.Date(2026, time.July, 20, 8, 0, 0, 0, time.UTC)
	noPIN := func(c *config.Config) { c.Secret = testCode }
	if rec := reveal(t, newTestServerWith(t, onTheDay, "Chihiro", noPIN), `{"pin":"x"}`); rec.Code != http.StatusNotFound {
		t.Errorf("status %d, want 404", rec.Code)
	}
}

func TestRevealIsThrottled(t *testing.T) {
	onTheDay := time.Date(2026, time.July, 20, 8, 0, 0, 0, time.UTC)
	h := newTestServerWith(t, onTheDay, "Chihiro", withPIN)
	for i := range pinBurst {
		if rec := reveal(t, h, `{"pin":"000000"}`); rec.Code != http.StatusForbidden {
			t.Fatalf("guess %d: status %d, want 403", i+1, rec.Code)
		}
	}
	rec := reveal(t, h, `{"pin":"482916"}`)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Errorf("after %d guesses: status %d, want 429 with Retry-After", pinBurst, rec.Code)
	}
}

func TestRevealRejectsCrossOrigin(t *testing.T) {
	onTheDay := time.Date(2026, time.July, 20, 8, 0, 0, 0, time.UTC)
	h := newTestServerWith(t, onTheDay, "Chihiro", withPIN)
	req := httptest.NewRequest(http.MethodPost, "/api/reveal", strings.NewReader(`{"pin":"482916"}`))
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || strings.Contains(rec.Body.String(), testCode) {
		t.Errorf("cross-site POST: status %d", rec.Code)
	}
}

func TestLimiterRefills(t *testing.T) {
	var l limiter
	t0 := time.Date(2026, time.July, 20, 8, 0, 0, 0, time.UTC)
	for range pinBurst {
		if !l.allow(t0) {
			t.Fatal("burst denied")
		}
	}
	if l.allow(t0) {
		t.Fatal("allowed past the burst")
	}
	if !l.allow(t0.Add(time.Minute / pinPerMinute)) {
		t.Error("no token after the refill interval")
	}
}
