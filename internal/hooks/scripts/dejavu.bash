# dejavu shell hook for bash (3.2+) — eval "$(err init bash)"
#
# Same design as the zsh hook: stderr is tee'd (passthrough + recording),
# the DEBUG trap snapshots the buffer offset and command line once per
# command line, PROMPT_COMMAND checks $? and calls `err hook-event` at
# most once, only on failure with fresh stderr.
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
# If the err binary is missing, everything below degrades to a no-op.

command -v err >/dev/null 2>&1 || return 0

# Marker for err doctor (hook loaded in this shell).
export DEJAVU_HOOK=bash

__dejavu_dir="${XDG_RUNTIME_DIR:-${TMPDIR:-/tmp}}/dejavu-$(id -u 2>/dev/null || echo u)"
mkdir -p "$__dejavu_dir" 2>/dev/null && chmod 700 "$__dejavu_dir" 2>/dev/null
__dejavu_sess="$__dejavu_dir/sess-$$"
: >> "$__dejavu_sess.err" 2>/dev/null

__dejavu_preexec() {
  # Only the first simple command of a line snapshots; the flag is
  # (re)armed by the prompt hook. Also never react to our own prompt hook.
  [ -n "$__dejavu_at_prompt" ] || return 0
  case "$BASH_COMMAND" in
    __dejavu_prompt*) return 0 ;;
  esac
  __dejavu_at_prompt=
  __dejavu_off=$(wc -c < "$__dejavu_sess.err" 2>/dev/null)
  __dejavu_off=${__dejavu_off//[[:space:]]/}
  __dejavu_off=${__dejavu_off:-0}
  # Prefer the full command line from history; fall back to BASH_COMMAND
  # (non-interactive shells, history disabled).
  __dejavu_cmd=$(HISTTIMEFORMAT= builtin history 1 2>/dev/null)
  __dejavu_cmd=${__dejavu_cmd#*[0-9]  }
  [ -n "$__dejavu_cmd" ] || __dejavu_cmd="$BASH_COMMAND"
  # Command-boundary sentinel (invisible OSC escape; see header comment).
  __dejavu_seq=$(( ${__dejavu_seq:-0} + 1 ))
  printf '\033]6973;dejavu;%s\007' "$__dejavu_seq" >&2
}

__dejavu_prompt() {
  local ec=$?
  __dejavu_at_prompt=1
  # Consume the command line unconditionally: an empty line or Ctrl-C
  # reuses $?, so a stale value here would attribute old stderr to a
  # command that never failed.
  local cmd="$__dejavu_cmd"
  __dejavu_cmd=
  command -v err >/dev/null 2>&1 || return "$ec"
  [ -n "$cmd" ] || return "$ec"
  if [ "$ec" -eq 0 ]; then
    # Success: maybe a pending error just got fixed (dev-guide 7.2
    # DETECTED_SUCCESS). Cheap gate: no database file => nothing pending
    # => no subprocess, the prompt path stays at zero cost.
    [ -f "${XDG_DATA_HOME:-$HOME/.local/share}/dejavu/dejavu.db" ] || return "$ec"
    err hook-event --exit-code 0 --cwd "$PWD" --command "$cmd" 2>/dev/null
    return "$ec"
  fi
  local size
  size=$(wc -c < "$__dejavu_sess.err" 2>/dev/null)
  size=${size//[[:space:]]/}
  size=${size:-0}
  if ! [ "$size" -gt "${__dejavu_off:-0}" ] 2>/dev/null; then
    # The tee subprocess may lag a few ms behind the command; grant one
    # short grace period on the error path before deciding "no stderr".
    sleep 0.05
    size=$(wc -c < "$__dejavu_sess.err" 2>/dev/null)
    size=${size//[[:space:]]/}
    size=${size:-0}
  fi
  [ "$size" -gt "${__dejavu_off:-0}" ] 2>/dev/null || return "$ec"
  err hook-event --exit-code "$ec" --offset "${__dejavu_off:-0}" \
    --seq "${__dejavu_seq:-0}" \
    --stderr-file "$__dejavu_sess.err" --cwd "$PWD" --command "$cmd" 2>/dev/null
  return "$ec"
}

trap '__dejavu_preexec' DEBUG
PROMPT_COMMAND="__dejavu_prompt${PROMPT_COMMAND:+;$PROMPT_COMMAND}"

# stderr diversion last, so hook installation noise is not recorded.
exec 2> >(tee -a "$__dejavu_sess.err" >&2)
