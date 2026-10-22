package server

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// lockPoll is how often a waiting reply checks whether the lock is free.
const lockPoll = 200 * time.Millisecond

// takeClaudeTurn waits until no other Claude Code runs, in this process or in
// another replica sharing the login's volume (a lock file there), and returns
// the function that ends the turn. Claude Code renews the login as it starts:
// two renewals at once can revoke one another and sign the friend out.
//
// If the volume can't lock (an NFS mount without lock support), replicas no
// longer take turns, but this process still does.
func (s *Server) takeClaudeTurn(ctx context.Context) (release func(), err error) {
	select {
	case s.claudeTurn <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	done := func() { <-s.claudeTurn }
	if s.cfg.ChatLockPath == "" {
		return done, nil
	}
	f, err := lockFile(ctx, s.cfg.ChatLockPath)
	switch {
	case err == nil:
		return func() {
			_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
			_ = f.Close()
			done()
		}, nil
	case ctx.Err() != nil:
		done()
		return nil, err
	default:
		s.log.Warn("no lock on the Claude login, replicas won't take turns", "error", err)
		return done, nil
	}
}

// lockFile takes an exclusive lock on path, waiting for it until ctx ends.
// Over NFS, Linux turns flock into a lock on the server, so it holds across
// machines too.
func lockFile(ctx context.Context, path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return f, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			_ = f.Close()
			return nil, err
		}
		select {
		case <-ctx.Done():
			_ = f.Close()
			return nil, ctx.Err()
		case <-time.After(lockPoll):
		}
	}
}
