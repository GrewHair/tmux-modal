package daemon

import (
	"strconv"
	"strings"
	"time"

	"github.com/GrewHair/tmux-modal/internal/tmux"
)

// Config is the daemon's view of the @modal_* options.
type Config struct {
	Enabled      bool
	Profile      string // effective: frugal | balanced | snappy | custom
	Burst        time.Duration
	Poll         time.Duration
	Idle         time.Duration
	BurstDecay   time.Duration
	Scope        string // active | visible | all
	CaptureRows  int
	CPUBudget    float64 // % of one core
	SpecPaths    []string
	Leader       string
	Colour       bool
	Hook         string
	HookTimeout  time.Duration
	HookDebounce time.Duration
	LogLevel     string
	Confirm      int // consecutive agreeing captures before remapping resumes
}

type profile struct{ burst, poll, idle int }

var profiles = map[string]profile{
	"frugal":   {100, 500, 5000},
	"balanced": {30, 150, 2000},
	"snappy":   {15, 60, 1000},
}

var optionNames = []string{
	"@modal_enabled", "@modal_profile", "@modal_poll_interval",
	"@modal_idle_interval", "@modal_burst_interval", "@modal_burst_decay",
	"@modal_scope", "@modal_capture_rows", "@modal_cpu_budget",
	"@modal_spec_paths", "@modal_escape_leader", "@modal_color_matching",
	"@modal_transition_hook", "@modal_hook_timeout", "@modal_hook_debounce",
	"@modal_log_level", "@modal_confirm_captures",
}

// configFormat reads every option in one display-message. The separator
// is a tab: display-message escapes non-printable characters (a \x1f comes
// back as the four characters "\037"), but passes tabs through.
var configFormat = func() string {
	parts := make([]string, len(optionNames))
	for i, o := range optionNames {
		parts[i] = "#{" + o + "}"
	}
	return strings.Join(parts, "\t")
}()

// ReadConfig reads the global options.
func ReadConfig(r tmux.Runner) (Config, error) {
	lines, err := r.Run("display-message", "-p", configFormat)
	if err != nil {
		return Config{}, err
	}
	raw := map[string]string{}
	if len(lines) > 0 {
		vals := strings.Split(strings.Join(lines, "\n"), "\t")
		for i, o := range optionNames {
			if i < len(vals) {
				raw[o] = vals[i]
			}
		}
	}
	return parseConfig(raw), nil
}

func parseConfig(raw map[string]string) Config {
	c := Config{
		Enabled:      raw["@modal_enabled"] != "off",
		Scope:        pick(raw["@modal_scope"], "active", "active", "visible", "all"),
		CaptureRows:  atoi(raw["@modal_capture_rows"], 6),
		CPUBudget:    atof(raw["@modal_cpu_budget"], 2),
		SpecPaths:    splitPaths(raw["@modal_spec_paths"]),
		Leader:       orDefault(raw["@modal_escape_leader"], "_"),
		Colour:       raw["@modal_color_matching"] != "off",
		Hook:         raw["@modal_transition_hook"],
		HookTimeout:  ms(atoi(raw["@modal_hook_timeout"], 500)),
		HookDebounce: ms(atoi(raw["@modal_hook_debounce"], 30)),
		LogLevel:     pick(raw["@modal_log_level"], "warn", "debug", "info", "warn", "error"),
		BurstDecay:   ms(atoi(raw["@modal_burst_decay"], 1200)),
		Confirm:      atoi(raw["@modal_confirm_captures"], 2),
	}
	if c.Confirm < 1 {
		c.Confirm = 1
	}
	name := pick(raw["@modal_profile"], "balanced", "frugal", "balanced", "snappy", "custom")
	p, ok := profiles[name]
	if !ok {
		p = profiles["balanced"]
	}
	c.Profile = name
	override := func(opt string, def int) time.Duration {
		if v, err := strconv.Atoi(strings.TrimSpace(raw[opt])); err == nil && v > 0 {
			if v != def {
				c.Profile = "custom"
			}
			return ms(v)
		}
		return ms(def)
	}
	c.Burst = override("@modal_burst_interval", p.burst)
	c.Poll = override("@modal_poll_interval", p.poll)
	c.Idle = override("@modal_idle_interval", p.idle)
	if len(c.SpecPaths) == 0 {
		c.SpecPaths = []string{defaultSpecDir}
	}
	return c
}

const defaultSpecDir = "~/.config/tmux-modal/specs"

// DefaultSpecPaths returns @modal_spec_paths from the running tmux, or the
// default user directory when tmux is unreachable or the option is unset.
func DefaultSpecPaths(r tmux.Runner) []string {
	if lines, err := r.Run("show-options", "-gqv", "@modal_spec_paths"); err == nil && len(lines) > 0 {
		if p := splitPaths(lines[0]); len(p) > 0 {
			return p
		}
	}
	return []string{defaultSpecDir}
}

func splitPaths(s string) []string {
	var out []string
	for _, p := range strings.FieldsFunc(s, func(r rune) bool { return r == ':' || r == ',' }) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func pick(v, def string, allowed ...string) string {
	v = strings.TrimSpace(v)
	for _, a := range allowed {
		if v == a {
			return v
		}
	}
	return def
}

func atoi(s string, def int) int {
	if v, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && v >= 0 {
		return v
	}
	return def
}

func atof(s string, def float64) float64 {
	if v, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil && v > 0 {
		return v
	}
	return def
}

func orDefault(s, def string) string {
	if s = strings.TrimSpace(s); s != "" {
		return s
	}
	return def
}

func ms(n int) time.Duration { return time.Duration(n) * time.Millisecond }
