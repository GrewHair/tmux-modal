package daemon

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/GrewHair/tmux-modal/internal/tmux"
)

func TestParseConfigProfiles(t *testing.T) {
	c := parseConfig(map[string]string{})
	if c.Profile != "balanced" || c.Burst != 30*time.Millisecond || c.Poll != 150*time.Millisecond || c.Idle != 2*time.Second {
		t.Errorf("defaults: %+v", c)
	}
	c = parseConfig(map[string]string{"@modal_profile": "snappy"})
	if c.Profile != "snappy" || c.Burst != 15*time.Millisecond {
		t.Errorf("snappy: %+v", c)
	}
	c = parseConfig(map[string]string{"@modal_profile": "frugal", "@modal_poll_interval": "250"})
	if c.Profile != "custom" || c.Poll != 250*time.Millisecond || c.Idle != 5*time.Second {
		t.Errorf("an explicit option must win over the profile and flip it to custom: %+v", c)
	}
	c = parseConfig(map[string]string{"@modal_profile": "frugal", "@modal_poll_interval": "500"})
	if c.Profile != "frugal" {
		t.Errorf("setting an option to the profile's own value keeps the profile: %+v", c)
	}
	if c := parseConfig(map[string]string{"@modal_scope": "bogus"}); c.Scope != "active" {
		t.Errorf("invalid scope must fall back to active, got %q", c.Scope)
	}
}

// ReadConfig against a real server: every option must round-trip through
// the packed format (this caught a separator that tmux escapes).
func TestReadConfigRoundTrip(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	label := "cfg-test-" + strings.ReplaceAll(t.Name(), "/", "-")
	srv := tmux.Server{Label: label}
	ex := tmux.Exec{Server: srv}
	if _, err := ex.Run("-f", "/dev/null", "new-session", "-d"); err != nil {
		t.Fatal(err)
	}
	sock, _ := ex.Run("display-message", "-p", "#{socket_path}")
	defer func() {
		ex.Run("kill-server")
		if len(sock) > 0 {
			os.Remove(sock[0]) // tmux leaves the socket file behind
		}
	}()
	ex.Run("set-option", "-g", "@modal_log_level", "debug")
	ex.Run("set-option", "-g", "@modal_scope", "visible")
	ex.Run("set-option", "-g", "@modal_spec_paths", "/a:/b")
	ex.Run("set-option", "-g", "@modal_idle_interval", "777")
	c, err := ReadConfig(ex)
	if err != nil {
		t.Fatal(err)
	}
	if c.LogLevel != "debug" || c.Scope != "visible" || strings.Join(c.SpecPaths, ",") != "/a,/b" || c.Idle != 777*time.Millisecond {
		t.Errorf("round trip: %+v", c)
	}
}
