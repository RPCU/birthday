package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// pinBurst guesses are allowed at once, then pinPerMinute: with a
	// 6-digit PIN, guessing it takes months.
	pinBurst     = 5
	pinPerMinute = 5
)

type revealRequest struct {
	PIN string `json:"pin"`
}

type revealResponse struct {
	Secret string `json:"secret"`
	Link   bool   `json:"link"`
}

// handleReveal hands out the PIN-guarded secret once the card is open.
func (s *Server) handleReveal(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.cfg.SecretPIN == "" || s.cfg.Secret == "" {
		http.NotFound(w, r)
		return
	}
	if open, _ := s.cfg.Birthday.Reveal(s.now().In(s.cfg.Location), s.cfg.OpenDays); !open {
		http.NotFound(w, r)
		return
	}
	if !s.pinTries.allow(s.now()) {
		w.Header().Set("Retry-After", strconv.Itoa(60/pinPerMinute))
		writeJSONError(w, http.StatusTooManyRequests, "too many tries")
		return
	}

	var req revealRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "bad request")
		return
	}
	if !samePIN(strings.TrimSpace(req.PIN), s.cfg.SecretPIN) {
		// Never log the PIN itself.
		s.log.Warn("wrong PIN for the secret")
		writeJSONError(w, http.StatusForbidden, "wrong PIN")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(revealResponse{Secret: s.cfg.Secret, Link: isLink(s.cfg.Secret)})
}

// samePIN compares in constant time; hashing first hides the length too.
func samePIN(got, want string) bool {
	g, w := sha256.Sum256([]byte(got)), sha256.Sum256([]byte(want))
	return subtle.ConstantTimeCompare(g[:], w[:]) == 1
}

func writeJSONError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// limiter is a token bucket shared by every client: the page has a single
// recipient, so a global budget is enough to make PIN guessing slow and can't
// be dodged by spoofing client addresses.
type limiter struct {
	mu     sync.Mutex
	tokens float64
	last   time.Time
}

func (l *limiter) allow(now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.last.IsZero() {
		l.tokens = pinBurst
	} else {
		l.tokens = min(pinBurst, l.tokens+now.Sub(l.last).Minutes()*pinPerMinute)
	}
	l.last = now
	if l.tokens < 1 {
		return false
	}
	l.tokens--
	return true
}
