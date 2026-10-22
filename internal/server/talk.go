package server

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// pinAfterChat reports whether the secret's PIN waits for a few messages to
// the chat friend (cfg.ChatBeforePIN). The chat is then the way in, so it
// opens without the PIN.
func (s *Server) pinAfterChat() bool {
	return s.cfg.Chat && s.cfg.ChatBeforePIN > 0 && s.cfg.SecretPIN != "" && s.cfg.Secret != ""
}

// chatNeedsPIN reports whether the chat is locked behind the PIN.
func (s *Server) chatNeedsPIN() bool {
	return s.cfg.SecretPIN != "" && !s.pinAfterChat()
}

// messagesLeft is how many more messages the PIN waits for.
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
