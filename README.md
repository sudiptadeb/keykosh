# keykosh

Keep your signing keys and secrets in one folder, sealed with one
passphrase, so the folder can live in a private git repo. On a new machine
you clone it, type the passphrase, and every key is back where it was.

```sh
curl -fsSL https://raw.githubusercontent.com/sudiptadeb/keykosh/main/install.sh | sh
```

## The idea

A **kosh** is a folder with a `.kosh/` folder in it. It is either locked or
unlocked.

- **Locked:** only sealed files, `<name>.age`. Safe to commit.
- **Unlocked:** each sealed file has its plain key next to it, mode 0600.

```
mykeykosh/
  .gitignore                       written by keykosh: commits sealed files only
  .kosh/recipient                  public key: sealing needs only this
  .kosh/identity.age               private key, locked with your passphrase
  android/release.jks.age
  android/release.jks              appears on unlock, never committed
  lab.env.age
```

## Commands

| Command | What it does |
|---|---|
| `keykosh init [dir]` | make a folder a kosh and set your passphrase |
| `keykosh unlock` | ask for the passphrase and lay every key out |
| `keykosh seal` | seal new and changed keys, keep the plain files |
| `keykosh lock` | seal, then remove the plain files |
| `keykosh status` | show each key's state |
| `keykosh rm <name>` | delete a key from the kosh |
| `keykosh passwd` | change the passphrase |
| `keykosh update` | update keykosh itself |

There is no `add`. Drop a file into the unlocked kosh, then `seal` or `lock`.
Sealing needs no passphrase, so scripts and other people can add keys to a
kosh they cannot open.

## Everyday use

```sh
keykosh init ~/mykeykosh                 # once; offers to set KEYKOSH_ROOT
cp ~/release.jks ~/mykeykosh/android/
keykosh seal
cd ~/mykeykosh && git init && git add -A && git commit -m keys

# a new machine
git clone git@github.com:you/mykeykosh.git ~/mykeykosh
cd ~/mykeykosh && keykosh unlock
```

Your builds read keys from the kosh: `$KEYKOSH_ROOT/android/release.jks`.

## Which kosh a command uses

1. The folder after `-C`.
2. This folder, if it has `.kosh/` directly in it. Parent folders are never
   searched.
3. `$KEYKOSH_ROOT`.

`init` and `unlock` offer to set `KEYKOSH_ROOT` in your login shell's
startup file. They ask first, write one line ending in `# keykosh`, and
replace that line on later runs:

| Shell | File |
|---|---|
| zsh | `~/.zshenv` (`$ZDOTDIR` if set): every zsh reads it, scripts too |
| bash on macOS | `~/.bash_profile`: macOS terminals start login shells |
| bash on Linux | `~/.bashrc` |
| fish | `~/.config/fish/conf.d/keykosh.fish` |

`--yes` writes without asking and `--no-shell` never writes. Services
started by launchd, systemd or amarit do not read these files, so set
`KEYKOSH_ROOT` in the service's own environment.

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
