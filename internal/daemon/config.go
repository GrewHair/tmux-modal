package daemon

import (
	"sort"
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
	NestedRemap  bool
	Transports   []string      // @modal_transports: commands added to the transport list
	HookFlash    time.Duration // @modal_hook_flash: how long the hook badge shows; 0 = never
	HookBlur     bool          // @modal_hook_blur: report a terminal losing focus
	FocusEvents  bool          // tmux's focus-events: the terminals report focus

	// Indicator templates by bucket, published per pane as @modal_indicator.
	Indicator map[string]string
	// Badge templates by key (defaultBadges); a key missing is a badge
	// switched off.
	Badges map[string]string
}

type profile struct{ burst, poll, idle int }

var profiles = map[string]profile{
	"frugal":   {100, 500, 5000},
	"balanced": {30, 150, 2000},
	"snappy":   {15, 60, 1000},
}

var optionNames = append([]string{
	"@modal_enabled", "@modal_profile", "@modal_poll_interval",
	"@modal_idle_interval", "@modal_burst_interval", "@modal_burst_decay",
	"@modal_scope", "@modal_capture_rows", "@modal_cpu_budget",
	"@modal_spec_paths", "@modal_escape_leader", "@modal_color_matching",
	"@modal_transition_hook", "@modal_hook_timeout", "@modal_hook_debounce",
	"@modal_log_level", "@modal_confirm_captures", "@modal_nested_remap", "@modal_transports", "@modal_hook_flash",
	"@modal_hook_blur", "focus-events",
	"@modal_indicator_format", "@modal_indicator_commanding",
	"@modal_indicator_typing", "@modal_indicator_unknown", "@modal_indicator_none",
}, badgeTemplateOptions()...)

func badgeTemplateOptions() []string {
	var opts []string
	for key := range defaultBadges {
		opts = append(opts, badgeOption(key))
	}
	sort.Strings(opts)
	return opts
}

// Default indicator templates. {MODE}/{mode}, {APP}/{app}, {bucket} and
// {confidence} are substituted by the daemon; the result is published as a
// plain pane option so status formats only ever read a variable.
var defaultIndicators = map[string]string{
	"commanding": "#[fg=black,bg=green,bold] {MODE} #[default]",
	"typing":     "#[fg=black,bg=yellow,bold] {MODE} #[default]",
	"unknown":    "#[fg=black,bg=colour244] N/A #[default]",
	"none":       "",
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
	for k, v := range raw {
		raw[k] = unescapeOption(v)
	}
	return parseConfig(raw), nil
}

// unescapeOption undoes tmux's output escaping of option values: tmux 3.x
// prints every '$' as '\$' (in show-options and in #{@option} alike), and
// nothing else. A literal '\$' in the stored value is printed as '\\$',
// which this maps back correctly.
func unescapeOption(v string) string {
	return strings.ReplaceAll(v, `\$`, "$")
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
		NestedRemap:  raw["@modal_nested_remap"] != "off",
		Transports:   strings.Fields(raw["@modal_transports"]),
		HookFlash:    ms(atoi(raw["@modal_hook_flash"], 1500)),
		HookBlur:     raw["@modal_hook_blur"] == "on",
		FocusEvents:  raw["focus-events"] == "1" || raw["focus-events"] == "on", // a format prints a flag as 1
	}
	if c.Confirm < 1 {
		c.Confirm = 1
	}
	c.Indicator = map[string]string{}
	for bucket, def := range defaultIndicators {
		opt := "@modal_indicator_" + bucket
		if v, ok := raw[opt]; ok && v != "" {
			c.Indicator[bucket] = v
		} else if bucket == "commanding" && raw["@modal_indicator_format"] != "" {
			c.Indicator[bucket] = raw["@modal_indicator_format"] // the brief's name
		} else {
			c.Indicator[bucket] = def
		}
	}
	if raw["@modal_indicator_none"] == "off" {
		c.Indicator["none"] = ""
	}
	c.Badges = map[string]string{}
	for key, def := range defaultBadges {
		switch v := raw[badgeOption(key)]; v {
		case "off":
		case "":
			c.Badges[key] = def
		default:
			c.Badges[key] = v
		}
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
