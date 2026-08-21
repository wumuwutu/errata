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

## Shell hook (recommended)

Capture every failing command in your terminal, no `err run` prefix needed:

```sh
# zsh — add to ~/.zshrc:
eval "$(err init zsh)"

# bash (3.2+) — add to ~/.bashrc (macOS login shells: ~/.bash_profile):
eval "$(err init bash)"
```

Or let err append the line for you: `err init zsh --write` (it tells you exactly which
file it touched and never appends twice). fish and other shells are not supported yet —
`err init fish` says so and exits cleanly.

The hook is invisible by design: stderr still reaches your terminal byte-for-byte
(it is tee'd, never intercepted), the success path spawns zero extra processes
(<50ms prompt budget), and a missing `err` binary degrades the hook to a no-op.

## Quick start

```sh
# With the hook installed, errors are captured as they happen.
# Without it, wrap commands explicitly — stdin/stdout/stderr pass through
# untouched (vim and other interactive programs work):
err run python train.py

# When a command fails, the error is fingerprinted and stored.
# Fix it, then record the fix:
err fix -m "LD_LIBRARY_PATH was polluted by conda; conda deactivate and reinstall"

# The next time the same error recurs — even from a different path, with
# different line numbers — a two-line gray hint appears under the traceback
# with the date, directory, occurrence count and your solution.

# List errors you haven't written a solution for, plus your record rate:
err pending

# Record a solution for a specific error (fix always shows the target first):
err fix 3 -m "pin torch==2.1 in requirements.txt"

# Browse everything in a TUI (plain table when piped):
err list

# Distribution, most repeated errors, weekly trend, record rate:
err stats

# Every pit one project put you through, oldest first:
err history --project ~/projects/api

# Self-check (db, hook installation, prompt latency budget):
err doctor

# Precise removal of shell hooks; data kept unless --purge:
err uninstall

# Inspect any recorded error:
err show 3

# Never record certain commands or directories:
err ignore add --command npm
err ignore add --dir ~/work/secrets
err ignore list
err ignore remove --command npm
```

`err run` always exits with the wrapped command's exit code. Pending errors older than
`archive_after_days` (default 30, set in `~/.config/dejavu/config.yaml`) are archived
automatically — never deleted, just out of the pending queue.

When a command succeeds within `success_window_minutes` (default 5) after a pending error
in the same directory, err prints a single gray line — "looks like you just fixed
<signature>? `err fix` to record the solution" — at most once per error per day.

## How it works

- **Capture** — either the shell hook (zsh `preexec`/`precmd`, bash DEBUG trap +
  `PROMPT_COMMAND`) which tees stderr to a per-session buffer and calls the hidden
  `err hook-event` once per failing command, or `err run`, which executes the command
  under a pseudo-terminal ([creack/pty](https://github.com/creack/pty)) with stderr tee'd
  byte-for-byte to your terminal. The scene is captured too: command line, working
  directory, git commit, runtime version, OS.
- **Fingerprint** — ANSI codes are stripped, then volatile parts are normalized to
  placeholders (`/abs/path` → `<PATH>`, `0x…` → `<ADDR>`, numbers → `<N>`, UUIDs /
  timestamps / IPs / quoted values likewise). Python and Node get precise signature
  extraction (Python: last exception line of the traceback, plus the SyntaxError
  family which prints none; Node: the first error line of the stack). Everything else
  falls back to a conservative generic extractor that only trusts unambiguous markers
  (`Exception in thread …`, `panic:`, `fatal:`, `file.c:12: error: …`,
  `command not found`, …) and records the error as language `unknown`. The signature
  is hashed with a self-contained 64-bit SimHash. Identical fingerprints hit directly;
  a Hamming distance ≤ 6 is shown as a degraded "similar error". Precision over recall:
  output with no clear error marker is skipped, never guessed.
- **Storage** — local SQLite ([modernc.org/sqlite](https://gitlab.com/cznic/sqlite),
  pure Go, no CGO) at `~/.local/share/dejavu/dejavu.db` (XDG-aware), with an FTS5
  full-text index over signatures and solutions. Config lives at
  `~/.config/dejavu/config.yaml`.
- **Hints** — on a hit, at most two gray (ANSI 90) lines. Restraint is a feature;
  set `hint.enabled: false` in the config to turn them off.

## Current scope

Deliberately small:

- Precise signatures for Python and Node; a conservative generic fallback records
  other errors as `unknown` (Java/gcc/Go/shell markers). Output with no clear error
  marker is skipped — precision over recall.
- Capture via the shell hook (zsh/bash) and `err run`.
- No LLM features, reports, watch or MCP yet.

Internals for contributors: [docs/architecture.md](docs/architecture.md).

## Fingerprint evaluation

The fingerprint pipeline ships with a labeled-corpus evaluation tool (kept as a
separate binary so it never pollutes the CLI):

```sh
go run ./cmd/err-eval                     # precision/recall/F1 at Hamming thresholds 0..10
go run ./cmd/err-eval -disable val,num    # ablation: disable normalization rules
```

Corpus format and annotation guide: [docs/eval.md](docs/eval.md). Sample corpus:
[eval/corpus.jsonl](eval/corpus.jsonl).

## Layout

```
cmd/err/              main package (binary name: err)
cmd/err-eval/         fingerprint evaluation tool (separate binary)
internal/cli/         cobra commands (run / fix / show / pending / list / stats /
                      history / ignore / init / doctor / uninstall + hidden hook-event)
internal/capture/     PTY passthrough executor + scene capture
internal/hooks/       embedded zsh/bash hook scripts + rc writer
internal/fingerprint/ ANSI strip, normalization, signature extraction, SimHash
internal/match/       the Matcher interface (SimHash today, embedding later)
internal/list/        err list TUI model (pure, unit-tested update logic)
internal/eval/        corpus loading + pairwise precision/recall evaluation
internal/store/       SQLite (errors / fixes / pending + FTS5)
internal/hint/        the restrained gray hit hint
internal/config/      viper config + ignore blacklist
scripts/              shell hook integration tests (driven by go test)
```

## Development

```sh
go build ./...
go test ./...    # includes real bash/zsh hook integration tests (skipped if missing)
go vet ./...
```
