package daemon

import (
	"regexp"
	"strings"
	"testing"
)

// plain drops tmux style directives, leaving the text a badge shows.
func plain(s string) string {
	return strings.Join(strings.Fields(regexp.MustCompile(`#\[[^]]*\]`).ReplaceAllString(s, "")), " ")
}

func TestBadges(t *testing.T) {
	tpl := parseConfig(map[string]string{}).Badges
	normal := modeState{App: "htop", Mode: "normal", Bucket: "commanding", Confidence: "high"}
	cases := []struct {
		name string
		m    modeState
		det  detail
		want string
	}{
		{"shell", modeState{Mode: "none", Bucket: "none"}, detail{}, ""},
		{"unrecognised full-screen app", modeState{Mode: "unknown", Bucket: "unknown"},
			detail{Alt: true, Unknown: true}, "ALT ?"},
		{"htop by its command", normal, detail{Alt: true, Evidence: "cmd"}, "ALT htop cmd NORMAL"},
		{"htop by fingerprint, remapped", normal,
			detail{Alt: true, Evidence: "fp", Score: "80/40", Remap: true},
			"ALT htop fp 80/40 NORMAL #{?#{m:modal-literal-*,#{client_key_table}},MAP _,MAP}"},
		{"vim remembered, mode unreadable", modeState{App: "vim", Mode: "unknown", Bucket: "unknown"},
			detail{Alt: true, Evidence: "mem", Score: "0/60"}, "ALT vim mem 0/60 ?"},
		{"split remote tmux over ssh, focus by border", normal,
			detail{Alt: true, Via: "ssh", Evidence: "fp", Score: "80/40", NestedKind: "tmux", InnerPanes: 3, FocusBy: "border colour"},
			"ALT VIA ssh NEST tmux SPLIT 3 border htop fp 80/40 NORMAL"},
		{"split remote tmux, no focus", modeState{Mode: "unknown", Bucket: "unknown"},
			detail{Alt: true, Via: "docker", Unknown: true, NestedKind: "tmux", InnerPanes: 2},
			"ALT VIA docker NEST tmux SPLIT 2 ? ?"},
		{"local screen client by command", modeState{Mode: "unknown", Bucket: "unknown"},
			detail{Alt: true, Unknown: true, NestedKind: "screen"}, "ALT NEST screen ?"},
		{"htop over ssh, no nesting", normal, detail{Alt: true, Via: "ssh", Evidence: "fp", Score: "80/40"},
			"ALT VIA ssh htop fp 80/40 NORMAL"},
		{"cursor shape reported", modeState{App: "nvim", Mode: "insert", Bucket: "typing"},
			detail{Alt: true, Evidence: "cmd+fp", Score: "100/100", Shape: "bar"},
			"ALT nvim cmd+fp 100/100 INSERT ▏"},
		{"block cursor", modeState{App: "nvim", Mode: "normal", Bucket: "commanding"},
			detail{Alt: true, Evidence: "cmd", Shape: "block"}, "ALT nvim cmd NORMAL █"},
		{"underline cursor", modeState{App: "nvim", Mode: "replace", Bucket: "typing"},
			detail{Alt: true, Evidence: "cmd", Shape: "underline"}, "ALT nvim cmd REPLACE ▁"},
	}
	for _, c := range cases {
		if got := plain(joinBadges(renderBadges(tpl, c.m, c.det, "_"))); got != c.want {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
	}

	// Seen but not in effect (no transport): the same text, struck through.
	off := renderBadges(tpl, modeState{Mode: "unknown", Bucket: "unknown"},
		detail{Alt: true, Unknown: true, NestedKind: "tmux", NestedOff: "borders", InnerPanes: 2, FocusBy: "cursor"}, "_")
	if got := plain(joinBadges(off)); got != "ALT NEST tmux SPLIT 2 cursor ?" {
		t.Errorf("seen but off: %q", got)
	}
	if !strings.Contains(off[2], "strikethrough") || !strings.Contains(off[3], "strikethrough") {
		t.Errorf("seen but off, not struck through: %q %q", off[2], off[3])
	}

	// "off" hides a badge; a template of the user's own is used as is.
	own := parseConfig(map[string]string{
		badgeOption("alt"): "off",
		badgeOption("app"): "[{APP}{ score}]",
	}).Badges
	got := joinBadges(renderBadges(own, normal, detail{Alt: true, Evidence: "cmd"}, "_"))
	if plain(got) != "[HTOP] NORMAL" {
		t.Errorf("own templates: %q", got)
	}
}

func TestBadgeExpand(t *testing.T) {
	vals := map[string]string{"a": "x", "e": ""}
	for in, want := range map[string]string{
		"{a}{ a}{ e}|":        "x x|",
		"#{?#{m:x,{a}},y,z}":  "#{?#{m:x,x},y,z}",
		"{unknown} {a":        "{unknown} {a",
		"#{client_key_table}": "#{client_key_table}",
	} {
		if got := expand(in, vals); got != want {
			t.Errorf("expand(%q) = %q, want %q", in, got, want)
		}
	}
	if got := formatEscape("#,}"); got != "###,#}" {
		t.Errorf("formatEscape: %q", got)
	}
}
