# dejavu shell hook for zsh — eval "$(err init zsh)"
#
# How it works:
#   - stderr is diverted through tee: it still reaches your terminal
#     byte-for-byte (never intercepted, never delayed) AND is appended to a
#     per-session buffer.
#   - preexec snapshots the buffer offset and the full command line, then
#     writes an invisible OSC sentinel to stderr.
#   - precmd reads $?: if the command failed and the buffer grew, a single
#     `err hook-event` call does the fingerprinting/storage/hint in Go.
#
# Command attribution does NOT rely on the byte offset alone: tee flushes
# asynchronously (and interactive shells write the prompt + input echo to
# stderr too), so bytes from an earlier command can land in the buffer
# after the next command's offset snapshot. The sentinel is written at
# preexec, so it reaches the buffer FIFO-ordered behind any straggler
# bytes, and hook-event only reads what follows it — an earlier command's
# late stderr (or the output of an `err ...` command you ran in between)
# can never leak into the next command's capture. Keep the sentinel format
# in sync with sentinelPrefix in internal/cli/hook_event.go.
#
# Performance: the success path runs zero subprocesses; the failure path
# runs at most one `err` invocation.
# If the err binary is missing, everything below degrades to a no-op.

command -v err >/dev/null 2>&1 || return 0

# Marker for err doctor (hook loaded in this shell).
export DEJAVU_HOOK=zsh

__dejavu_dir="${XDG_RUNTIME_DIR:-${TMPDIR:-/tmp}}/dejavu-$(id -u 2>/dev/null || echo u)"
mkdir -p "$__dejavu_dir" 2>/dev/null && chmod 700 "$__dejavu_dir" 2>/dev/null
__dejavu_sess="$__dejavu_dir/sess-$$"
: >> "$__dejavu_sess.err" 2>/dev/null

__dejavu_preexec() {
  __dejavu_off=$(wc -c < "$__dejavu_sess.err" 2>/dev/null)
  __dejavu_off=${__dejavu_off//[[:space:]]/}
  __dejavu_off=${__dejavu_off:-0}
  __dejavu_cmd="$1"
  # Command-boundary sentinel (invisible OSC escape; see header comment).
  __dejavu_seq=$(( ${__dejavu_seq:-0} + 1 ))
  printf '\033]6973;dejavu;%s\007' "$__dejavu_seq" >&2
}

__dejavu_precmd() {
  local ec=$?
  # Consume the command line unconditionally: an empty line or Ctrl-C
  # reuses $?, so a stale value here would attribute old stderr to a
  # command that never failed.
  local cmd="$__dejavu_cmd"
  __dejavu_cmd=""
  command -v err >/dev/null 2>&1 || return 0
  [[ -n "$cmd" ]] || return 0
  if (( ec == 0 )); then
    # Success: maybe a pending error just got fixed (dev-guide 7.2
    # DETECTED_SUCCESS). Cheap gate: no database file => nothing pending
    # => no subprocess, the prompt path stays at zero cost.
    [[ -f "${XDG_DATA_HOME:-$HOME/.local/share}/dejavu/dejavu.db" ]] || return 0
    err hook-event --exit-code 0 --cwd "$PWD" --command "$cmd" 2>/dev/null
    return 0
  fi
  local size
  size=$(wc -c < "$__dejavu_sess.err" 2>/dev/null)
  size=${size//[[:space:]]/}
  size=${size:-0}
  if [[ ! "$size" -gt "${__dejavu_off:-0}" ]] 2>/dev/null; then
    # The tee subprocess may lag a few ms behind the command; grant one
    # short grace period on the error path before deciding "no stderr".
    sleep 0.05
    size=$(wc -c < "$__dejavu_sess.err" 2>/dev/null)
    size=${size//[[:space:]]/}
    size=${size:-0}
  fi
  [[ "$size" -gt "${__dejavu_off:-0}" ]] 2>/dev/null || return 0
  err hook-event --exit-code "$ec" --offset "${__dejavu_off:-0}" \
    --seq "${__dejavu_seq:-0}" \
    --stderr-file "$__dejavu_sess.err" --cwd "$PWD" --command "$cmd" 2>/dev/null
}

autoload -Uz add-zsh-hook
add-zsh-hook preexec __dejavu_preexec
add-zsh-hook precmd __dejavu_precmd

# stderr diversion last, so hook installation noise is not recorded.
exec 2> >(tee -a "$__dejavu_sess.err" >&2)
