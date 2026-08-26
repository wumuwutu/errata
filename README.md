# err (errata)

[![test](https://github.com/wumuwutu/errata/actions/workflows/test.yml/badge.svg)](https://github.com/wumuwutu/errata/actions/workflows/test.yml)

**A personal memory for terminal errors.** `err` captures failing commands, fingerprints the
error, remembers how *you* fixed it — and hands the fix back the next time the same error
shows up. Local-first: everything stays in a SQLite database on your machine, nothing is
ever uploaded — and credentials found in error output are masked before it is stored
(see **Redact** below).

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/wumuwutu/errata/main/install.sh | sh
```

The script detects your OS/arch (Linux/macOS × amd64/arm64), downloads the matching
release tarball, verifies its SHA256 against `checksums.txt`, and installs `err` into
`~/.local/bin` (when it exists and is on your PATH) or `/usr/local/bin` (asking sudo
when needed). It is plain POSIX sh and lives in this repo — read it before you pipe it.
Pin a version with `ERR_VERSION=v0.1.14 sh install.sh`.

Alternatives:

```sh
# go install (requires Go 1.22+; the module is CGO-free):
go install -ldflags="-s -w" github.com/wumuwutu/errata/cmd/err@latest
```

`-ldflags="-s -w"` strips the symbol table and DWARF info, shrinking the binary by
~32% (16.7 MB → 11.3 MB) — worth it especially on WSL, where Windows Defender scans
the binary on every exec and startup drops from ~1.5s to ~0.35s. (The release
binaries are already built this way.)

Or download a tarball manually from
[GitHub Releases](https://github.com/wumuwutu/errata/releases) and check it against
`checksums.txt` yourself.

Windows is not supported — the capture engine is Unix-only (shell hooks, PTY). Use WSL.

(While the repository is private: `go install` needs
`go env -w GOPRIVATE=github.com/wumuwutu/*` plus GitHub credentials in git, e.g. via
`gh auth setup-git`; install.sh needs `GITHUB_TOKEN=$(gh auth token)` in the environment.)

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
(it is tee'd, never intercepted), the success path spawns zero extra processes in
sessions that have seen no failure (and one re-check per success within 5 minutes
of the last failure otherwise), and a missing `err` binary degrades the hook to a
no-op.
An invisible sentinel written at command start delimits each command's stderr in the
session buffer, so a slow tee (or output of an `err ...` command you ran in between)
can never attribute one command's error to another.
The hook also appends each command line to a per-session log (`sess-<pid>.cmds`, one
`epoch<TAB>command` line, written with a shell builtin — zero subprocesses on bash ≥ 4.2
and zsh); that log is the data source for the `err fix` solution drafts below, and the
7-day session cleanup covers it.

## Quick start

```sh
# With the hook installed, errors are captured as they happen.
# Without it, wrap commands explicitly — stdin/stdout/stderr pass through
# untouched (vim and other interactive programs work):
err run python train.py

# When a command fails, the error is fingerprinted and stored.
# Fix it, then record the fix (no argument = the error you just hit;
# a one-line summary is shown, then you type the solution):
err fix -m "LD_LIBRARY_PATH was polluted by conda; conda deactivate and reinstall"

# In a hooked shell, plain `err fix` also drafts candidates from what you
# ran between the error and now — type the number to adopt one as-is, or
# anything else for a handwritten solution (draft_enabled: false turns
# this off; err run / piped sessions simply show no drafts):
err fix
# err: fixing #3: ModuleNotFoundError: No module named 'numpy'
#   at ~/proj · last seen 2026-08-26 21:04 · cmd: python3 train.py
#   since the error you ran:
#     1. pip install numpy
# solution> 1

# The next time the same error recurs — even from a different path, with
# different line numbers — a two-line gray hint appears under the traceback
# with the date, directory, occurrence count and your solution:
#   --err-- seen 2026-08-22 in ~/proj (occurrence #2)
#   fix: LD_LIBRARY_PATH was polluted by conda; ... (err show 3 for details)

# List errors you haven't written a solution for, plus your record rate
# (latest 20 by default; --all shows everything):
err pending

# Record a solution for a specific error (fix always shows the target first):
err fix 3 -m "pin torch==2.1 in requirements.txt"

# Browse everything in a TUI with scrolling (plain table when piped):
err list

# Distribution, most repeated errors, weekly trend, record rate:
err stats

# Every pit one project put you through, oldest first:
err history --project ~/projects/api

# Export the whole library as Markdown (grouped by project), e.g. to
# migrate machines or mine your history; read-only:
err export                      # ./errata-export-<date>.md
err export --output ~/backups/  # a directory gets the default file name

# Delete one record, or wipe the whole library (both ask first):
err delete 3
err clear

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

## Watching log streams: `err watch`

Some errors never pass through your shell prompt — they live in logs: a container that
keeps crashing, a long build, a service's log file. `err watch` follows such a stream
and captures every error it recognizes, so the same "seen this before" hints and your
recorded solutions apply there too:

```sh
# Follow a container's logs (note the 2>&1: docker logs writes to stderr):
docker logs -f myapp 2>&1 | err watch

# Tail an existing log file from its current end (history is not replayed):
err watch /var/log/myapp.log

# Watch a build log being written by another process:
tail -f build.log | err watch
```

Stop with Ctrl-C; err prints a one-line summary of how many errors it captured.

How it differs from the shell hook: the hook only records output of commands that
**failed** (non-zero exit code). A stream has no exit codes, so watch records any text
that fingerprints as an error — that is exactly what you want from a log, just know it
is the more eager of the two. The watched stream itself is not echoed.

`err run` always exits with the wrapped command's exit code. Pending errors older than
`archive_after_days` (default 30, set in `~/.config/errata/config.yaml`) are archived
automatically — never deleted, just out of the pending queue.

When a command succeeds within `success_window_minutes` (default 5) after a pending error
in the same directory — and the successful command runs the *same program* as the one that
failed (`python3` counts as `python`) and shares a *target argument* (the script or
subcommand, with flags stripped: `FIXED=1 python3 -u demo7.py` matches a failed
`python demo7.py`, but `python3 other.py` and `python3 -c 'print(1)'` do not;
targetless commands like `pip` fall back to program-only matching) — err prints two
short gray lines:

```
--err-- looks fixed: err #3
err fix to record the solution
```

at most once per error per day. Unrelated successes (`ls`, `vim`, …) never trigger it.

Since v0.1.12 this check only runs within five minutes of a failure seen in the
*current* shell session: a pending error left over from yesterday's terminal no
longer nudges on the first successful command of a new one. That trade buys a
zero-subprocess prompt path for failure-free sessions — on WSL every `err` exec
can cost ~1.5s to a Windows Defender scan. `err fix` is unaffected.

## How it works

- **Capture** — either the shell hook (zsh `preexec`/`precmd`, bash DEBUG trap +
  `PROMPT_COMMAND`) which tees stderr to a per-session buffer and calls the hidden
  `err hook-event` once per failing command, or `err run`, which executes the command
  under a pseudo-terminal ([creack/pty](https://github.com/creack/pty)) with stderr tee'd
  byte-for-byte to your terminal. The scene is captured too: command line, working
  directory, git commit, runtime version, OS.
- **Redact** — before anything is stored, stderr passes a conservative redaction
  layer ([internal/redact](internal/redact/redact.go), one auditable file):
  passwords embedded in URLs (`scheme://user:pass@host`), `key=value` secrets
  (`password`/`token`/`secret`/`api_key`/`authorization` and friends,
  case-insensitive), well-known token prefixes (`ghp_`/`gho_`/`github_pat_`,
  `sk-…`, AWS `AKIA…`, Slack `xox…-`) and JWTs are all masked to `***`. Neither
  the stored sample, nor the signature, nor any hint can carry a credential.
  Everything stays local regardless: nothing is ever uploaded.
- **Fingerprint** — ANSI codes are stripped, then volatile parts are normalized to
  placeholders (`/abs/path` → `<PATH>`, `0x…` → `<ADDR>`, numbers → `<N>`, UUIDs /
  timestamps / IPs / quoted values likewise). Python, Node, Java, Go and C get
  precise signature extraction (Python: last exception line of the traceback, plus
  the SyntaxError family which prints none; Node: the first error line of the
  stack; Java: exception class + message from `Exception in thread …`; Go: the
  `panic:` line or a `file.go:l:c: …` compile error; C: the message of a
  gcc/clang `file.c:l:c: error: …`). Everything else
  falls back to a conservative generic extractor that only trusts unambiguous markers
  (`fatal:`, `command not found`) plus the locale-independent shell-builtin shape
  (`bash: cd: …: <message>` — matched structurally, so a zh_CN message is recorded
  too, and Ubuntu's command-not-found helper), and records the error as language
  `unknown`. The signature
  is hashed with a self-contained 64-bit SimHash. Identical fingerprints hit directly;
  a Hamming distance ≤ 6 is shown as a degraded "similar error". Precision over recall:
  output with no clear error marker is skipped, never guessed.
- **Storage** — local SQLite ([modernc.org/sqlite](https://gitlab.com/cznic/sqlite),
  pure Go, no CGO) at `~/.local/share/errata/errata.db` (XDG-aware), with an FTS5
  full-text index over signatures and solutions. Config lives at
  `~/.config/errata/config.yaml`.
- **Hints** — every errata notice starts with `--err--` and uses faint gray
  (ANSI 90) base text, so it reads as system text next to your own terminal
  output: command names (`err fix` / `err show` / `err pending`) in cyan,
  `looks fixed` in bright green, error numbers and signatures in bright
  white, solutions in green.
  At most two lines per notice; restraint is a feature. Colors honor
  `NO_COLOR`; piped CLI output stays plain. Set `hint.enabled: false` in the
  config to turn the capture-time hints off.

## Current scope

Deliberately small — and staying that way:

- Precise signatures for Python, Node, Java, Go and C; a conservative generic
  fallback records other errors as `unknown` (`fatal:`/`command not found` markers,
  shell builtins in any locale, Ubuntu's command-not-found helper). Output with no
  clear error marker is skipped — precision over recall.
- Capture via the shell hook (zsh/bash), `err run`, and `err watch` for log streams.
- Credentials in error output are masked before anything is stored (see
  **Redact** above); `err ignore` blacklists whole commands/directories.
- Single-user, local-first. **Not planned** (cut by design decision, see
  `docs/dev-guide.md` §15 v0.4): embedding/LLM features (`err why`, weekly
  reports), team sharing. If you need an AI explanation, paste the error into
  your AI tool of choice — errata's job is to remember *your* fix.

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
                      history / ignore / init / doctor / delete / clear / export /
                      watch / uninstall + hidden hook-event)
internal/capture/     PTY passthrough executor + scene capture
internal/hooks/       embedded zsh/bash hook scripts + rc writer
internal/fingerprint/ ANSI strip, normalization, signature extraction (python /
                      node / java / go / c + generic fallback), SimHash
internal/redact/      credential masking before stderr is stored
internal/match/       the Matcher interface (SimHash today, embedding later)
internal/list/        err list TUI model (pure, unit-tested update logic)
internal/eval/        corpus loading + pairwise precision/recall evaluation
internal/store/       SQLite (errors / fixes / pending + FTS5)
internal/hint/        the restrained hit/solved notices
internal/termx/       NO_COLOR-aware ANSI palette + display-width truncation
internal/config/      viper config + ignore blacklist
scripts/              shell hook integration tests (driven by go test)
install.sh            POSIX installer (GitHub Releases + SHA256 verify)
```

## Development

```sh
go build ./...
go test ./...    # includes real bash/zsh hook integration tests (skipped if missing)
go vet ./...
```
