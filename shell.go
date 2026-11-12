package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// The default kosh lives in KEYKOSH_ROOT, set by one line in a shell's
// startup file. keykosh owns that line (it ends in "# keykosh") and
// replaces it rather than adding another.
const rootEnv = "KEYKOSH_ROOT"

const marker = "# keykosh"

// rcFile is one startup file keykosh can write the default into.
type rcFile struct {
	shell string // zsh, bash, fish
	path  string
	note  string
}

// rcFiles lists the files to offer, the login shell's own first.
func rcFiles() []rcFile {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	zdot := os.Getenv("ZDOTDIR")
	if zdot == "" {
		zdot = home
	}
	login, interactive := "login shells (what macOS terminals start)", "interactive shells (what Linux terminals start)"
	if runtime.GOOS != "darwin" {
		login = "login shells (ssh, console)"
	}
	all := []rcFile{
		{"zsh", filepath.Join(zdot, ".zshenv"), "every zsh, scripts included"},
		{"bash", filepath.Join(home, ".bash_profile"), "bash " + login},
		{"bash", filepath.Join(home, ".bashrc"), "bash " + interactive},
		{"fish", filepath.Join(home, ".config", "fish", "conf.d", "keykosh.fish"), "every fish"},
	}
	first := 0
	switch filepath.Base(os.Getenv("SHELL")) {
	case "bash":
		first = 1
		if runtime.GOOS != "darwin" {
			first = 2
		}
	case "fish":
		first = 3
	}
	return append([]rcFile{all[first]}, append(all[:first:first], all[first+1:]...)...)
}

func shellQuote(s string) string { return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"` }

// lineFor is the line that sets the default in that shell's syntax.
func lineFor(shell, root string) string {
	if shell == "fish" {
		return "set -gx " + rootEnv + " " + shellQuote(root)
	}
	return "export " + rootEnv + "=" + shellQuote(root)
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
	keep = append(keep, line+"  "+marker)
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	return writeFile(file, []byte(strings.Join(keep, "\n")+"\n"), mode)
}

// printLines shows the default for every place it can go, to copy by hand.
func printLines(root string) {
	fmt.Fprintf(os.Stderr, `
To set the default kosh yourself, add one of these where you need it:

  zsh, bash, sh      %s
  fish               %s
  launchd, systemd,  set %s=%s in the service's own environment
  amarit, CI         (services do not read shell startup files)
`, lineFor("zsh", root), lineFor("fish", root), rootEnv, root)
}

// offerDefault makes root the default kosh. On a terminal it offers the
// startup files, the login shell's first; with --yes it writes the login
// shell's file; otherwise it prints the lines. always offers even when
// KEYKOSH_ROOT already points here (keykosh default).
func offerDefault(root string, f flags, always bool) {
	if f.noShell || (!always && os.Getenv(rootEnv) == root) {
		return
	}
	files := rcFiles()
	if len(files) == 0 {
		printLines(root)
		return
	}
	var chosen []rcFile
	switch {
	case f.yes:
		chosen = files[:1]
	case !interactive():
		printLines(root)
		say("or run keykosh default --yes to write it to %s", tilde(files[0].path))
		return
	default:
		fmt.Fprintf(os.Stderr, "\nMake %s your default kosh? keykosh can add the %s line to:\n\n", tilde(root), rootEnv)
		for i, rc := range files {
			mark := ""
			if i == 0 {
				mark = "  (your shell)"
			}
			fmt.Fprintf(os.Stderr, "  %d. %-38s %s%s\n", i+1, tilde(rc.path), rc.note, mark)
		}
		fmt.Fprintf(os.Stderr, "  0. none, I'll add it myself\n\n")
		chosen = choose(files)
		if chosen == nil {
			printLines(root)
			return
		}
	}
	for _, rc := range chosen {
		if err := setShellDefault(rc.path, lineFor(rc.shell, root)); err != nil {
			say("could not write %s: %v", tilde(rc.path), err)
			continue
		}
		say("added the default to %s", tilde(rc.path))
	}
	say("open a new terminal, or run: source %s", tilde(chosen[0].path))
}

// choose reads "1", "1,3", "0" or an empty line (the first file).
func choose(files []rcFile) []rcFile {
	fmt.Fprint(os.Stderr, "Choose one or more, like 1,3 [1]: ")
	line, _ := stdin.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return files[:1]
	}
	var out []rcFile
	for _, p := range strings.FieldsFunc(line, func(r rune) bool { return r == ',' || r == ' ' }) {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || n > len(files) {
			say("%q is not on the list; nothing written", p)
			return nil
		}
		if n == 0 {
			return nil
		}
		out = append(out, files[n-1])
	}
	return out
}

func tilde(p string) string {
	if home, err := os.UserHomeDir(); err == nil && (p == home || strings.HasPrefix(p, home+"/")) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}
