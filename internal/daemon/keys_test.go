package daemon

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/GrewHair/tmux-modal/internal/spec"
)

func TestRetable(t *testing.T) {
	cases := []struct{ in, key, out string }{
		{`bind-key    -T root   MouseDown1Pane       select-pane -t = \; send-keys -M`, "MouseDown1Pane",
			`bind-key    -T modal-x   MouseDown1Pane       select-pane -t = \; send-keys -M`},
		{`bind-key -r -T root   M-Up                 resize-pane -U`, "M-Up",
			`bind-key -r -T modal-x   M-Up                 resize-pane -U`},
		{`bind-key    -T prefix c new-window`, "", ""},
	}
	for _, c := range cases {
		key, out, ok := retable(c.in, "modal-x")
		if c.key == "" {
			if ok {
				t.Errorf("non-root line accepted: %s", c.in)
			}
			continue
		}
		if !ok || key != c.key || out != c.out {
			t.Errorf("retable(%q) = %q, %q, %v", c.in, key, out, ok)
		}
	}
}

func TestBindingCommands(t *testing.T) {
	fsys := fstest.MapFS{"a.toml": {Data: []byte(`
modes=["normal","insert"]
[buckets]
typing=["insert"]
commanding=["normal"]
[match]
command=["a"]
[keys]
j = "Down"
";" = "Right"
[escape]
leader = "\\"
`)}}
	set := spec.Load([]spec.Source{{FS: fsys, Dir: "."}})
	sp := set.Specs["a"]
	if sp == nil {
		t.Fatal(set.Warnings)
	}
	root := []string{
		`bind-key    -T root   MouseDown1Pane       select-pane -t = \; send-keys -M`,
		`bind-key    -T root   j                    display-message shadowed`,
	}
	cmds := strings.Join(BindingCommands(sp, "_", root), "\n")
	for _, want := range []string{
		"MouseDown1Pane",                                           // root copied
		"bind-key -T modal-a j if-shell -F",                        // guarded remap
		"{ send-keys Down } { send-keys j }",                       // mapped / literal branches
		"bind-key -T modal-a ';' if-shell",                         // special keys quoted
		`bind-key -T modal-a '\' switch-client -T modal-literal-a`, // spec leader wins
		`bind-key -T modal-literal-a '\' send-keys '\'`,            // leader leader
		"bind-key -T modal-literal-a j send-keys j",                // literal escape
	} {
		if !strings.Contains(cmds, want) {
			t.Errorf("missing %q in:\n%s", want, cmds)
		}
	}
	if strings.Contains(cmds, "display-message shadowed") {
		t.Errorf("a root binding for a remapped key must not be copied")
	}
}
