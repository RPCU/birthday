package server

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// noteAnswer adds the birthday person's newest message, after the friend's
// line it answers, to cfg.ChatNotesPath: a Markdown file for the page's owner
// to read their answers, next to the login so it shares its volume. Unlike
// the day's conversation it's never cleared, and the page never shows it.
func (s *Server) noteAnswer(msgs []chatMessage) {
	path := s.cfg.ChatNotesPath
	if path == "" {
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## %s\n\n", s.now().In(s.cfg.Location).Format("Monday, January 2, 2006, 15:04"))
	if n := len(msgs); n > 1 && msgs[n-2].From == "friend" {
		// As the page sent it: what the friend asked.
		q := strings.ReplaceAll(strings.TrimSpace(msgs[n-2].Text), "\n", "\n> ")
		fmt.Fprintf(&b, "> **%s:** %s\n\n", s.cfg.ChatName, q)
	}
	fmt.Fprintf(&b, "**%s:** %s\n\n", s.cfg.Name, strings.TrimSpace(msgs[len(msgs)-1].Text))

	s.notes.Lock()
	defer s.notes.Unlock()
	err := os.MkdirAll(filepath.Dir(path), 0o700)
	if err == nil {
		var f *os.File
		if f, err = os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
			_, err = f.WriteString(b.String())
			if cerr := f.Close(); err == nil {
				err = cerr
			}
		}
	}
	if err != nil {
		s.log.Warn("note an answer", "error", err)
	}
}
