package main

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/term"
)

const projectURL = "https://github.com/sudiptadeb/keykosh"

// repoGuide is docs/repo-guide.md: what a coding agent needs to move a
// repo's keys into a kosh. keykosh prompt prints it with the kosh and the
// repo filled in.
//
//go:embed docs/repo-guide.md
var repoGuide string

// copyHint is how to put a command's output on the clipboard here.
func copyHint(cmd string) string {
	if runtime.GOOS == "darwin" {
		return cmd + " | pbcopy"
	}
	return cmd
}

// runPrompt prints the repo guide for the repo in this folder (or the one
// named), ready to paste to a coding agent.
func runPrompt(f flags, args []string) {
	if len(args) > 1 {
		fail(fmt.Errorf("prompt: one repo name at most"))
	}
	root := "$KEYKOSH_ROOT"
	if k := tryFind(f); k != nil {
		root = tilde(k.Root)
	}
	repo := "<repo>"
	if len(args) == 1 {
		repo = args[0]
	} else if top, err := exec.Command("git", "rev-parse", "--show-toplevel").Output(); err == nil {
		repo = filepath.Base(strings.TrimSpace(string(top)))
	}
	out := strings.NewReplacer("{{root}}", root, "{{repo}}", repo).Replace(repoGuide)
	if term.IsTerminal(int(os.Stdout.Fd())) {
		say("paste this to your coding agent in %s (or run: %s)\n", repo, copyHint("keykosh prompt"))
	}
	fmt.Print(out)
}

// tryFind is find without failing: nil when no kosh is in reach.
func tryFind(f flags) *Kosh {
	dir := f.dir
	switch {
	case dir != "":
	case IsKosh("."):
		dir = "."
	default:
		dir = os.Getenv(rootEnv)
	}
	if dir == "" {
		return nil
	}
	k, err := Open(dir)
	if err != nil {
		return nil
	}
	return k
}

// afterInit is the screen a new kosh ends on: what to do next, and how
// repos and their coding agents use it.
func afterInit(k *Kosh) {
	root := tilde(k.Root)
	git := "cd " + root + " && git init && git add -A && git commit -m keys"
	if _, err := os.Stat(filepath.Join(k.Root, ".git")); err == nil {
		git = "cd " + root + " && git add -A && git commit -m keys"
	}
	fmt.Printf(`
%s is a kosh, locked with your passphrase.

Next
  1. Put keys in it, a folder per repo:   mkdir %s/myapp && cp release.jks %s/myapp/
  2. Seal them (no passphrase needed):    keykosh seal
  3. Keep it in a private git repo:       %s

Use it from your repos
  Scripts read keys from $KEYKOSH_ROOT/<repo>/, or from <repo>/.keys/
  where keykosh is not set up. To have a coding agent move a repo's keys
  in and switch its scripts over, run this in that repo and paste it:

    %s

On a new machine: clone the kosh, then keykosh unlock.
Learn more: %s
`, root, root, root, git, copyHint("keykosh prompt"), projectURL)
}

// hints says what to run next, one line per kind of key that needs it.
func hints(k *Kosh, items []Item) {
	if len(items) == 0 {
		say("no keys yet. Put files in %s, then run: keykosh seal", tilde(k.Root))
		return
	}
	n := map[string]int{}
	for _, it := range items {
		n[it.Status]++
	}
	if c := n[New] + n[Changed]; c > 0 {
		say("%s new or changed: run keykosh seal", count(c))
	}
	if c := n[Locked]; c > 0 {
		say("%s locked: run keykosh unlock", count(c))
	}
	if c := n[Stale]; c > 0 {
		say("%s sealed again elsewhere: run keykosh unlock to refresh", count(c))
	}
	if c := n[Unknown]; c > 0 {
		say("%s not yet unlocked on this machine: run keykosh unlock to check", count(c))
	}
	if c := n[Conflict]; c > 0 {
		say("%s changed here and in the sealed copy: keep one by hand, then seal", count(c))
	}
}

func count(n int) string {
	if n == 1 {
		return "1 key"
	}
	return fmt.Sprintf("%d keys", n)
}
