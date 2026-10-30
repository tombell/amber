package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadResolvesRootsThroughNestedSplits(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := writeConfig(t, filepath.Join(home, ".config", "amber"), "demo.yml", `
root: ~/Code/demo
windows:
  - label: server
    root: cmd/server
    split: right
    ratio: 0.6
    panes:
      - commands: ["go run ."]
      - root: ../..
        split: down
        panes:
          - label: tests
            commands: ["go test ./..."]
          - root: /var/log
            commands: ["tail -f app.log"]
  - label: editor
    commands: ["nvim ."]
`)

	cfg, err := Load(path, true)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	project := filepath.Join(home, "Code", "demo")
	server := cfg.Windows[0]
	nested := server.Panes[1]
	roots := map[string]string{
		"project":      cfg.Root,
		"window":       server.Root,
		"first pane":   server.Panes[0].Root,
		"nested split": nested.Root,
		"tests pane":   nested.Panes[0].Root,
		"logs pane":    nested.Panes[1].Root,
		"editor":       cfg.Windows[1].Root,
	}
	want := map[string]string{
		"project":      project,
		"window":       filepath.Join(project, "cmd", "server"),
		"first pane":   filepath.Join(project, "cmd", "server"),
		"nested split": project,
		"tests pane":   project,
		"logs pane":    "/var/log",
		"editor":       project,
	}
	for name, got := range roots {
		if got != want[name] {
			t.Errorf("%s root = %q, want %q", name, got, want[name])
		}
	}
}

func TestValidateReportsLayoutProblems(t *testing.T) {
	ratio := 1.5
	cfg := Config{
		Root: "/project",
		Windows: []Window{
			{Label: "a", Split: "left", Ratio: &ratio, Commands: []string{"x"}, Panes: []Pane{{}}},
			{Label: "a", Panes: []Pane{{}, {}}},
			{Label: "b", Split: "down", Panes: []Pane{{Commands: []string{" "}}, {Label: "x", Split: "right", Panes: []Pane{{}, {}}}}},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil")
	}
	for _, problem := range []string{
		"windows[0].split must be right or down",
		"windows[0].ratio must be greater than 0 and less than 1",
		"windows[0].commands cannot be used with split",
		"windows[0].panes must contain exactly two panes",
		"windows[1].label must be unique",
		"windows[1].panes requires split",
		"windows[2].panes[0].commands[0] must not be empty",
		"windows[2].panes[1].label cannot be used with split",
	} {
		if !strings.Contains(err.Error(), problem) {
			t.Errorf("error %q does not contain %q", err, problem)
		}
	}
}

func TestLoadForServerRejectsLocalOnlyPaths(t *testing.T) {
	dir := t.TempDir()
	tests := map[string]string{
		"relative project root": "root: Code/demo\nwindows: [{label: a}]\n",
		"other user's home":     "root: ~alice/demo\nwindows: [{label: a}]\n",
		"climbs out of home":    "root: ~/demo\nwindows: [{label: a, root: ../..}]\n",
		"backslash":             "root: '~/a\\b'\nwindows: [{label: a}]\n",
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			path := writeConfig(t, dir, "demo.yml", body)
			if _, err := Load(path, false); err == nil {
				t.Fatal("Load() error = nil")
			}
		})
	}
}

func TestLoadForServerKeepsHomePathsForTheServer(t *testing.T) {
	path := writeConfig(t, t.TempDir(), "demo.yml", `
root: ~/Code/demo
windows:
  - label: app
    root: cmd/../app
    split: right
    panes:
      - root: ..
      - root: /var/log
  - label: home
    root: ~/
  - label: srv
    root: /srv/demo
`)

	cfg, err := Load(path, false)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	roots := map[string]string{
		"project":   cfg.Root,
		"app":       cfg.Windows[0].Root,
		"up pane":   cfg.Windows[0].Panes[0].Root,
		"logs pane": cfg.Windows[0].Panes[1].Root,
		"home":      cfg.Windows[1].Root,
		"srv":       cfg.Windows[2].Root,
	}
	want := map[string]string{
		"project":   "~/Code/demo",
		"app":       "~/Code/demo/app",
		"up pane":   "~/Code/demo",
		"logs pane": "/var/log",
		"home":      "~",
		"srv":       "/srv/demo",
	}
	for name, got := range roots {
		if got != want[name] {
			t.Errorf("%s root = %q, want %q", name, got, want[name])
		}
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	path := writeConfig(t, t.TempDir(), "demo.yml", "root: /demo\nworkspaces: []\nwindows: [{label: a}]\n")
	if _, err := Load(path, true); err == nil || !strings.Contains(err.Error(), "workspaces") {
		t.Fatalf("Load() error = %v, want unknown field workspaces", err)
	}
}

func writeConfig(t *testing.T, dir, name, body string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(strings.TrimLeft(body, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
