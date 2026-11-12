package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const pass = "correct horse"

func newKosh(t *testing.T) *Kosh {
	t.Helper()
	k, err := Init(t.TempDir(), pass)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func put(t *testing.T, k *Kosh, name, body string) {
	t.Helper()
	p := k.path(name)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, k *Kosh, name string) string {
	t.Helper()
	b, err := os.ReadFile(k.path(name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func statusOf(t *testing.T, k *Kosh) map[string]string {
	t.Helper()
	items, err := k.Status()
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]string{}
	for _, it := range items {
		m[it.Name] = it.Status
	}
	return m
}

func must(t *testing.T) func(*Result, error) *Result {
	return func(r *Result, err error) *Result {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
}

func TestLockUnlockRoundTrip(t *testing.T) {
	k := newKosh(t)
	put(t, k, "android/release.jks", "jks-bytes")
	put(t, k, "lab.env", "A=1\n")

	must(t)(k.Lock())
	if _, err := os.Stat(k.path("lab.env")); !os.IsNotExist(err) {
		t.Fatal("lock left the plain file")
	}
	if _, err := os.Stat(k.path("android")); err != nil {
		t.Fatal("lock removed a folder that still holds a sealed key")
	}
	if got := statusOf(t, k); got["lab.env"] != Locked || got["android/release.jks"] != Locked {
		t.Fatalf("after lock: %v", got)
	}

	must(t)(k.Unlock(pass))
	if read(t, k, "android/release.jks") != "jks-bytes" || read(t, k, "lab.env") != "A=1\n" {
		t.Fatal("unlock did not restore the keys")
	}
	fi, _ := os.Stat(k.path("lab.env"))
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("plain key mode %v, want 0600", fi.Mode().Perm())
	}
}

func TestWrongPassphrase(t *testing.T) {
	k := newKosh(t)
	put(t, k, "a", "x")
	must(t)(k.Lock())
	if _, err := k.Unlock("nope"); err == nil || !strings.Contains(err.Error(), "wrong passphrase") {
		t.Fatalf("got %v", err)
	}
}

func TestSealOnlyRewritesChangedKeys(t *testing.T) {
	k := newKosh(t)
	put(t, k, "a", "1")
	put(t, k, "b", "1")
	must(t)(k.Seal())
	before, _ := os.ReadFile(k.path("b.age"))

	put(t, k, "a", "2")
	if s := statusOf(t, k)["a"]; s != Changed {
		t.Fatalf("a is %s, want changed", s)
	}
	res := must(t)(k.Seal())
	if len(res.Done) != 1 || res.Done[0].Name != "a" {
		t.Fatalf("sealed %v, want only a", res.Done)
	}
	after, _ := os.ReadFile(k.path("b.age"))
	if string(before) != string(after) {
		t.Fatal("an unchanged key was re-sealed")
	}
}

// A newer sealed copy arrives (a git pull) while this machine is unlocked.
func TestStaleIsRefreshedByUnlock(t *testing.T) {
	k := newKosh(t)
	put(t, k, "a", "old")
	must(t)(k.Seal())
	r, _ := k.recipient()
	newer, _ := encrypt([]byte("new"), r)
	os.WriteFile(k.path("a.age"), newer, 0o644)

	if s := statusOf(t, k)["a"]; s != Stale {
		t.Fatalf("a is %s, want stale", s)
	}
	if res := must(t)(k.Seal()); len(res.Done) != 0 {
		t.Fatal("seal overwrote a newer sealed copy with an old plain file")
	}
	must(t)(k.Unlock(pass))
	if read(t, k, "a") != "new" {
		t.Fatal("unlock did not refresh the stale key")
	}
}

func TestConflictIsLeftAlone(t *testing.T) {
	k := newKosh(t)
	put(t, k, "a", "base")
	must(t)(k.Seal())
	r, _ := k.recipient()
	theirs, _ := encrypt([]byte("theirs"), r)
	os.WriteFile(k.path("a.age"), theirs, 0o644)
	put(t, k, "a", "mine")

	if s := statusOf(t, k)["a"]; s != Conflict {
		t.Fatalf("a is %s, want conflict", s)
	}
	if res := must(t)(k.Lock()); res.OK() {
		t.Fatal("lock reported success on a conflict")
	}
	if res := must(t)(k.Unlock(pass)); res.OK() {
		t.Fatal("unlock reported success on a conflict")
	}
	if read(t, k, "a") != "mine" {
		t.Fatal("a conflicting plain file was overwritten or removed")
	}
}

// A fresh clone that already has plain files lying around: nothing is known
// until unlock compares them.
func TestUnknownNeedsUnlock(t *testing.T) {
	k := newKosh(t)
	put(t, k, "a", "same")
	must(t)(k.Seal())
	os.Remove(k.path(stateFile))

	if s := statusOf(t, k)["a"]; s != Unknown {
		t.Fatalf("a is %s, want unknown", s)
	}
	if res := must(t)(k.Lock()); res.OK() {
		t.Fatal("lock removed a plain file it could not check")
	}
	must(t)(k.Unlock(pass))
	if s := statusOf(t, k)["a"]; s != Unlocked {
		t.Fatalf("after unlock a is %s, want unlocked", s)
	}
}

func TestRemove(t *testing.T) {
	k := newKosh(t)
	put(t, k, "dir/a", "x")
	must(t)(k.Seal())
	if err := k.Remove("dir/a.age"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(k.path("dir")); !os.IsNotExist(err) {
		t.Fatal("rm left the empty folder")
	}
	if err := k.Remove("../x"); err == nil {
		t.Fatal("rm accepted a path outside the kosh")
	}
}

func TestPasswd(t *testing.T) {
	k := newKosh(t)
	put(t, k, "a", "x")
	must(t)(k.Lock())
	if err := k.Passwd(pass, "new pass"); err != nil {
		t.Fatal(err)
	}
	if _, err := k.Unlock(pass); err == nil {
		t.Fatal("the old passphrase still works")
	}
	must(t)(k.Unlock("new pass"))
	if read(t, k, "a") != "x" {
		t.Fatal("keys sealed before passwd did not unlock")
	}
}

// Only sealed files and the public half of .kosh can ever be committed.
func TestGitignoreCommitsOnlySealed(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	k := newKosh(t)
	put(t, k, "dir/key.p12", "secret")
	must(t)(k.Seal())
	git := func(args ...string) string {
		out, err := exec.Command("git", append([]string{"-C", k.Root}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	git("init", "-q")
	git("add", "-A")
	files := strings.Fields(git("ls-files"))
	want := []string{".gitignore", ".kosh/identity.age", ".kosh/recipient", "dir/key.p12.age"}
	if strings.Join(files, " ") != strings.Join(want, " ") {
		t.Fatalf("staged %v, want %v", files, want)
	}
}

func TestShellDefaultReplacesItsLine(t *testing.T) {
	f := filepath.Join(t.TempDir(), ".zshenv")
	os.WriteFile(f, []byte("export PATH=/x\n"), 0o600)
	setShellDefault(f, `export KEYKOSH_ROOT="/a"`)
	setShellDefault(f, `export KEYKOSH_ROOT="/b"`)
	b, _ := os.ReadFile(f)
	if string(b) != "export PATH=/x\nexport KEYKOSH_ROOT=\"/b\"  # keykosh\n" {
		t.Fatalf("got %q", b)
	}
	if fi, _ := os.Stat(f); fi.Mode().Perm() != 0o600 {
		t.Fatal("the rc file's mode changed")
	}
}

func TestPromptFillsRootAndRepo(t *testing.T) {
	if strings.Contains(repoGuide, "{{") == false {
		t.Fatal("the guide has no placeholders")
	}
	out := strings.NewReplacer("{{root}}", "~/k", "{{repo}}", "app").Replace(repoGuide)
	if strings.Contains(out, "{{") || !strings.Contains(out, "$KEYKOSH_ROOT/app") {
		t.Fatal("placeholders left in the guide")
	}
}

// Running init on an existing kosh writes nothing.
func TestInitAgainChangesNothing(t *testing.T) {
	k := newKosh(t)
	put(t, k, "a", "x")
	must(t)(k.Seal())
	snap := func() map[string]string {
		m := map[string]string{}
		filepath.Walk(k.Root, func(p string, fi os.FileInfo, err error) error {
			if err == nil && !fi.IsDir() {
				b, _ := os.ReadFile(p)
				m[p] = string(b)
			}
			return nil
		})
		return m
	}
	before := snap()
	if _, err := Init(k.Root, "other passphrase"); err == nil {
		t.Fatal("init on a kosh succeeded")
	}
	after := snap()
	if len(before) != len(after) {
		t.Fatal("init again added or removed files")
	}
	for p, b := range before {
		if after[p] != b {
			t.Fatalf("init again changed %s", p)
		}
	}
	must(t)(k.Unlock(pass))
}
