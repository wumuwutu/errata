# err (dejavu)

**A personal memory for terminal errors.** `err` captures failing commands, fingerprints the
error, remembers how *you* fixed it — and hands the fix back the next time the same error
shows up. Local-first: everything stays in a SQLite database on your machine, nothing is
ever uploaded.

## Install

```sh
go install github.com/wumuwutu/dejavu/cmd/err@latest
```

This produces a single `err` binary. Requires Go 1.22+ (built with Go 1.27, CGO-free).

## Quick start

```sh
# Wrap any command. stdin/stdout/stderr pass through untouched
# (vim and other interactive programs work); stderr is recorded on the side.
err run python train.py

# When it fails, the error is fingerprinted and stored. Fix it, then record the fix:
err fix -m "LD_LIBRARY_PATH was polluted by conda; conda deactivate and reinstall"

# The next time the same error recurs — even from a different path, with
# different line numbers — a two-line gray hint appears under the traceback
# with the date, directory, occurrence count and your solution.

# Inspect any recorded error:
err show 3

# Never record certain commands or directories:
err ignore add --command npm
err ignore add --dir ~/work/secrets
err ignore list
err ignore remove --command npm
```

`err run` always exits with the wrapped command's exit code.

## How it works

- **Capture** — `err run` executes the command under a pseudo-terminal
  ([creack/pty](https://github.com/creack/pty)). stderr is tee'd byte-for-byte to your
  terminal (never intercepted, never delayed) and recorded on the side. The scene is
  captured too: command line, working directory, git commit, runtime version, OS.
- **Fingerprint** — ANSI codes are stripped, then volatile parts are normalized to
  placeholders (`/abs/path` → `<PATH>`, `0x…` → `<ADDR>`, numbers → `<N>`, UUIDs /
  timestamps / IPs / quoted values likewise). The error signature (Python: the last
  exception line of the traceback; Node: the first error line of the stack) is hashed
  with a self-contained 64-bit SimHash. Identical fingerprints hit directly; a Hamming
  distance ≤ 6 is shown as a degraded "similar error". Precision over recall:
  unrecognized output is skipped, never guessed.
- **Storage** — local SQLite ([modernc.org/sqlite](https://gitlab.com/cznic/sqlite),
  pure Go, no CGO) at `~/.local/share/dejavu/dejavu.db` (XDG-aware), with an FTS5
  full-text index over signatures and solutions. Config lives at
  `~/.config/dejavu/config.yaml`.
- **Hints** — on a hit, at most two gray (ANSI 90) lines. Restraint is a feature;
  set `hint.enabled: false` in the config to turn them off.

## MVP scope (v0.1.0)

Deliberately small:

- Python and Node errors only (other languages are skipped silently).
- Capture only via `err run` — no shell hooks, no TUI, no LLM features,
  no stats/report/init/doctor/watch/mcp yet.

## Layout

```
cmd/err/              main package (binary name: err)
internal/cli/         cobra commands (run / fix / show / ignore)
internal/capture/     PTY passthrough executor + scene capture
internal/fingerprint/ ANSI strip, normalization, signature extraction, SimHash
internal/store/       SQLite (errors / fixes / pending + FTS5)
internal/hint/        the restrained gray hit hint
internal/config/      viper config + ignore blacklist
```

## Development

```sh
go build ./...
go test ./...
go vet ./...
```
