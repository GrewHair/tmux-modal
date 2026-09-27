package screen

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// A fixture is a saved capture: a header of "# key: value" lines ending in
// "# ---", followed by the plain rendered screen. An optional sibling file
// with the extension ".ansi" holds the same screen captured with -e.
//
//	# tmux-modal fixture v1
//	# size: 200x50
//	# cursor: 40,49
//	# cursor_visible: 1
//	# cursor_shape: bar
//	# alternate_on: 1
//	# command: htop
//	# expect_app: htop
//	# expect_mode: insert
//	# ---
//	<screen text>
type Fixture struct {
	Screen *Screen
	Meta   map[string]string
}

const fixtureMagic = "# tmux-modal fixture v1"

// LoadFixture reads a fixture file (and its .ansi sibling, if present).
// A file without a header is accepted as a bare text capture.
func LoadFixture(path string) (*Fixture, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	meta, body := parseHeader(string(data))
	s := &Screen{}
	if v, ok := meta["size"]; ok {
		if _, err := fmt.Sscanf(v, "%dx%d", &s.Width, &s.Height); err != nil {
			return nil, fmt.Errorf("%s: bad size %q", path, v)
		}
	}
	if v, ok := meta["top"]; ok {
		s.Top, _ = strconv.Atoi(v)
	}
	rows := 0
	if s.Height > 0 {
		rows = s.Height - s.Top
	}
	s.Lines = splitLines(body, rows)
	if s.Height == 0 {
		s.Height = len(s.Lines)
	}
	if s.Width == 0 {
		for _, l := range s.Lines {
			if w := StringWidth(l); w > s.Width {
				s.Width = w
			}
		}
	}
	if v, ok := meta["cursor"]; ok {
		if _, err := fmt.Sscanf(v, "%d,%d", &s.Cursor.X, &s.Cursor.Y); err != nil {
			return nil, fmt.Errorf("%s: bad cursor %q", path, v)
		}
	}
	s.Cursor.Visible = meta["cursor_visible"] == "1"
	s.Cursor.Shape = meta["cursor_shape"]
	s.AltScreen = meta["alternate_on"] == "1"
	s.Command = meta["command"]
	s.Title = meta["title"]

	ansiPath := strings.TrimSuffix(path, ".txt") + ".ansi"
	if ansi, err := os.ReadFile(ansiPath); err == nil {
		s.Cells = ParseANSI(string(ansi), rows)
	}
	return &Fixture{Screen: s, Meta: meta}, nil
}

func parseHeader(data string) (map[string]string, string) {
	meta := map[string]string{}
	if !strings.HasPrefix(data, fixtureMagic) {
		return meta, data
	}
	sc := bufio.NewScanner(strings.NewReader(data))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	consumed := 0
	for sc.Scan() {
		line := sc.Text()
		consumed += len(line) + 1
		if line == "# ---" {
			break
		}
		if k, v, ok := strings.Cut(strings.TrimPrefix(line, "# "), ":"); ok {
			meta[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	if consumed > len(data) {
		consumed = len(data)
	}
	return meta, data[consumed:]
}

// WriteFixture saves a screen as a fixture. plain is the raw capture-pane
// text (trailing whitespace is preserved as captured); ansi, if non-empty,
// is written to the .ansi sibling.
func WriteFixture(path string, s *Screen, plain, ansi string, extra map[string]string) error {
	var b strings.Builder
	b.WriteString(fixtureMagic + "\n")
	fmt.Fprintf(&b, "# size: %dx%d\n", s.Width, s.Height)
	if s.Top != 0 {
		fmt.Fprintf(&b, "# top: %d\n", s.Top)
	}
	fmt.Fprintf(&b, "# cursor: %d,%d\n", s.Cursor.X, s.Cursor.Y)
	fmt.Fprintf(&b, "# cursor_visible: %s\n", b01(s.Cursor.Visible))
	if s.Cursor.Shape != "" {
		fmt.Fprintf(&b, "# cursor_shape: %s\n", s.Cursor.Shape)
	}
	fmt.Fprintf(&b, "# alternate_on: %s\n", b01(s.AltScreen))
	fmt.Fprintf(&b, "# command: %s\n", s.Command)
	if s.Title != "" {
		fmt.Fprintf(&b, "# title: %s\n", s.Title)
	}
	keys := make([]string, 0, len(extra))
	for k := range extra {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, "# %s: %s\n", k, extra[k])
	}
	b.WriteString("# ---\n")
	b.WriteString(plain)
	if !strings.HasSuffix(plain, "\n") {
		b.WriteString("\n")
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return err
	}
	if ansi != "" {
		return os.WriteFile(strings.TrimSuffix(path, ".txt")+".ansi", []byte(ansi), 0o644)
	}
	return nil
}

func b01(v bool) string {
	if v {
		return "1"
	}
	return "0"
}
