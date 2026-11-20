package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveSshTarget_ValidTargets(t *testing.T) {
	tests := []struct {
		name     string
		target   string
		expected string
	}{
		{
			name:     "user and hostname",
			target:   "alice@server.example.com",
			expected: "alice@server.example.com",
		},
		{
			name:     "ip address destination",
			target:   "backup@192.168.1.50",
			expected: "backup@192.168.1.50",
		},
		{
			name:     "custom port-style host",
			target:   "deploy@host-01.internal",
			expected: "deploy@host-01.internal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveSSHTarget(tt.target)
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.target, err)
			}
			if got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestResolveSSHTarget_RejectsHyphenAndCommandInjection(t *testing.T) {
	tests := []struct {
		name   string
		target string
	}{
		{
			name:   "leading hyphen flag option",
			target: "-oProxyCommand=touch /tmp/pwned",
		},
		{
			name:   "user with leading hyphen",
			target: "-oProxyCommand=id@attacker.com",
		},
		{
			name:   "host with leading hyphen",
			target: "alice@-oProxyCommand=calc.exe",
		},
		{
			name:   "double dash flag",
			target: "--target=evil.com",
		},
		{
			name:   "semicolon injection",
			target: "alice@server;rm -rf /",
		},
		{
			name:   "backtick command substitution",
			target: "alice@`id`",
		},
		{
			name:   "space separated injection",
			target: "alice@server extra args",
		},
		{
			name:   "pipe injection",
			target: "alice@server|cat",
		},
		{
			name:   "multiple at signs",
			target: "user@host1@host2",
		},
		{
			name:   "empty target",
			target: "   ",
		},
		{
			name:   "empty host with user",
			target: "user@",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveSSHTarget(tt.target)
			if err == nil {
				t.Errorf("expected error for malicious target %q, got nil (result: %q)", tt.target, got)
			}
			if !strings.Contains(err.Error(), "migrate:") {
				t.Errorf("expected error to have migrate prefix, got: %v", err)
			}
		})
	}
}

func TestParseTarget(t *testing.T) {
	u, h := parseTarget("user@host.com")
	if u != "user" || h != "host.com" {
		t.Errorf("unexpected result: user=%q, host=%q", u, h)
	}

	u2, h2 := parseTarget("hostonly.local")
	if u2 != "" || h2 != "hostonly.local" {
		t.Errorf("unexpected result: user=%q, host=%q", u2, h2)
	}
}

// The remote's directory is chosen as cfg_g.ConfigDirIn chooses one locally,
// so a migration never leaves the remote with a feral directory beside a real
// mail_cli one -- which every run there would refuse.
func TestRemoteConfigDirScript(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh")
	}
	home := t.TempDir()
	cfg := filepath.Join(home, ".config")
	answer := func() string {
		cmd := exec.Command(sh, "-c", remoteConfigDirScript)
		cmd.Dir = home
		out, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(string(out))
	}
	if got := answer(); got != ".config/feral" {
		t.Errorf("fresh remote: %q", got)
	}
	if err := os.MkdirAll(filepath.Join(cfg, "mail_cli"), 0o700); err != nil {
		t.Fatal(err)
	}
	if got := answer(); got != ".config/mail_cli" {
		t.Errorf("remote with only the old directory: %q", got)
	}
	if err := os.Symlink("mail_cli", filepath.Join(cfg, "feral")); err != nil {
		t.Fatal(err)
	}
	if got := answer(); got != ".config/feral" {
		t.Errorf("remote already migrated: %q", got)
	}
}
