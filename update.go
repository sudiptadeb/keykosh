package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sudiptadeb/amarit"
)

// Every run says when a newer keykosh is out, checking the release manifest
// at most once per checkEvery. It never updates: that is keykosh update.
const checkEvery = 15 * time.Minute

// homeDir is keykosh's own folder: the last check and amarit's install id.
func homeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".keykosh")
}

// noticeUpdate prints a line when a newer release is known. The file
// ~/.keykosh/latest holds the newest version seen, or nothing; its mtime is
// when it was last checked, whether or not that check worked.
func noticeUpdate() {
	dir := homeDir()
	if dir == "" {
		return
	}
	running := strings.TrimPrefix(amarit.BuildVersion(), "v")
	if running == "" {
		return // a dev build has no release to compare with
	}
	file := filepath.Join(dir, "latest")
	if fi, err := os.Stat(file); err != nil || time.Since(fi.ModTime()) >= checkEvery {
		latest := ""
		if c, err := amarit.NewClient(dir, "stable"); err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			if d, err := c.Check(ctx); err == nil && d.Update {
				latest = strings.TrimPrefix(d.Version, "v")
			}
			cancel()
		}
		_ = os.MkdirAll(dir, 0o700)
		_ = os.WriteFile(file, []byte(latest), 0o600)
	}
	b, _ := os.ReadFile(file)
	if latest := strings.TrimSpace(string(b)); latest != "" && latest != running {
		say("keykosh %s is out (you have %s); run: keykosh update", latest, running)
	}
}
