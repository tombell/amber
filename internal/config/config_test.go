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
sessions:
  - label: app
    root: app
    tabs:
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
  - label: docs
    tabs:
      - label: shell
`)

	cfg, err := Load(path, true)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	project := filepath.Join(home, "Code", "demo")
	app := filepath.Join(project, "app")
	server := cfg.Sessions[0].Tabs[0]
	nested := server.Panes[1]
	roots := map[string]string{
		"project":      cfg.Root,
		"app session":  cfg.Sessions[0].Root,
		"tab":          server.Root,
		"first pane":   server.Panes[0].Root,
		"nested split": nested.Root,
		"tests pane":   nested.Panes[0].Root,
		"logs pane":    nested.Panes[1].Root,
		"editor":       cfg.Sessions[0].Tabs[1].Root,
		"docs session": cfg.Sessions[1].Root,
		"docs tab":     cfg.Sessions[1].Tabs[0].Root,
	}
	want := map[string]string{
		"project":      project,
		"app session":  app,
		"tab":          filepath.Join(app, "cmd", "server"),
		"first pane":   filepath.Join(app, "cmd", "server"),
		"nested split": app,
		"tests pane":   app,
		"logs pane":    "/var/log",
		"editor":       app,
		"docs session": project,
		"docs tab":     project,
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
		Sessions: []Session{
			{Label: "s", Tabs: []Tab{
				{Label: "a", Layout: Layout{Split: "left", Ratio: &ratio, Commands: []string{"x"}, Panes: []Pane{{}}}},
				{Label: "a", Layout: Layout{Panes: []Pane{{}, {}}}},
				{Label: "b", Layout: Layout{Split: "down", Panes: []Pane{
					{Layout: Layout{Commands: []string{" "}}},
					{Label: "x", Layout: Layout{Split: "right", Panes: []Pane{{}, {}}}},
				}}},
			}},
			{Label: "s", Hooks: Hooks{AfterStart: []string{""}}},
			{Label: "a b", Tabs: []Tab{{Label: "a"}}},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil")
	}
	for _, problem := range []string{
		"sessions[0].tabs[0].split must be right or down",
		"sessions[0].tabs[0].ratio must be greater than 0 and less than 1",
		"sessions[0].tabs[0].commands cannot be used with split",
		"sessions[0].tabs[0].panes must contain exactly two panes",
		"sessions[0].tabs[1].label must be unique",
		"sessions[0].tabs[1].panes requires split",
		"sessions[0].tabs[2].panes[0].commands[0] must not be empty",
		"sessions[0].tabs[2].panes[1].label cannot be used with split",
		"sessions[1].label must be unique",
		"sessions[1].after_start[0] must not be empty",
		"sessions[1].tabs must contain at least one tab",
		"sessions[2].label must be 1 to 64 ASCII",
	} {
		if !strings.Contains(err.Error(), problem) {
			t.Errorf("error %q does not contain %q", err, problem)
		}
	}
}

func TestLoadForServerRejectsLocalOnlyPaths(t *testing.T) {
	dir := t.TempDir()
	tests := map[string]string{
		"relative project root": "root: Code/demo\nsessions: [{label: s, tabs: [{label: a}]}]\n",
		"other user's home":     "root: ~alice/demo\nsessions: [{label: s, tabs: [{label: a}]}]\n",
		"climbs out of home":    "root: ~/demo\nsessions: [{label: s, tabs: [{label: a, root: ../..}]}]\n",
		"session climbs out":    "root: ~/demo\nsessions: [{label: s, root: ../.., tabs: [{label: a}]}]\n",
		"backslash":             "root: '~/a\\b'\nsessions: [{label: s, tabs: [{label: a}]}]\n",
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
sessions:
  - label: demo
    tabs:
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
	tabs := cfg.Sessions[0].Tabs
	roots := map[string]string{
		"project":   cfg.Root,
		"app":       tabs[0].Root,
		"up pane":   tabs[0].Panes[0].Root,
		"logs pane": tabs[0].Panes[1].Root,
		"home":      tabs[1].Root,
		"srv":       tabs[2].Root,
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
	path := writeConfig(t, t.TempDir(), "demo.yml", "root: /demo\nwindows: [{label: a}]\n")
	if _, err := Load(path, true); err == nil || !strings.Contains(err.Error(), "windows") {
		t.Fatalf("Load() error = %v, want unknown field windows", err)
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
