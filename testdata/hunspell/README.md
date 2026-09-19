# Hunspell conformance fixtures

Copied from the `tests/` directory of [hunspell/hunspell](https://github.com/hunspell/hunspell)
at commit `f06471975def682cf0c7cb291235e2512de7abb5` (2026-09-13). Only the
`.aff`, `.dic`, `.good`, `.wrong`, and `.sug` files are kept.

Hunspell is tri-licensed MPL 1.1 / GPL 2.0+ / LGPL 2.1+ (`license.hunspell`).
These files are used here under the Mozilla Public License 1.1, a copy of which
is in `LICENSE`. They are test data read by `go test` only and are never
compiled into or distributed with a Vale binary. The rest of this repository
remains under its own license.

`internal/spell/hunspell_test.go` runs every fixture: each word in `.good` must be accepted,
and each line in `.wrong` must be rejected. Fixtures the checker does not pass
yet are listed there with the reason, so the list is the scoreboard.
