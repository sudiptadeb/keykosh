package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// The default kosh lives in KEYKOSH_ROOT, set by one line in the rc file of
// the user's login shell. keykosh owns that line (it ends in "# keykosh")
// and replaces it rather than adding another.
const rootEnv = "KEYKOSH_ROOT"

const marker = "# keykosh"

// shellFile picks the file and the line for the user's login shell. ok is
// false for a shell keykosh does not know.
func shellFile(root string) (file, line string, ok bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", false
	}
	quoted := `"` + strings.ReplaceAll(root, `"`, `\"`) + `"`
	export := "export " + rootEnv + "=" + quoted + "  " + marker
	switch filepath.Base(os.Getenv("SHELL")) {
	case "zsh":
		// .zshenv is read by every zsh, scripts included; .zshrc only by
		// interactive ones.
		dir := os.Getenv("ZDOTDIR")
		if dir == "" {
			dir = home
		}
		return filepath.Join(dir, ".zshenv"), export, true
	case "bash":
		// macOS terminals start login shells (.bash_profile); Linux ones do
		// not (.bashrc).
		if runtime.GOOS == "darwin" {
			return filepath.Join(home, ".bash_profile"), export, true
		}
		return filepath.Join(home, ".bashrc"), export, true
	case "fish":
		return filepath.Join(home, ".config", "fish", "conf.d", "keykosh.fish"),
			"set -gx " + rootEnv + " " + quoted + "  " + marker, true
	}
	return "", "export " + rootEnv + "=" + quoted, false
}

// setShellDefault writes line into file, replacing an earlier keykosh line.
func setShellDefault(file, line string) error {
	var keep []string
	mode := os.FileMode(0o644)
	if fi, err := os.Stat(file); err == nil {
		mode = fi.Mode().Perm()
		b, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		for _, l := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
			if !strings.HasSuffix(strings.TrimSpace(l), marker) {
				keep = append(keep, l)
			}
		}
		if len(keep) == 1 && keep[0] == "" {
			keep = nil
		}
	}
	keep = append(keep, line)
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	return writeFile(file, []byte(strings.Join(keep, "\n")+"\n"), mode)
}

// offerDefault makes root the default kosh in the shell's rc file: asking
// first on a terminal, without asking under --yes, and only printing the
// line otherwise.
func offerDefault(root string, f flags) {
	if f.noShell || os.Getenv(rootEnv) == root {
		return
	}
	file, line, ok := shellFile(root)
	if !ok {
		say("to make this your default kosh, add to your shell's startup file:\n  %s", line)
		return
	}
	if !f.yes {
		if !interactive() {
			say("to make this your default kosh, add to %s:\n  %s\n(or run again with --yes)", tilde(file), line)
			return
		}
		if !confirm(fmt.Sprintf("Make %s your default kosh? This writes one line to %s", tilde(root), tilde(file))) {
			return
		}
	}
	if err := setShellDefault(file, line); err != nil {
		say("could not write %s: %v", tilde(file), err)
		return
	}
	say("default kosh set in %s; open a new terminal or run: source %s", tilde(file), tilde(file))
}

func tilde(p string) string {
	if home, err := os.UserHomeDir(); err == nil && (p == home || strings.HasPrefix(p, home+"/")) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}
