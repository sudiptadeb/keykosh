# Changelog

## v0.2.0

- `init` ends on a short guide: next steps, how repos read keys
  (`$KEYKOSH_ROOT/<repo>`, else `<repo>/.keys`), and where to learn more.
- `keykosh prompt [repo]` prints [docs/repo-guide.md](docs/repo-guide.md)
  with the kosh and the repo filled in, to paste to a coding agent.
- Setting the default kosh offers zsh, bash (login and interactive) and fish
  startup files, several at once, and prints the lines for everything else.
  `keykosh default` offers it again.
- `status` says what to run next; `seal`, `unlock` and `lock` say when there
  is nothing to do.

## v0.1.0

- First release: `init`, `unlock`, `seal`, `lock`, `status`, `rm`, `passwd`,
  `update`. Keys are plain age files; the kosh's `.gitignore` commits sealed
  files only; `KEYKOSH_ROOT` is offered to the login shell's startup file.
- Every run says when a newer release is out, checking at most every 15
  minutes; `keykosh update` applies it through amarit.
