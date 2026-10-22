package server

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// pinAfterChat reports whether the secret waits for a few messages to the
// chat friend (cfg.ChatBeforePIN): the PIN opens the chat first, and the
// secret only once they've talked.
func (s *Server) pinAfterChat() bool {
	return s.cfg.Chat && s.cfg.ChatBeforePIN > 0 && s.cfg.SecretPIN != "" && s.cfg.Secret != ""
}

// messagesLeft is how many more messages the secret waits for.
func (s *Server) messagesLeft() int {
	s.talkMu.Lock()
	defer s.talkMu.Unlock()
	return max(0, s.cfg.ChatBeforePIN-s.talkedLocked())
}

// talkedLocked is how many messages count so far: the count kept next to the
// login (cfg.ChatTalkedPath, shared by the replicas on its volume), or this
// process's own without one. It never resets: once the PIN is open, it stays
// open.
func (s *Server) talkedLocked() int {
	n := s.talked
	if p := s.cfg.ChatTalkedPath; p != "" {
		if b, err := os.ReadFile(p); err == nil {
			if v, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil {
				n = max(n, v)
			}
		}
	}
	return n
}

// countTalk counts a message the birthday person sent, up to
// cfg.ChatBeforePIN.
func (s *Server) countTalk() {
	if !s.pinAfterChat() {
		return
	}
	s.talkMu.Lock()
	defer s.talkMu.Unlock()
	n := s.talkedLocked()
	if n >= s.cfg.ChatBeforePIN {
		return
	}
	s.talked = n + 1
	if p := s.cfg.ChatTalkedPath; p != "" {
		if err := writeFileAtomic(filepath.Dir(p), filepath.Base(p), []byte(strconv.Itoa(s.talked)+"\n")); err != nil {
			s.log.Warn("count a chat message", "error", err)
		}
	}
}
