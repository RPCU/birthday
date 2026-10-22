package server

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// chatHistory keeps today's conversation with the chat friend in one file per
// day (in the birthday's time zone) under cfg.ChatHistoryDir, next to the
// friend's Claude login, so it lives on the same volume and is shared by
// every replica. Only today's file is read, and the other days' are removed,
// so the conversation starts over every midnight.
type chatHistory struct {
	mu sync.Mutex
}

type savedChat struct {
	Messages []chatMessage `json:"messages"`
}

func (s *Server) chatDay() string {
	return s.now().In(s.cfg.Location).Format("2006-01-02")
}

// loadChat returns today's conversation, or nil.
func (s *Server) loadChat() []chatMessage {
	dir := s.cfg.ChatHistoryDir
	if dir == "" {
		return nil
	}
	s.history.mu.Lock()
	defer s.history.mu.Unlock()
	today := s.chatDay()
	s.forgetOtherDays(dir, today)

	b, err := os.ReadFile(filepath.Join(dir, today+".json"))
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			s.log.Warn("read the chat", "error", err)
		}
		return nil
	}
	var saved savedChat
	if json.Unmarshal(b, &saved) != nil || !validSaved(saved.Messages) {
		s.log.Warn("unreadable saved chat, starting over")
		return nil
	}
	return saved.Messages
}

// validSaved accepts a saved conversation: validChat's rules, except that it
// ends with the friend's reply.
func validSaved(msgs []chatMessage) bool {
	if len(msgs) < 2 || msgs[len(msgs)-1].From != "friend" {
		return false
	}
	return validChat(msgs[:len(msgs)-1]) == nil
}

// saveChat stores the conversation the page sent plus the friend's reply as
// today's.
func (s *Server) saveChat(msgs []chatMessage, reply string) {
	dir := s.cfg.ChatHistoryDir
	reply = strings.TrimSpace(reply)
	if dir == "" || reply == "" {
		return
	}
	s.history.mu.Lock()
	defer s.history.mu.Unlock()
	today := s.chatDay()

	all := append(append([]chatMessage(nil), msgs...), chatMessage{From: "friend", Text: reply})
	b, err := json.Marshal(savedChat{Messages: all})
	if err == nil {
		err = writeFileAtomic(dir, today+".json", b)
	}
	if err != nil {
		s.log.Warn("save the chat", "error", err)
		return
	}
	s.forgetOtherDays(dir, today)
}

// forgetOtherDays removes every saved conversation but today's.
func (s *Server) forgetOtherDays(dir, today string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".json") && name != today+".json" {
			if err := os.Remove(filepath.Join(dir, name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
				s.log.Warn("forget an old chat", "error", err)
			}
		}
	}
}

// writeFileAtomic writes through a temporary file and a rename, so a reader
// (another replica sharing the volume) never sees half a file.
func writeFileAtomic(dir, name string, b []byte) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".chat-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err := f.Write(b); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(dir, name))
}
