// keykosh keeps your keys in a folder, sealed with one passphrase, so the
// folder can live in a private git repo. See README.md.
package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sudiptadeb/amarit"
	"golang.org/x/term"
)

const usage = `keykosh keeps your keys in a folder, sealed with one passphrase.

  keykosh init [dir]     make dir (default: here) a kosh
  keykosh unlock         lay every key out as a plain file
  keykosh seal           seal new and changed keys, keep the plain files
  keykosh lock           seal, then remove the plain files
  keykosh status         show each key's state and what to run next
  keykosh rm <name>      delete a key from the kosh
  keykosh passwd         change the passphrase
  keykosh default        make this kosh the default (KEYKOSH_ROOT)
  keykosh prompt [repo]  print a guide for a coding agent to move a repo's
                         keys into the kosh
  keykosh update         update keykosh itself (every run checks, at most
                         every 15 minutes, and only tells you)
  keykosh version

Which kosh: -C <dir>, else this folder if it has .kosh/, else $KEYKOSH_ROOT.

  -C <dir>     use the kosh in dir
  --yes        write the default to your shell's startup file without asking
  --no-shell   never touch shell startup files

The passphrase is read from the terminal, or from the first line of stdin
when stdin is not a terminal.

Learn more: https://github.com/sudiptadeb/keykosh
`

type flags struct {
	dir     string
	yes     bool
	noShell bool
}

func main() {
	// Updates are explicit: a CLI that runs for a second has no use for
	// amarit's hourly loop, which AMARIT_MANAGED (inherited from any
	// shell amarit started) would otherwise switch on.
	if len(os.Args) > 1 && os.Args[1] == "update" {
		os.Remove(filepath.Join(homeDir(), "latest")) // the next run checks afresh
		amarit.Start()
	}
	noticeUpdate()
	f, args, err := parseFlags(os.Args[1:])
	if err != nil {
		fail(err)
	}
	if len(args) == 0 {
		fmt.Print(usage)
		return
	}
	cmd, args := args[0], args[1:]
	switch cmd {
	case "help", "-h", "--help":
		fmt.Print(usage)
	case "version", "-version", "--version":
		v := strings.TrimPrefix(amarit.BuildVersion(), "v")
		if v == "" {
			v = "dev"
		}
		fmt.Println("keykosh", v)
	case "init":
		runInit(f, args)
	case "unlock":
		noArgs(cmd, args)
		k := find(f)
		res, err := k.Unlock(passphrase("Passphrase: "))
		if err != nil {
			fail(err)
		}
		report(res, "unlock")
		offerDefault(k.Root, f, false)
		exitFor(res)
	case "seal":
		noArgs(cmd, args)
		k := find(f)
		res, err := k.Seal()
		if err != nil {
			fail(err)
		}
		report(res, "seal")
		if len(res.Done) > 0 {
			say("commit them: cd %s && git add -A && git commit -m keys", tilde(k.Root))
		}
		exitFor(res)
	case "lock":
		noArgs(cmd, args)
		res, err := find(f).Lock()
		if err != nil {
			fail(err)
		}
		report(res, "lock")
		exitFor(res)
	case "status":
		noArgs(cmd, args)
		k := find(f)
		items, err := k.Status()
		if err != nil {
			fail(err)
		}
		say("kosh %s", tilde(k.Root))
		for _, it := range items {
			fmt.Printf("%-9s %s\n", it.Status, it.Name)
		}
		hints(k, items)
	case "rm":
		if len(args) == 0 {
			fail(errors.New("rm: name a key"))
		}
		k := find(f)
		for _, n := range args {
			if err := k.Remove(n); err != nil {
				fail(err)
			}
			say("removed %s", n)
		}
	case "default":
		noArgs(cmd, args)
		offerDefault(find(f).Root, f, true)
	case "prompt":
		runPrompt(f, args)
	case "passwd":
		noArgs(cmd, args)
		k := find(f)
		old := passphrase("Current passphrase: ")
		if _, err := k.identity(old); err != nil {
			fail(err)
		}
		if err := k.Passwd(old, newPassphrase()); err != nil {
			fail(err)
		}
		say("passphrase changed; commit .kosh/identity.age")
	default:
		fail(fmt.Errorf("unknown command %q; run keykosh help", cmd))
	}
}

func runInit(f flags, args []string) {
	dir := "."
	switch {
	case len(args) > 1:
		fail(errors.New("init: one folder at most"))
	case len(args) == 1:
		dir = args[0]
	case f.dir != "":
		dir = f.dir
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		fail(err)
	}
	if IsKosh(dir) {
		fail(fmt.Errorf("%s is already a kosh", dir))
	}
	k, err := Init(dir, newPassphrase())
	if err != nil {
		fail(err)
	}
	offerDefault(k.Root, f, false)
	afterInit(k)
}

func parseFlags(in []string) (flags, []string, error) {
	var f flags
	var rest []string
	for i := 0; i < len(in); i++ {
		switch a := in[i]; {
		case a == "-C":
			if i+1 == len(in) {
				return f, nil, errors.New("-C needs a folder")
			}
			i++
			f.dir = in[i]
		case strings.HasPrefix(a, "-C="):
			f.dir = strings.TrimPrefix(a, "-C=")
		case a == "--yes" || a == "-y":
			f.yes = true
		case a == "--no-shell":
			f.noShell = true
		default:
			rest = append(rest, a)
		}
	}
	return f, rest, nil
}

// find picks the kosh: -C, else this folder, else $KEYKOSH_ROOT. It never
// looks in parent folders.
func find(f flags) *Kosh {
	var k *Kosh
	var err error
	switch {
	case f.dir != "":
		k, err = Open(f.dir)
	case IsKosh("."):
		k, err = Open(".")
	case os.Getenv(rootEnv) != "":
		k, err = Open(os.Getenv(rootEnv))
		if err != nil {
			err = fmt.Errorf("%s=%s: %w", rootEnv, os.Getenv(rootEnv), err)
		}
	default:
		err = fmt.Errorf("this folder is not a kosh and %s is not set; cd into a kosh, pass -C <dir>, or run keykosh init", rootEnv)
	}
	if err != nil {
		fail(err)
	}
	return k
}

var stdin = bufio.NewReader(os.Stdin)

func interactive() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stderr.Fd()))
}

func passphrase(prompt string) string {
	var p string
	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprint(os.Stderr, prompt)
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			fail(err)
		}
		p = string(b)
	} else {
		line, err := stdin.ReadString('\n')
		if err != nil && line == "" {
			fail(errors.New("no passphrase on stdin"))
		}
		p = strings.TrimRight(line, "\r\n")
	}
	if p == "" {
		fail(errors.New("empty passphrase"))
	}
	return p
}

// newPassphrase asks twice on a terminal, once on stdin.
func newPassphrase() string {
	p := passphrase("New passphrase: ")
	if term.IsTerminal(int(os.Stdin.Fd())) && passphrase("Again: ") != p {
		fail(errors.New("the passphrases do not match"))
	}
	return p
}

func confirm(q string) bool {
	fmt.Fprintf(os.Stderr, "%s [y/N] ", q)
	line, _ := stdin.ReadString('\n')
	a := strings.ToLower(strings.TrimSpace(line))
	return a == "y" || a == "yes"
}

func report(r *Result, verb string) {
	if len(r.Done) == 0 && len(r.Skipped) == 0 {
		say("nothing to %s", verb)
	}
	for _, it := range r.Done {
		say("%-8s %s", it.Status, it.Name)
	}
	for _, it := range r.Skipped {
		say("SKIPPED  %s: %s", it.Name, it.Status)
	}
}

func exitFor(r *Result) {
	if !r.OK() {
		os.Exit(1)
	}
}

func noArgs(cmd string, args []string) {
	if len(args) > 0 {
		fail(fmt.Errorf("%s takes no arguments", cmd))
	}
}

func say(format string, a ...any) { fmt.Fprintf(os.Stderr, "keykosh: "+format+"\n", a...) }

func fail(err error) {
	say("%v", err)
	os.Exit(1)
}
