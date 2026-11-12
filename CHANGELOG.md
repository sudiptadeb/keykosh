# Changelog

## v0.1.0

- First release: `init`, `unlock`, `seal`, `lock`, `status`, `rm`, `passwd`,
  `update`. Keys are plain age files; the kosh's `.gitignore` commits sealed
  files only; `KEYKOSH_ROOT` is offered to the login shell's startup file.
- Every run says when a newer release is out, checking at most every 15
  minutes; `keykosh update` applies it through amarit.
