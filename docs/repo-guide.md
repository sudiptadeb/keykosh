# Move this repo's keys into keykosh

This repo's signing keys and secrets are moving into a **kosh**: a folder of
keys sealed with one passphrase, kept in its own private git repo by
[keykosh](https://github.com/sudiptadeb/keykosh). `KEYKOSH_ROOT` points at
the kosh, and on this machine it is unlocked: each plain key sits next to its
sealed `<name>.age` copy.

The kosh for this machine: `{{root}}`. This repo's folder in it: `{{repo}}/`.

## The rule for every script that reads a key

- **Key folder:** `$KEYKOSH_ROOT/{{repo}}` when `KEYKOSH_ROOT` is set, else
  `.keys/` at the root of this repo. File names are the same either way, so
  someone without keykosh puts their own keys in `.keys/`.
  ```sh
  KEYS=${KEYKOSH_ROOT:+$KEYKOSH_ROOT/{{repo}}}
  KEYS=${KEYS:-$REPO_ROOT/.keys}
  ```
- **Add `.keys/` to this repo's `.gitignore`.**
- **A missing key stops the script** with the full path it looked for, plus
  "run keykosh unlock" when `KEYKOSH_ROOT` is set. Never fall back from a set
  `KEYKOSH_ROOT` to `.keys/`, and never generate a new key silently: a new
  signing key breaks every existing install.
- **CI keeps its own secrets.** Change workflows only where they read a
  local path.
- **Write the rule once in the README** and point to it elsewhere.

## Steps

1. List the keys this repo uses, where each one is now, and which scripts
   read it.
2. Copy each into `$KEYKOSH_ROOT/{{repo}}/` with `cp -p`, which keeps mode
   0600. Leave the originals where they are and list them in your report,
   so a person deletes them once everything works.
3. Run `keykosh seal`, then commit in the kosh:
   ```sh
   cd "$KEYKOSH_ROOT"
   git pull --rebase
   git add {{repo}}/
   git status          # only .age files should be staged
   git commit -m "{{repo}}: add keys"
   git push
   ```
   Other repos commit to the kosh too, so add only this repo's folder.
4. Change the scripts and docs to the rule above, and remove the old
   hard-coded paths.
5. Prove it: run the real signing or build step reading from the kosh, and
   show it uses the same key as before, for example a matching certificate
   SHA-1 or keystore fingerprint. Print fingerprints, never key material.
6. Commit in this repo the way it usually takes changes.
7. Report the keys copied, the kosh commit, the scripts changed, the
   verification, and the originals left to delete.

## Don't

- Print key material or a passphrase.
- Run `keykosh lock`: other tools read the plain files.
- Ask for the passphrase in chat. If a key is missing from an unlocked
  kosh, say so and stop.
