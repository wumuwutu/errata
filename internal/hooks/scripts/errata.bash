# errata shell hook for bash (3.2+) — eval "$(err init bash)"
#
# Same design as the zsh hook: stderr is tee'd (passthrough + recording),
# the DEBUG trap snapshots the buffer offset and command line once per
# command line, PROMPT_COMMAND checks $? and calls `err hook-event` — on
# failure with fresh stderr, and on the first success after this session
# saw a failure (a pending error may just have been fixed).
#
# Command attribution does NOT rely on the byte offset alone: tee flushes
# asynchronously (and interactive shells write the prompt + input echo to
# stderr too), so bytes from an earlier command can land in the buffer
# after the next command's offset snapshot. To cut the buffer exactly at
# command boundaries, preexec writes an invisible OSC sentinel to stderr
# after snapshotting; it reaches the buffer FIFO-ordered behind any
# straggler bytes, and hook-event only reads what follows it. Keep the
# sentinel format in sync with sentinelPrefix in internal/cli/hook_event.go.
#
# Performance: a successful command's prompt path spawns zero
# subprocesses unless this shell session saw a failure earlier (then
# exactly one err call re-checks for a fixed pending and the gate closes
# again — one success check per failure, hint or no hint).
# If the err binary is missing, everything below degrades to a no-op.

command -v err >/dev/null 2>&1 || return 0

# Marker for err doctor (hook loaded in this shell).
export ERRATA_HOOK=bash

__errata_dir="${XDG_RUNTIME_DIR:-${TMPDIR:-/tmp}}/errata-$(id -u 2>/dev/null || echo u)"
mkdir -p "$__errata_dir" 2>/dev/null && chmod 700 "$__errata_dir" 2>/dev/null
__errata_sess="$__errata_dir/sess-$$"
: >> "$__errata_sess.err" 2>/dev/null

__errata_preexec() {
  # Only the first simple command of a line snapshots; the flag is
  # (re)armed by the prompt hook. Also never react to our own prompt hook.
  [ -n "$__errata_at_prompt" ] || return 0
  case "$BASH_COMMAND" in
    __errata_prompt*) return 0 ;;
  esac
  __errata_at_prompt=
  __errata_off=$(wc -c < "$__errata_sess.err" 2>/dev/null)
  __errata_off=${__errata_off//[[:space:]]/}
  __errata_off=${__errata_off:-0}
  # Prefer the full command line from history; fall back to BASH_COMMAND
  # (non-interactive shells, history disabled).
  __errata_cmd=$(HISTTIMEFORMAT= builtin history 1 2>/dev/null)
  __errata_cmd=${__errata_cmd#*[0-9]  }
  [ -n "$__errata_cmd" ] || __errata_cmd="$BASH_COMMAND"
  # Command-boundary sentinel (invisible OSC escape; see header comment).
  __errata_seq=$(( ${__errata_seq:-0} + 1 ))
  printf '\033]6973;errata;%s\007' "$__errata_seq" >&2
}

__errata_prompt() {
  local ec=$?
  __errata_at_prompt=1
  # Consume the command line unconditionally: an empty line or Ctrl-C
  # reuses $?, so a stale value here would attribute old stderr to a
  # command that never failed.
  local cmd="$__errata_cmd"
  __errata_cmd=
  command -v err >/dev/null 2>&1 || return "$ec"
  [ -n "$cmd" ] || return "$ec"
  if [ "$ec" -eq 0 ]; then
    # Success: maybe a pending error just got fixed (dev-guide 7.2
    # DETECTED_SUCCESS). Cheap gates, in order, so the common prompt path
    # stays at zero subprocesses: this session must have seen a failing
    # command (otherwise nothing pending here can have just been fixed),
    # and the database must exist.
    [ -n "${__errata_saw_failure:-}" ] || return "$ec"
    [ -f "${XDG_DATA_HOME:-$HOME/.local/share}/errata/errata.db" ] || return "$ec"
    # One check per failure: clear the flag whether or not a hint prints.
    __errata_saw_failure=
    err hook-event --exit-code 0 --cwd "$PWD" --command "$cmd" 2>/dev/null
    return "$ec"
  fi
  # Failure: arm the success gate, then record as before.
  __errata_saw_failure=1
  local size
  size=$(wc -c < "$__errata_sess.err" 2>/dev/null)
  size=${size//[[:space:]]/}
  size=${size:-0}
  if ! [ "$size" -gt "${__errata_off:-0}" ] 2>/dev/null; then
    # The tee subprocess may lag a few ms behind the command; grant one
    # short grace period on the error path before deciding "no stderr".
    sleep 0.05
    size=$(wc -c < "$__errata_sess.err" 2>/dev/null)
    size=${size//[[:space:]]/}
    size=${size:-0}
  fi
  [ "$size" -gt "${__errata_off:-0}" ] 2>/dev/null || return "$ec"
  err hook-event --exit-code "$ec" --offset "${__errata_off:-0}" \
    --seq "${__errata_seq:-0}" \
    --stderr-file "$__errata_sess.err" --cwd "$PWD" --command "$cmd" 2>/dev/null
  return "$ec"
}

trap '__errata_preexec' DEBUG
PROMPT_COMMAND="__errata_prompt${PROMPT_COMMAND:+;$PROMPT_COMMAND}"

# stderr diversion last, so hook installation noise is not recorded.
exec 2> >(tee -a "$__errata_sess.err" >&2)
