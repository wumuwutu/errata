# err (errata)

[![test](https://github.com/wumuwutu/errata/actions/workflows/test.yml/badge.svg)](https://github.com/wumuwutu/errata/actions/workflows/test.yml)

**A personal memory for terminal errors.** `err` captures failing commands, fingerprints
the error, remembers how *you* fixed it — and hands the fix back the next time the same
error shows up. Local-first: everything stays in a SQLite database on your machine,
nothing is ever uploaded.

Runs on Linux and macOS (amd64/arm64), with bash 3.2+ or zsh. Windows is not supported
(the capture engine is Unix-only); use WSL. fish is not supported.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/wumuwutu/errata/main/install.sh | sh
```

The script detects your OS/arch, downloads the matching release tarball, verifies its
SHA256, and installs `err` into `~/.local/bin` or `/usr/local/bin`. It is plain POSIX sh
and lives in this repo — read it before you pipe it.

Alternatives:

```sh
# go install (requires Go 1.22+; already stripped via -ldflags="-s -w"):
go install -ldflags="-s -w" github.com/wumuwutu/errata/cmd/err@latest
```

Or grab a tarball from [GitHub Releases](https://github.com/wumuwutu/errata/releases)
and check it against `checksums.txt` yourself.

## Get started

Two steps: install the shell hook, then just work.

```sh
err init zsh --write   # or: err init bash --write  (tells you what file it touched)
exec zsh               # reload your shell
```

From now on every failing command is captured automatically. The first occurrence is
silent; when the same error comes back — even from a different path, with different
line numbers — a two-line gray hint appears under the traceback:

```
--err-- seen 2026-08-22 in ~/proj (occurrence #2)
fix: LD_LIBRARY_PATH was polluted by conda; ... (err show 3 for details)
```

When you fix an error, record the solution. If you fixed it just now, err drafts
candidates from the commands you ran since the error — type a number to adopt one:

```sh
err fix
# err: fixing #3: ModuleNotFoundError: No module named 'numpy'
#   at ~/proj · last seen 2026-08-26 21:04 · cmd: python3 train.py
#   since the error you ran:
#     1. pip install numpy
# solution> 1
```

No hook? Wrap commands explicitly instead — stdin/stdout/stderr pass through untouched,
vim and other interactive programs work inside it:

```sh
err run python train.py
```

## Commands

```sh
err fix [id] [-m "..."]   # record a solution (no arg: the most recent pending error)
err pending               # errors without a solution yet, plus your record rate
err show <id>             # full detail: raw stderr, scene, timeline, solution
err list                  # TUI browser (w/s navigate, a/d filter, e edit inline)
err stats                 # distribution by language/project, weekly trend, record rate
err history --project .   # every pit one project put you through
err export [--output DIR] # dump the library as Markdown (read-only)
err watch [file]          # capture errors from a log stream (see below)
err ignore add --command npm / --dir ~/secrets   # never record these
err delete <id>           # remove one record (asks first)
err clear                 # wipe the whole library (type "yes")
err doctor                # self-check: db, hook, prompt latency budget
err uninstall             # remove the hook lines; data kept unless --purge
```

`err run` always exits with the wrapped command's exit code. Pending errors older than
`archive_after_days` (default 30, in `~/.config/errata/config.yaml`) are archived
automatically — never deleted, just out of the queue.

### Watching log streams

Some errors never pass through your shell prompt — they live in logs. `err watch`
follows a stream and captures every error it recognizes:

```sh
docker logs -f myapp 2>&1 | err watch   # note the 2>&1: docker logs writes to stderr
err watch /var/log/myapp.log            # tails from the current end
tail -f build.log | err watch
```

Stop with Ctrl-C for a one-line capture summary. Unlike the hook (which only records
failing commands), a stream has no exit codes, so watch records any text that
fingerprints as an error — the more eager of the two, by design.

## How it works

- **Capture** — the shell hook (zsh `preexec`/`precmd`, bash DEBUG trap +
  `PROMPT_COMMAND`) tees stderr to a per-session buffer and calls the hidden
  `err hook-event` once per failing command; `err run` wraps a command in a
  pseudo-terminal with stderr tee'd byte-for-byte. The scene is captured too: command
  line, working directory, git commit, runtime version, OS. The prompt path stays at
  zero extra processes in failure-free sessions.
- **Fingerprint** — ANSI codes stripped, volatile parts normalized to placeholders
  (`/abs/path` → `<PATH>`, numbers → `<N>`, UUIDs/timestamps/IPs likewise), then a
  signature is extracted and hashed with a self-contained 64-bit SimHash. Python,
  Node, Java, Go and C get precise extractors; everything else falls back to a
  conservative generic one (language `unknown`) that trusts only unambiguous markers
  and locale-independent shell-builtin shapes. Identical fingerprints hit directly;
  Hamming distance ≤ 6 degrades to a "similar error" hint. Precision over recall:
  output with no clear error marker is skipped, never guessed.
- **Storage** — local SQLite (pure Go, no CGO) at `~/.local/share/errata/errata.db`,
  with an FTS5 index over signatures and solutions. Config at
  `~/.config/errata/config.yaml`.
- **Hints** — every notice starts with `--err--`, at most two lines, faint gray with
  cyan command names and a green solution. Honors `NO_COLOR`; piped output stays plain.
  `hint.enabled: false` turns capture-time hints off.

## Privacy

Before anything is stored, stderr passes a redaction layer
([internal/redact](internal/redact/redact.go), one auditable file): passwords embedded
in URLs, `key=value` secrets (`password`/`token`/`secret`/`api_key` and friends),
well-known token prefixes (`ghp_`, `sk-…`, AWS `AKIA…`, Slack `xox…-`) and JWTs are
masked to `***`. Neither the stored sample nor any hint can carry a credential.
Everything stays local regardless: nothing is ever uploaded.

## Current scope

Deliberately small — and staying that way:

- Python/Node/Java/Go/C precise signatures, conservative generic fallback for the rest.
- Capture via shell hook, `err run`, and `err watch`.
- Single-user, local-first. **Not planned** (cut by design decision): embedding/LLM
  features (`err why`, weekly reports), team sharing. If you need an AI explanation,
  paste the error into your AI tool of choice — errata's job is to remember *your* fix.

## Development

```sh
go build ./...
go test ./...    # includes real bash/zsh hook integration tests (skipped if missing)
go vet ./...
```

The fingerprint pipeline has a labeled-corpus evaluation tool, kept as a separate
binary so it never pollutes the CLI:

```sh
go run ./cmd/err-eval                     # precision/recall/F1 at Hamming thresholds 0..10
go run ./cmd/err-eval -disable val,num    # ablation: disable normalization rules
```
