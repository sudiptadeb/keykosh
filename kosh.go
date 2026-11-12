package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"filippo.io/age"
)

// A kosh is a folder with a .kosh/ folder in it. Every key is stored as
// <name>.age, encrypted to the kosh's recipient. Unlocking writes <name>
// next to it; locking removes it again.
const (
	markerDir     = ".kosh"
	recipientFile = ".kosh/recipient"
	identityFile  = ".kosh/identity.age"
	stateFile     = ".kosh/state.json" // local only: the gitignore keeps it out
	sealedExt     = ".age"
)

const gitignore = `# Written by keykosh. Only sealed files are committed; plain keys never are.
*
!*/
!.gitignore
!*.age
!.kosh/recipient
`

type Kosh struct {
	Root string
}

// entry is what this machine last saw for one key: the hashes of the plain
// file and the sealed file when they were last known to match.
type entry struct {
	Plain  string `json:"plain"`
	Sealed string `json:"sealed"`
}

type state map[string]entry

// Status of one key.
const (
	Locked   = "locked"   // sealed only
	Unlocked = "unlocked" // plain matches sealed
	New      = "new"      // plain only: the next seal encrypts it
	Changed  = "changed"  // plain edited here: the next seal re-encrypts it
	Stale    = "stale"    // sealed is newer (a pull): unlock refreshes it
	Conflict = "conflict" // both changed: sort it out by hand
	Unknown  = "unknown"  // both exist but this machine never unlocked: unlock checks it
)

type Item struct {
	Name   string
	Status string
}

func IsKosh(dir string) bool {
	fi, err := os.Stat(filepath.Join(dir, markerDir))
	return err == nil && fi.IsDir()
}

// Init makes dir a kosh locked with passphrase.
func Init(dir, passphrase string) (*Kosh, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if IsKosh(abs) {
		return nil, fmt.Errorf("%s is already a kosh", abs)
	}
	gi := filepath.Join(abs, ".gitignore")
	if b, err := os.ReadFile(gi); err == nil && string(b) != gitignore {
		return nil, fmt.Errorf("%s already has a .gitignore; a kosh needs keykosh's own, which commits only sealed files. Move it aside and run init again", abs)
	}
	id, err := age.GenerateX25519Identity()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(abs, markerDir), 0o700); err != nil {
		return nil, err
	}
	k := &Kosh{Root: abs}
	if err := k.writeIdentity(id, passphrase); err != nil {
		return nil, err
	}
	if err := writeFile(filepath.Join(abs, recipientFile), []byte(id.Recipient().String()+"\n"), 0o644); err != nil {
		return nil, err
	}
	if err := writeFile(gi, []byte(gitignore), 0o644); err != nil {
		return nil, err
	}
	return k, nil
}

func Open(dir string) (*Kosh, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if !IsKosh(abs) {
		return nil, fmt.Errorf("%s is not a kosh (no %s folder)", abs, markerDir)
	}
	return &Kosh{Root: abs}, nil
}

func (k *Kosh) path(rel string) string { return filepath.Join(k.Root, filepath.FromSlash(rel)) }

func (k *Kosh) recipient() (age.Recipient, error) {
	b, err := os.ReadFile(k.path(recipientFile))
	if err != nil {
		return nil, err
	}
	return age.ParseX25519Recipient(strings.TrimSpace(string(b)))
}

func (k *Kosh) writeIdentity(id *age.X25519Identity, passphrase string) error {
	r, err := age.NewScryptRecipient(passphrase)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	w, err := age.Encrypt(&buf, r)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(w, id.String()+"\n"); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return writeFile(k.path(identityFile), buf.Bytes(), 0o644)
}

// identity unlocks the kosh's private key with passphrase.
func (k *Kosh) identity(passphrase string) (*age.X25519Identity, error) {
	f, err := os.Open(k.path(identityFile))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sid, err := age.NewScryptIdentity(passphrase)
	if err != nil {
		return nil, err
	}
	r, err := age.Decrypt(f, sid)
	if err != nil {
		var wrong *age.NoIdentityMatchError
		if errors.As(err, &wrong) || strings.Contains(err.Error(), "incorrect passphrase") {
			return nil, errors.New("wrong passphrase")
		}
		return nil, err
	}
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	id, err := age.ParseX25519Identity(strings.TrimSpace(string(b)))
	if err != nil {
		return nil, err
	}
	want, err := k.recipient()
	if err != nil {
		return nil, err
	}
	if id.Recipient().String() != want.(*age.X25519Recipient).String() {
		return nil, errors.New("the private key does not match .kosh/recipient; the kosh is damaged")
	}
	return id, nil
}

// Passwd re-locks the private key with a new passphrase.
func (k *Kosh) Passwd(old, new string) error {
	id, err := k.identity(old)
	if err != nil {
		return err
	}
	return k.writeIdentity(id, new)
}

func (k *Kosh) loadState() state {
	s := state{}
	if b, err := os.ReadFile(k.path(stateFile)); err == nil {
		_ = json.Unmarshal(b, &s)
	}
	return s
}

func (k *Kosh) saveState(s state) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(k.path(stateFile), append(b, '\n'), 0o600)
}

// skipped names are never treated as keys.
func skipped(name string) bool {
	return name == ".gitignore" || name == ".DS_Store"
}

// scan lists sealed names and plain names, both as slash paths relative to
// the root, the sealed ones without their .age.
func (k *Kosh) scan() (sealed, plain map[string]bool, err error) {
	sealed, plain = map[string]bool{}, map[string]bool{}
	err = filepath.WalkDir(k.Root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(k.Root, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == markerDir || rel == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || skipped(d.Name()) {
			return nil
		}
		if strings.HasSuffix(rel, sealedExt) {
			sealed[strings.TrimSuffix(rel, sealedExt)] = true
		} else {
			plain[rel] = true
		}
		return nil
	})
	return
}

func hashFile(p string) (string, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	return hashBytes(b), nil
}

func hashBytes(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func (k *Kosh) classify(name string, hasSealed, hasPlain bool, s state) (string, error) {
	switch {
	case hasSealed && !hasPlain:
		return Locked, nil
	case !hasSealed && hasPlain:
		return New, nil
	}
	e, known := s[name]
	if !known {
		return Unknown, nil
	}
	ph, err := hashFile(k.path(name))
	if err != nil {
		return "", err
	}
	sh, err := hashFile(k.path(name + sealedExt))
	if err != nil {
		return "", err
	}
	switch {
	case ph == e.Plain && sh == e.Sealed:
		return Unlocked, nil
	case ph != e.Plain && sh == e.Sealed:
		return Changed, nil
	case ph == e.Plain && sh != e.Sealed:
		return Stale, nil
	}
	return Conflict, nil
}

// Status reports every key, sorted by name.
func (k *Kosh) Status() ([]Item, error) {
	sealed, plain, err := k.scan()
	if err != nil {
		return nil, err
	}
	names := map[string]bool{}
	for n := range sealed {
		names[n] = true
	}
	for n := range plain {
		names[n] = true
	}
	s := k.loadState()
	var items []Item
	for n := range names {
		st, err := k.classify(n, sealed[n], plain[n], s)
		if err != nil {
			return nil, err
		}
		items = append(items, Item{n, st})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items, nil
}

// Result of an unlock, seal or lock: what was done to which key, and the
// keys left alone because they need a person.
type Result struct {
	Done    []Item // Status holds the action: "unlocked", "sealed", "removed"
	Skipped []Item // Status holds why
}

func (r *Result) OK() bool { return len(r.Skipped) == 0 }

// Unlock writes every sealed key next to its .age file.
func (k *Kosh) Unlock(passphrase string) (*Result, error) {
	id, err := k.identity(passphrase)
	if err != nil {
		return nil, err
	}
	sealed, plain, err := k.scan()
	if err != nil {
		return nil, err
	}
	s := k.loadState()
	res := &Result{}
	for _, n := range sortedKeys(sealed) {
		sb, err := os.ReadFile(k.path(n + sealedExt))
		if err != nil {
			return nil, err
		}
		pb, err := decrypt(sb, id)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", n+sealedExt, err)
		}
		if plain[n] {
			cur, err := os.ReadFile(k.path(n))
			if err != nil {
				return nil, err
			}
			e, known := s[n]
			switch {
			case bytes.Equal(cur, pb):
				s[n] = entry{hashBytes(pb), hashBytes(sb)}
				continue
			case known && hashBytes(cur) == e.Plain:
				// untouched here, newer sealed copy: refresh below
			default:
				res.Skipped = append(res.Skipped, Item{n, "plain file differs from the sealed copy; seal or delete it, then unlock"})
				continue
			}
		}
		if err := writeFile(k.path(n), pb, 0o600); err != nil {
			return nil, err
		}
		s[n] = entry{hashBytes(pb), hashBytes(sb)}
		res.Done = append(res.Done, Item{n, "unlocked"})
	}
	return res, k.saveState(s)
}

// Seal encrypts every new or changed plain key. It needs no passphrase.
func (k *Kosh) Seal() (*Result, error) {
	r, err := k.recipient()
	if err != nil {
		return nil, err
	}
	items, err := k.Status()
	if err != nil {
		return nil, err
	}
	s := k.loadState()
	res := &Result{}
	for _, it := range items {
		switch it.Status {
		case New, Changed:
			pb, err := os.ReadFile(k.path(it.Name))
			if err != nil {
				return nil, err
			}
			sb, err := encrypt(pb, r)
			if err != nil {
				return nil, err
			}
			if err := writeFile(k.path(it.Name+sealedExt), sb, 0o644); err != nil {
				return nil, err
			}
			s[it.Name] = entry{hashBytes(pb), hashBytes(sb)}
			res.Done = append(res.Done, Item{it.Name, "sealed"})
		case Conflict:
			res.Skipped = append(res.Skipped, Item{it.Name, "changed here and in the sealed copy; keep one by hand"})
		case Unknown:
			res.Skipped = append(res.Skipped, Item{it.Name, "never unlocked on this machine; run unlock first"})
		}
	}
	return res, k.saveState(s)
}

// Lock seals what changed, then removes every plain key whose sealed copy
// holds all of it.
func (k *Kosh) Lock() (*Result, error) {
	res, err := k.Seal()
	if err != nil {
		return nil, err
	}
	items, err := k.Status()
	if err != nil {
		return nil, err
	}
	s := k.loadState()
	for _, it := range items {
		if it.Status != Unlocked && it.Status != Stale {
			continue
		}
		if err := os.Remove(k.path(it.Name)); err != nil {
			return nil, err
		}
		delete(s, it.Name)
		k.pruneDirs(filepath.Dir(k.path(it.Name)))
		res.Done = append(res.Done, Item{it.Name, "removed"})
	}
	return res, k.saveState(s)
}

// Remove deletes a key from the kosh: its sealed copy and its plain file.
func (k *Kosh) Remove(name string) error {
	name = strings.TrimSuffix(filepath.ToSlash(filepath.Clean(name)), sealedExt)
	if strings.HasPrefix(name, "../") || name == ".." || filepath.IsAbs(name) {
		return fmt.Errorf("%s is outside the kosh", name)
	}
	found := false
	for _, p := range []string{k.path(name + sealedExt), k.path(name)} {
		if err := os.Remove(p); err == nil {
			found = true
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	if !found {
		return fmt.Errorf("no key named %s", name)
	}
	s := k.loadState()
	delete(s, name)
	k.pruneDirs(filepath.Dir(k.path(name)))
	return k.saveState(s)
}

// pruneDirs removes empty folders from dir up to, not including, the root.
func (k *Kosh) pruneDirs(dir string) {
	for dir != k.Root && strings.HasPrefix(dir, k.Root+string(filepath.Separator)) {
		if os.Remove(dir) != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}

func encrypt(b []byte, r age.Recipient) ([]byte, error) {
	var buf bytes.Buffer
	w, err := age.Encrypt(&buf, r)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(b); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decrypt(b []byte, id age.Identity) ([]byte, error) {
	r, err := age.Decrypt(bytes.NewReader(b), id)
	if err != nil {
		return nil, err
	}
	return io.ReadAll(r)
}

// writeFile writes atomically: a temp file in the same folder, then rename.
// Folders it creates are private.
func writeFile(p string, b []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(p), ".keykosh-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

func sortedKeys(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
