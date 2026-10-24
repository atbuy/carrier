package shell

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// WriteHookDir writes the shell hooks for shellProgram into a new temporary
// directory. banner prints the one-line session banner at startup.
func WriteHookDir(carrierPath, statePath, shellProgram string, sessionID int64, label, labelColor string, banner bool) (string, error) {
	// The process ID in the name lets CleanTemp tell when the shell is gone.
	dir, err := os.MkdirTemp("", fmt.Sprintf("carrier-shell-%d-*", os.Getpid()))
	if err != nil {
		return "", err
	}
	name := filepath.Base(shellProgram)
	switch {
	case strings.Contains(name, "zsh"):
		return dir, os.WriteFile(filepath.Join(dir, ".zshrc"), []byte(zshHooks(carrierPath, statePath, sessionID, label, labelColor, banner)), 0o600)
	case strings.Contains(name, "bash"):
		return dir, os.WriteFile(filepath.Join(dir, ".bashrc"), []byte(bashHooks(carrierPath, statePath, sessionID, label, labelColor, banner)), 0o600)
	case strings.Contains(name, "fish"):
		return dir, os.WriteFile(filepath.Join(dir, "carrier.fish"), []byte(fishHooks(carrierPath, statePath, sessionID, label, labelColor, banner)), 0o600)
	default:
		return dir, os.WriteFile(filepath.Join(dir, ".zshrc"), []byte(zshHooks(carrierPath, statePath, sessionID, label, labelColor, banner)), 0o600)
	}
}

// startupBanner returns the banner command, or nothing when banner is false.
func startupBanner(banner bool, sessionID int64, label, labelColor string) string {
	if !banner {
		return ""
	}
	return shellBanner(sessionID, label, labelColor)
}

// shellBanner returns a printf command that prints a one-line colored startup
// banner. Works in bash, zsh, and fish (all support printf with ANSI escapes).
func shellBanner(sessionID int64, label, labelColor string) string {
	line := fmt.Sprintf(`%s%s\033[0m`, shellANSIColor(labelColor, true), shellSessionText(sessionID))
	if label != "" {
		line += fmt.Sprintf(`  %s%s\033[0m`, shellANSIColor(labelColor, false), label)
	}
	return `printf '` + line + `\n'`
}

func shellSessionText(sessionID int64) string { return fmt.Sprintf("carrier #%d", sessionID) }

func shellANSIColor(color string, bold bool) string {
	color = strings.TrimSpace(color)
	if color == "" {
		return ""
	}
	weight := ""
	if bold {
		weight = ";1"
	}
	if r, g, b, ok := parseHexColor(color); ok {
		return fmt.Sprintf(`\033[38;2;%d;%d;%d%sm`, r, g, b, weight)
	}
	if n, err := strconv.Atoi(color); err == nil && n >= 0 && n <= 255 {
		return fmt.Sprintf(`\033[38;5;%d%sm`, n, weight)
	}
	if code, ok := ansiColorCode(strings.ToLower(color)); ok {
		if bold {
			return fmt.Sprintf(`\033[%d;1m`, code)
		}
		return fmt.Sprintf(`\033[%dm`, code)
	}
	return ""
}

func parseHexColor(color string) (int64, int64, int64, bool) {
	if len(color) != 7 || color[0] != '#' {
		return 0, 0, 0, false
	}
	r, e1 := strconv.ParseInt(color[1:3], 16, 0)
	g, e2 := strconv.ParseInt(color[3:5], 16, 0)
	b, e3 := strconv.ParseInt(color[5:7], 16, 0)
	return r, g, b, e1 == nil && e2 == nil && e3 == nil
}

func ansiColorCode(name string) (int, bool) {
	codes := map[string]int{
		"black": 30, "red": 31, "green": 32, "yellow": 33, "blue": 34, "magenta": 35, "cyan": 36, "white": 37,
		"gray": 90, "grey": 90, "brightblack": 90, "brightred": 91, "brightgreen": 92, "brightyellow": 93,
		"brightblue": 94, "brightmagenta": 95, "brightcyan": 96, "brightwhite": 97,
	}
	code, ok := codes[name]
	return code, ok
}

func zshColor(color string) string {
	return strings.TrimSpace(color)
}

func fishColor(color string) string {
	color = strings.TrimSpace(color)
	if _, _, _, ok := parseHexColor(color); ok {
		return strings.TrimPrefix(color, "#")
	}
	return color
}

func zshHooks(carrierPath, statePath string, sessionID int64, label, labelColor string, banner bool) string {
	color := zshColor(labelColor)
	return fmt.Sprintf(`
# zsh found this file through ZDOTDIR; give the shell back the user's own, so
# shells started from this one (tmux panes, a nested zsh) load the user's
# config instead of these hooks.
if [[ -n "${CARRIER_ZDOTDIR-}" ]]; then
  ZDOTDIR="$CARRIER_ZDOTDIR"
else
  unset ZDOTDIR
fi
unset CARRIER_ZDOTDIR
[[ -f "${ZDOTDIR:-$HOME}/.zshrc" ]] && source "${ZDOTDIR:-$HOME}/.zshrc"
unsetopt warn_create_global 2>/dev/null
autoload -Uz add-zsh-hook
_carrier_zle_init() {
  unsetopt warn_create_global 2>/dev/null
}
zle -N zle-line-init _carrier_zle_init
_carrier_session_file=%s
_carrier_session_id=%d
_carrier_session_label=%s
_carrier_rprompt=
# powerlevel10k draws its own right prompt and resets RPROMPT before every
# prompt, so the session is shown as a p10k segment there instead.
prompt_carrier() {
  local text="carrier #${_carrier_session_id}"
  [[ -n "$_carrier_session_label" ]] && text+=" ${_carrier_session_label//\%%/%%%%}"
  p10k segment -f %s -t "$text"
}
if (( $+functions[p10k] )) && (( ! ${POWERLEVEL9K_RIGHT_PROMPT_ELEMENTS[(Ie)carrier]} )); then
  POWERLEVEL9K_RIGHT_PROMPT_ELEMENTS=(carrier $POWERLEVEL9K_RIGHT_PROMPT_ELEMENTS)
  p10k reload 2>/dev/null
fi
# Reads the current session, which "carrier attach" changes when it switches
# sessions, and updates the prompt indicator, title, and environment.
_carrier_session() {
  local id label
  { IFS= read -r id; IFS= read -r label } < "$_carrier_session_file" 2>/dev/null
  if [[ -n "$id" ]]; then
    _carrier_session_id="$id"
    _carrier_session_label="$label"
  fi
  export CARRIER_SESSION_ID="$_carrier_session_id" CARRIER_SESSION_LABEL="$_carrier_session_label"
  local indicator="%%B%%F{%s}carrier #${_carrier_session_id}%%f%%b"
  [[ -n "$_carrier_session_label" ]] && indicator+=" %%F{%s}${_carrier_session_label//\%%/%%%%}%%f"
  # Replace the indicator added before, or add it once; precmd runs before
  # every prompt. powerlevel10k shows the prompt_carrier segment instead.
  if (( $+functions[p10k] )); then
    :
  elif [[ -n "$_carrier_rprompt" && "$RPROMPT" == "$_carrier_rprompt"* ]]; then
    RPROMPT="$indicator${RPROMPT#"$_carrier_rprompt"}"
  else
    RPROMPT="$indicator${RPROMPT:+ $RPROMPT}"
  fi
  _carrier_rprompt="$indicator"
  local title="carrier #${_carrier_session_id}"
  [[ -n "$_carrier_session_label" ]] && title+=" · ${_carrier_session_label}"
  printf '\033]0;%%s\007' "$title"
}
_carrier_preexec() {
  [[ -n "$CARRIER_HOOK_ACTIVE" ]] && return
  export CARRIER_HOOK_ACTIVE=1
  command %q internal begin --state %q --cmd "$1" --cwd "$PWD" >/dev/null 2>&1
  unset CARRIER_HOOK_ACTIVE
  _carrier_ran=1
}
_carrier_precmd() {
  local ec="$?"
  [[ -n "$CARRIER_HOOK_ACTIVE" ]] && return
  # precmd also runs at startup and after an empty line; only end a run that
  # preexec began.
  if [[ -n "$_carrier_ran" ]]; then
    _carrier_ran=
    export CARRIER_HOOK_ACTIVE=1
    command %q internal end --state %q --exit "$ec" >/dev/null 2>&1
    unset CARRIER_HOOK_ACTIVE
  fi
  _carrier_session
}
_carrier_zshexit() {
  [[ "$CARRIER_ASK_OUTCOME" == 1 ]] || return
  export CARRIER_HOOK_ACTIVE=1
  command %q internal outcome --state %q
}
add-zsh-hook preexec _carrier_preexec
add-zsh-hook precmd _carrier_precmd
add-zsh-hook zshexit _carrier_zshexit
%s
_carrier_session
`, shellQuote(SessionInfoPath(statePath)), sessionID, shellQuote(label), p10kColor(labelColor), color, color,
		carrierPath, statePath, carrierPath, statePath, carrierPath, statePath, startupBanner(banner, sessionID, label, labelColor))
}

// p10kColor is the label color for "p10k segment -f", which takes a color
// name, number, or #RRGGBB; it falls back to blue.
func p10kColor(color string) string {
	if color = strings.TrimSpace(color); color == "" {
		return "blue"
	}
	return shellQuote(color)
}

func bashHooks(carrierPath, statePath string, sessionID int64, label, labelColor string, banner bool) string {
	// The indicator refers to the session variables instead of holding their
	// values, so PS1 expands them as plain text at each prompt: a label with
	// "$" or "\" in it is shown as is, never run or decoded.
	idIndicator := fmt.Sprintf(`\[%s\]carrier #${_carrier_session_id}\[\033[0m\] `, shellANSIColor(labelColor, true))
	labelIndicator := fmt.Sprintf(`\[%s\]${_carrier_session_label}\[\033[0m\] `, shellANSIColor(labelColor, false))
	// Each command line begins exactly one run:
	//   - PS0 (bash >= 4.4) is expanded once per line, including subshell-only
	//     lines like "(cd x; make)" that never fire DEBUG in this shell. It
	//     begins the run when the line added a history entry, whose text is
	//     the exact command line.
	//   - Otherwise (ignoredups/ignorespace lines, history off, or bash < 4.4)
	//     the first DEBUG trap after a prompt begins it. DEBUG fires before
	//     every simple command, so it is gated by _carrier_at_prompt to keep
	//     "a && b" and "a | b" as one run.
	//     DEBUG also fires before trap actions, with $BASH_COMMAND still
	//     holding the last command: closing the terminal runs the EXIT trap
	//     and would record that command again. With PS0, a run only begins
	//     after a line was read, which PS0 marks by setting _carrier_read
	//     through an assignment that expands to nothing. Bash before 4.4
	//     has no PS0, but its $BASH_COMMAND then holds the last command of
	//     _carrier_end, which is never a command line.
	// Without PS0, a line only ends a run when the DEBUG trap began one:
	// history alone also moves for lines like "(cd x; make)".
	// The trap is installed last so startup commands are not recorded.
	return fmt.Sprintf(`
[[ -f "$HOME/.bashrc" ]] && source "$HOME/.bashrc"
_carrier_session_file=%s
_carrier_session_id=%d
_carrier_session_label=%s
# Reads the current session, which "carrier attach" changes when it switches
# sessions, and updates the prompt indicator, title, and environment.
_carrier_session() {
  local id label
  { IFS= read -r id; IFS= read -r label; } < "$_carrier_session_file" 2>/dev/null
  if [[ -n "$id" ]]; then
    _carrier_session_id="$id"
    _carrier_session_label="$label"
  fi
  export CARRIER_SESSION_ID="$_carrier_session_id" CARRIER_SESSION_LABEL="$_carrier_session_label"
  _carrier_indicator=%s
  [[ -n "$_carrier_session_label" ]] && _carrier_indicator+=%s
  local title="carrier #${_carrier_session_id}"
  [[ -n "$_carrier_session_label" ]] && title+=" · ${_carrier_session_label}"
  printf '\033]0;%%s\007' "$title"
}
_carrier_base_ps1="${PS1}"
_carrier_original_prompt_command="${PROMPT_COMMAND:-}"
_carrier_has_ps0=
(( BASH_VERSINFO[0] > 4 || (BASH_VERSINFO[0] == 4 && BASH_VERSINFO[1] >= 4) )) && _carrier_has_ps0=1
# Sets _carrier_hnum and _carrier_htext from the newest history entry.
_carrier_hist() {
  local line
  line="$(HISTTIMEFORMAT= builtin history 1)"
  _carrier_hnum= _carrier_htext=
  if [[ "$line" =~ ^[[:space:]]*([0-9]+)[[:space:]]+(.*)$ ]]; then
    _carrier_hnum="${BASH_REMATCH[1]}"
    _carrier_htext="${BASH_REMATCH[2]}"
  fi
}
_carrier_run_begin() {
  command %q internal begin --state %q --cmd "$1" --cwd "$PWD" >/dev/null 2>&1
}
# Runs in the PS0 command substitution, so it cannot set variables here.
_carrier_ps0() {
  _carrier_hist
  [[ -n "$_carrier_hnum" && "$_carrier_hnum" != "$_carrier_last_hist" ]] && _carrier_run_begin "$_carrier_htext"
}
_carrier_begin() {
  [[ -n "$_carrier_at_prompt" ]] || return
  [[ -n "$_carrier_has_ps0" && -z "${_carrier_read+set}" ]] && return
  [[ "$BASH_COMMAND" == "trap _carrier_begin DEBUG" ]] && return
  [[ -n "$CARRIER_HOOK_ACTIVE" ]] && return
  [[ -n "${COMP_LINE:-}" ]] && return
  [[ "$BASH_COMMAND" == _carrier_* ]] && return
  _carrier_at_prompt=
  _carrier_hist
  # A new history entry means PS0 already began this run.
  [[ -n "$_carrier_has_ps0" && -n "$_carrier_hnum" && "$_carrier_hnum" != "$_carrier_last_hist" ]] && return
  local cmd="$BASH_COMMAND"
  # With ignoredups the newest entry is an identical earlier line; with
  # ignorespace it is an unrelated one, so fall back to $BASH_COMMAND.
  [[ -n "$_carrier_htext" && "$_carrier_htext" == *"$cmd"* ]] && cmd="$_carrier_htext"
  export CARRIER_HOOK_ACTIVE=1
  _carrier_run_begin "$cmd"
  unset CARRIER_HOOK_ACTIVE
}
_carrier_end() {
  local ec="$?"
  trap - DEBUG
  _carrier_session
  if [[ -n "$_carrier_original_prompt_command" ]]; then
    eval "$_carrier_original_prompt_command"
    PS1="${_carrier_indicator}${PS1}"
  else
    PS1="${_carrier_indicator}${_carrier_base_ps1}"
  fi
  _carrier_hist
  # Skip empty lines: no trap fired and no history entry was added.
  if [[ -z "$_carrier_at_prompt" || ( -n "$_carrier_has_ps0" && "$_carrier_hnum" != "$_carrier_last_hist" ) ]]; then
    export CARRIER_HOOK_ACTIVE=1
    command %q internal end --state %q --exit "$ec" >/dev/null 2>&1
    unset CARRIER_HOOK_ACTIVE
  fi
  _carrier_last_hist="$_carrier_hnum"
  _carrier_at_prompt=1
  unset _carrier_read
  trap _carrier_begin DEBUG
}
_carrier_exit() {
  [[ "$CARRIER_ASK_OUTCOME" == 1 ]] || return
  trap - DEBUG
  export CARRIER_HOOK_ACTIVE=1
  command %q internal outcome --state %q
}
# Leave an EXIT trap from the user's config alone.
[[ -z "$(trap -p EXIT)" ]] && trap _carrier_exit EXIT
%s
_carrier_session
_carrier_hist
_carrier_last_hist="$_carrier_hnum"
_carrier_at_prompt=1
PS0="${PS0:-}"'${_carrier_read=}$(_carrier_ps0)'
PROMPT_COMMAND=_carrier_end
trap _carrier_begin DEBUG
`, shellQuote(SessionInfoPath(statePath)), sessionID, shellQuote(label), shellQuote(idIndicator), shellQuote(labelIndicator),
		carrierPath, statePath, carrierPath, statePath, carrierPath, statePath, startupBanner(banner, sessionID, label, labelColor))
}

func fishHooks(carrierPath, statePath string, sessionID int64, label, labelColor string, banner bool) string {
	color := fishColor(labelColor)
	return fmt.Sprintf(`
set -g _carrier_session_file %s
set -g _carrier_session_id %d
set -g _carrier_session_label %s
# Reads the current session, which "carrier attach" changes when it switches
# sessions, and updates the title and environment; fish_right_prompt shows it.
function _carrier_session --on-event fish_prompt
    if read --local --line id label < $_carrier_session_file 2>/dev/null; and test -n "$id"
        set -g _carrier_session_id $id
        set -g _carrier_session_label $label
    end
    set -gx CARRIER_SESSION_ID $_carrier_session_id
    set -gx CARRIER_SESSION_LABEL $_carrier_session_label
    set -l title "carrier #$_carrier_session_id"
    test -n "$_carrier_session_label"; and set title "$title · $_carrier_session_label"
    printf '\033]0;%%s\007' $title
end
function _carrier_preexec --on-event fish_preexec
    set -q CARRIER_HOOK_ACTIVE; and return
    set -xg CARRIER_HOOK_ACTIVE 1
    command %q internal begin --state %q --cmd "$argv" --cwd "$PWD" >/dev/null 2>&1
    set -eg CARRIER_HOOK_ACTIVE
end
function _carrier_postcmd --on-event fish_postexec
    set -l ec $status
    set -q CARRIER_HOOK_ACTIVE; and return
    set -xg CARRIER_HOOK_ACTIVE 1
    command %q internal end --state %q --exit $ec >/dev/null 2>&1
    set -eg CARRIER_HOOK_ACTIVE
end
function _carrier_exit --on-event fish_exit
    test "$CARRIER_ASK_OUTCOME" = 1; or return
    set -xg CARRIER_HOOK_ACTIVE 1
    command %q internal outcome --state %q
end
if functions -q fish_right_prompt
    functions --copy fish_right_prompt _carrier_orig_right_prompt
else
    function _carrier_orig_right_prompt; end
end
function fish_right_prompt
    set_color --bold %s
    echo -n "carrier #$_carrier_session_id"
    if test -n "$_carrier_session_label"
        set_color normal
        set_color %s
        echo -n " · $_carrier_session_label"
    end
    set_color normal
    set -l _orig (_carrier_orig_right_prompt)
    if test -n "$_orig"
        echo -n " $_orig"
    end
end
%s
_carrier_session
`, shellQuote(SessionInfoPath(statePath)), sessionID, shellQuote(label),
		carrierPath, statePath, carrierPath, statePath, carrierPath, statePath, color, color, startupBanner(banner, sessionID, label, labelColor))
}
