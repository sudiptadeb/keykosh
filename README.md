# keykosh

Keep your signing keys and secrets in one folder, sealed with one
passphrase, so the folder can live in a private git repo. On a new machine
you clone it, type the passphrase, and every key is back where it was.

```sh
curl -fsSL https://raw.githubusercontent.com/sudiptadeb/keykosh/main/install.sh | sh
```

## Quick start

```sh
keykosh init ~/mykeykosh            # pick a passphrase; offers to set KEYKOSH_ROOT
mkdir ~/mykeykosh/myapp
cp release.jks ~/mykeykosh/myapp/   # drop keys in, a folder per repo
keykosh seal                        # no passphrase needed
cd ~/mykeykosh && git init && git add -A && git commit -m keys
# push it to a PRIVATE repo
```

On a new machine:

```sh
git clone git@github.com:you/mykeykosh.git ~/mykeykosh
cd ~/mykeykosh && keykosh unlock    # type the passphrase; every key is back
```

`keykosh status` shows each key and tells you what to run next.

## Use it from your repos

A repo's scripts read their keys from `$KEYKOSH_ROOT/<repo>/`, and from
`<repo>/.keys/` (gitignored) where keykosh is not set up. So anyone can build
your project with their own keys, kosh or not:

```sh
KEYS=${KEYKOSH_ROOT:+$KEYKOSH_ROOT/myapp}
KEYS=${KEYS:-$REPO_ROOT/.keys}
jarsigner -keystore "$KEYS/release.jks" ...
```

**Let your coding agent do the move.** In the repo, run

```sh
keykosh prompt | pbcopy
```

and paste it to Claude Code, Codex or any agent. It is
[docs/repo-guide.md](docs/repo-guide.md) with your kosh and the repo's name
filled in: copy the keys into the kosh, seal and commit them, switch the
scripts to the rule above, and prove the build still signs with the same key.

## The idea

A **kosh** is a folder with a `.kosh/` folder in it. It is either locked or
unlocked.

- **Locked:** only sealed files, `<name>.age`. Safe to commit.
- **Unlocked:** each sealed file has its plain key next to it, mode 0600.

```
mykeykosh/
  .gitignore                     written by keykosh: commits sealed files only
  .kosh/recipient                public key: sealing needs only this
  .kosh/identity.age             private key, locked with your passphrase
  myapp/release.jks.age
  myapp/release.jks              appears on unlock, never committed
```

There is no `add`. Drop a file into the kosh, then `seal` or `lock`.
Sealing needs no passphrase, so scripts and agents can add keys to a kosh
they cannot open.

## Commands

| Command | What it does |
|---|---|
| `keykosh init [dir]` | make a folder a kosh and set your passphrase |
| `keykosh unlock` | ask for the passphrase and lay every key out |
| `keykosh seal` | seal new and changed keys, keep the plain files |
| `keykosh lock` | seal, then remove the plain files |
| `keykosh status` | show each key's state and what to run next |
| `keykosh rm <name>` | delete a key from the kosh |
| `keykosh passwd` | change the passphrase |
| `keykosh default` | make this kosh the default (`KEYKOSH_ROOT`) |
| `keykosh prompt [repo]` | print the guide for a coding agent to move a repo's keys in |
| `keykosh update` | update keykosh itself |

## Which kosh a command uses

1. The folder after `-C`.
2. This folder, if it has `.kosh/` directly in it. Parent folders are never
   searched.
3. `$KEYKOSH_ROOT`.

## Setting the default kosh

`init` (and `unlock`, on a machine where `KEYKOSH_ROOT` is not set) offers
to write the default for you. Pick one or more files; your own shell's is
first:

```
Make ~/mykeykosh your default kosh? keykosh can add the KEYKOSH_ROOT line to:

  1. ~/.zshenv                              every zsh, scripts included  (your shell)
  2. ~/.bash_profile                        bash login shells (what macOS terminals start)
  3. ~/.bashrc                              bash interactive shells (what Linux terminals start)
  4. ~/.config/fish/conf.d/keykosh.fish     every fish
  0. none, I'll add it myself

Choose one or more, like 1,3 [1]:
```

Each file gets one line ending in `# keykosh`, replaced on later runs, never
duplicated. Choose `0` and keykosh prints the lines to add yourself:

```
  zsh, bash, sh      export KEYKOSH_ROOT="/Users/you/mykeykosh"
  fish               set -gx KEYKOSH_ROOT "/Users/you/mykeykosh"
  launchd, systemd,  set KEYKOSH_ROOT=/Users/you/mykeykosh in the service's own environment
  amarit, CI         (services do not read shell startup files)
```

`keykosh default` shows the same choice again later. `--yes` writes your
shell's file without asking, and `--no-shell` never writes.

## Key states

| State | Meaning | What fixes it |
|---|---|---|
| `locked` | sealed only | `unlock` |
| `unlocked` | plain file matches the sealed one | nothing |
| `new` | plain file with no sealed copy | `seal` |
| `changed` | plain file edited here | `seal` |
| `stale` | the sealed copy is newer, say after a `git pull` | `unlock` (`lock` also removes it safely) |
| `conflict` | changed here and in the sealed copy | keep one by hand |
| `unknown` | both exist, but this machine never unlocked | `unlock` compares them |

keykosh never overwrites or removes a plain file it cannot prove is already
sealed. A plain `rm` does not delete a key: the next `unlock` brings it back.
`keykosh rm` deletes it, and git history still has it.

## The passphrase

It is read from the terminal, or from the first line of stdin when stdin is
not a terminal (for CI: `printf '%s\n' "$PASS" | keykosh unlock`).

## Security

- **Encryption is [age](https://age-encryption.org).** Keys are sealed to an
  X25519 recipient; the private key is sealed with your passphrase (scrypt).
- **No format of our own.** If keykosh disappears, stock `age` still opens
  everything:
  ```sh
  age -d -o id.txt .kosh/identity.age     # asks for the passphrase
  age -d -i id.txt android/release.jks.age > release.jks
  ```
- **File names are not secret.** Anyone who can read the repo sees
  `android/release.jks.age`, though not what is in it. Keep the repo private.
- **Lock is tidiness, not erasure.** Deleting a file on an SSD does not wipe
  it. The protection is the passphrase and the private repo.
- **The `.gitignore` is a whitelist.** It ignores everything except `.age`
  files and `.kosh/recipient`, so a plain key cannot be committed by
  accident. Don't edit it.
- **`.kosh/state.json` is local.** It holds hashes of what this machine last
  unlocked or sealed, which is how keykosh tells `changed` from `stale`
  without the passphrase. It is never committed.

## Updating

Every run checks for a newer release, at most once every 15 minutes, and
only tells you: `keykosh 0.2.0 is out (you have 0.1.0); run: keykosh update`.
The check gives up after 3 seconds, and its result is cached in
`~/.keykosh/latest`.

`keykosh update` moves to the latest release through
[amarit](https://github.com/sudiptadeb/amarit), verifying the download and
rolling back if the new binary does not start.

## Releasing

`./release.sh v0.1.0` tests, builds every target and publishes the release.
It needs `gh` and the `amarit` CLI.

## License

MIT
