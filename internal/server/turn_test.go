package server

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Banh-Canh/birthday/internal/config"
)

func TestClaudeTakesTurnsAcrossReplicas(t *testing.T) {
	lock := filepath.Join(t.TempDir(), "claude", "birthday-claude.lock")
	onTheDay := time.Date(2026, time.July, 20, 8, 0, 0, 0, time.UTC)
	// Two servers stand for two replicas sharing the volume.
	a := newServer(t, onTheDay, "Chihiro", func(c *config.Config) { c.ChatLockPath = lock })
	b := newServer(t, onTheDay, "Chihiro", func(c *config.Config) { c.ChatLockPath = lock })

	release, err := a.takeClaudeTurn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*lockPoll)
	defer cancel()
	if _, err := b.takeClaudeTurn(ctx); err == nil {
		t.Fatal("the other replica ran while the lock was held")
	}

	got := make(chan error, 1)
	go func() {
		r, err := b.takeClaudeTurn(context.Background())
		if err == nil {
			r()
		}
		got <- err
	}()
	release()
	select {
	case err := <-got:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the other replica never got its turn")
	}
}

func TestClaudeTakesTurnsInOneProcess(t *testing.T) {
	onTheDay := time.Date(2026, time.July, 20, 8, 0, 0, 0, time.UTC)
	s := newServer(t, onTheDay, "Chihiro", func(c *config.Config) { c.ChatLockPath = filepath.Join(t.TempDir(), "l.lock") })
	release, err := s.takeClaudeTurn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := s.takeClaudeTurn(ctx); err == nil {
		t.Error("two Claude Code runs at once in one process")
	}
}
