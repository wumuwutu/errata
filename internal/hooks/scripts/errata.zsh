# errata shell hook for zsh — eval "$(err init zsh)"
#
# How it works:
#   - stderr is diverted through tee: it still reaches your terminal
#     byte-for-byte (never intercepted, never delayed) AND is appended to a
#     per-session buffer.
#   - preexec snapshots the buffer offset and the full command line, then
#     writes an invisible OSC sentinel to stderr.
#   - precmd reads $?: on failure with fresh stderr a single
#     `err hook-event` call records the error; on success within 5 minutes
#     of a failure this session saw, one call checks whether a pending
#     error just got fixed.
#   - preexec also appends the command line to a per-session log
#     (sess-$$.cmds, one `epoch<TAB>command` line per command) so
#     `err fix` can draft a solution from what you ran between the error
#     and the fix (dev-guide §7.3). A single builtin printf, no subprocess.
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
# Performance: a successful command's prompt path spawns zero
# subprocesses unless this shell session saw a failure in the last 5
# minutes (then each success pays one err exec re-checking for a fixed
# pending — exactly when a nudge is plausible); the failure path runs at
# most one `err` invocation.
# If the err binary is missing, everything below degrades to a no-op.

command -v err >/dev/null 2>&1 || return 0

# Marker for err doctor (hook loaded in this shell).
export ERRATA_HOOK=zsh

# Session id consumed by `err fix` to find this shell's command log.
export ERRATA_SESSION=$$

# EPOCHSECONDS for the command log (builtin, no subprocess per command).
zmodload zsh/datetime 2>/dev/null

__errata_dir="${XDG_RUNTIME_DIR:-${TMPDIR:-/tmp}}/errata-$(id -u 2>/dev/null || echo u)"
mkdir -p "$__errata_dir" 2>/dev/null && chmod 700 "$__errata_dir" 2>/dev/null
__errata_sess="$__errata_dir/sess-$$"
: >> "$__errata_sess.err" 2>/dev/null

__errata_preexec() {
  __errata_off=$(wc -c < "$__errata_sess.err" 2>/dev/null)
  __errata_off=${__errata_off//[[:space:]]/}
  __errata_off=${__errata_off:-0}
  __errata_cmd="$1"
  # Command log for `err fix` drafts (dev-guide §7.3): epoch<TAB>command.
  printf '%s\t%s\n' "${EPOCHSECONDS:-0}" "$__errata_cmd" >> "$__errata_sess.cmds" 2>/dev/null
  # Command-boundary sentinel (invisible OSC escape; see header comment).
  __errata_seq=$(( ${__errata_seq:-0} + 1 ))
  printf '\033]6973;errata;%s\007' "$__errata_seq" >&2
}
__errata_precmd() {
  local ec=$?
  # Consume the command line unconditionally: an empty line or Ctrl-C
  # reuses $?, so a stale value here would attribute old stderr to a
  # command that never failed.
  local cmd="$__errata_cmd"
  __errata_cmd=""
  command -v err >/dev/null 2>&1 || return 0
  [[ -n "$cmd" ]] || return 0
  if (( ec == 0 )); then
    # Success: maybe a pending error just got fixed (dev-guide 7.2
    # DETECTED_SUCCESS). Cheap gates, in order, so the common prompt path
    # stays at zero subprocesses: this session must have seen a failing
    # command within the last 5 minutes (matching the default
    # success_window_minutes; hardcoded here, the Go config is not read),
    # and the database must exist. The window is NOT consumed by a
    # success — vim saving the file must not eat the fixed re-run's
    # nudge; solvedHint's same-program+target matching rejects the rest.
    [[ -n "${__errata_failed_at:-}" ]] || return 0
    if (( SECONDS - __errata_failed_at > 300 )); then
      __errata_failed_at=""
      return 0
    fi
    [[ -f "${XDG_DATA_HOME:-$HOME/.local/share}/errata/errata.db" ]] || return 0
    err hook-event --exit-code 0 --cwd "$PWD" --command "$cmd" 2>/dev/null
    return 0
  fi
  # Failure: (re)open the success window, then record as before.
  __errata_failed_at=$SECONDS
  local size
  size=$(wc -c < "$__errata_sess.err" 2>/dev/null)
  size=${size//[[:space:]]/}
  size=${size:-0}
  if [[ ! "$size" -gt "${__errata_off:-0}" ]] 2>/dev/null; then
    # The tee subprocess may lag a few ms behind the command; grant one
    # short grace period on the error path before deciding "no stderr".
    sleep 0.05
    size=$(wc -c < "$__errata_sess.err" 2>/dev/null)
    size=${size//[[:space:]]/}
    size=${size:-0}
  fi
  [[ "$size" -gt "${__errata_off:-0}" ]] 2>/dev/null || return 0
  err hook-event --exit-code "$ec" --offset "${__errata_off:-0}" \
    --seq "${__errata_seq:-0}" \
    --stderr-file "$__errata_sess.err" --cwd "$PWD" --command "$cmd" 2>/dev/null
}

autoload -Uz add-zsh-hook
add-zsh-hook preexec __errata_preexec
add-zsh-hook precmd __errata_precmd

# stderr diversion last, so hook installation noise is not recorded.
#
# tee reads a fifo as a disowned background job: background jobs get their
# own process group under job control, so Ctrl-C at the prompt (SIGINT to
# the foreground group) never reaches the recorder. The proc-substitution
# form ( >(tee ...) ) shares the foreground group — tee died to Ctrl-C and
# the next prompt write to the dead pipe could take the shell down with
# SIGPIPE (observed killing interactive bash 5.2 on ubuntu 24.04).
__errata_fifo="$__errata_sess.fifo"
if command -v tee >/dev/null 2>&1 && mkfifo "$__errata_fifo" 2>/dev/null; then
  tee -a "$__errata_sess.err" < "$__errata_fifo" >&2 2>/dev/null &
  disown 2>/dev/null
  exec 2> "$__errata_fifo"
fi
