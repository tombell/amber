package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	SplitRight = "right"
	SplitDown  = "down"
)

// Layout places a tab's or pane's terminals. A layout without split is one
// terminal. A layout with split divides its space between exactly two child
// panes; ratio is the share given to the first.
type Layout struct {
	Root     string   `yaml:"root,omitempty"`
	Commands []string `yaml:"commands,omitempty"`
	Split    string   `yaml:"split,omitempty"`
	Ratio    *float64 `yaml:"ratio,omitempty"`
	Panes    []Pane   `yaml:"panes,omitempty"`
}

type Pane struct {
	Label  string `yaml:"label,omitempty"`
	Layout `yaml:",inline"`
}

// Tab is one Rex window in a session.
type Tab struct {
	Label  string `yaml:"label"`
	Layout `yaml:",inline"`
}

type Hooks struct {
	BeforeStart []string `yaml:"before_start,omitempty"`
	AfterStart  []string `yaml:"after_start,omitempty"`
	BeforeStop  []string `yaml:"before_stop,omitempty"`
	AfterStop   []string `yaml:"after_stop,omitempty"`
}

func (h Hooks) any() bool {
	return len(h.BeforeStart)+len(h.AfterStart)+len(h.BeforeStop)+len(h.AfterStop) > 0
}

// Session is one Rex session in a project. Its label is the Rex session label.
type Session struct {
	Label string `yaml:"label"`
	Root  string `yaml:"root,omitempty"`
	Hooks `yaml:",inline"`
	Tabs  []Tab `yaml:"tabs"`
}

type Config struct {
	Root     string `yaml:"root"`
	Hooks    `yaml:",inline"`
	Sessions []Session `yaml:"sessions"`
}

// HasHooks reports whether any project or session lifecycle hook is configured.
func (c Config) HasHooks() bool {
	if c.Hooks.any() {
		return true
	}
	for _, session := range c.Sessions {
		if session.Hooks.any() {
			return true
		}
	}
	return false
}

// ValidName reports whether name can be used as a project or session name: 1
// to 64 ASCII letters, digits, '.', '_', or '-', and not "." or "..".
func ValidName(name string) bool {
	if name == "" || len(name) > 64 || name == "." || name == ".." {
		return false
	}
	for _, character := range name {
		switch {
		case character >= 'a' && character <= 'z',
			character >= 'A' && character <= 'Z',
			character >= '0' && character <= '9',
			character == '.', character == '_', character == '-':
		default:
			return false
		}
	}
	return true
}

// Load reads and validates a project config. Local configs expand ~ and resolve
// a relative project root from the config's directory. Configs for a remote
// server must use an absolute or ~ project root, because those paths are
// resolved on another machine. Their ~ paths stay unexpanded, since only the
// server knows its home directory.
func Load(name string, local bool) (*Config, error) {
	path, err := expandHome(name)
	if err != nil {
		return nil, fmt.Errorf("expand config path: %w", err)
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open config: %w", err)
	}
	defer file.Close()

	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)

	var cfg Config
	if err := decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err != nil {
			return nil, fmt.Errorf("decode config: %w", err)
		}
		return nil, errors.New("decode config: multiple YAML documents are not allowed")
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	resolver := pathResolver{local: local}
	if err := cfg.resolvePaths(resolver, filepath.Dir(path)); err != nil {
		return nil, fmt.Errorf("resolve paths: %w", err)
	}
	return &cfg, nil
}

func (c Config) Validate() error {
	var problems []string
	if strings.TrimSpace(c.Root) == "" {
		problems = append(problems, "root is required")
	}
	validateHooks(&problems, "", c.Hooks)
	if len(c.Sessions) == 0 {
		problems = append(problems, "sessions must contain at least one session")
	}

	sessionLabels := make(map[string]struct{}, len(c.Sessions))
	for si, session := range c.Sessions {
		sessionPath := fmt.Sprintf("sessions[%d]", si)
		if session.Label == "" {
			problems = append(problems, sessionPath+".label is required")
		} else if !ValidName(session.Label) {
			problems = append(problems, sessionPath+".label must be 1 to 64 ASCII letters, digits, '.', '_', or '-'")
		} else if _, exists := sessionLabels[session.Label]; exists {
			problems = append(problems, sessionPath+".label must be unique")
		} else {
			sessionLabels[session.Label] = struct{}{}
		}
		validateHooks(&problems, sessionPath+".", session.Hooks)
		if len(session.Tabs) == 0 {
			problems = append(problems, sessionPath+".tabs must contain at least one tab")
		}

		tabLabels := make(map[string]struct{}, len(session.Tabs))
		for ti, tab := range session.Tabs {
			tabPath := fmt.Sprintf("%s.tabs[%d]", sessionPath, ti)
			label := strings.TrimSpace(tab.Label)
			if label == "" {
				problems = append(problems, tabPath+".label is required")
			} else if _, exists := tabLabels[label]; exists {
				problems = append(problems, tabPath+".label must be unique")
			} else {
				tabLabels[label] = struct{}{}
			}
			validatePane(&problems, tabPath, Pane{Layout: tab.Layout})
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("invalid config: %s", strings.Join(problems, "; "))
	}
	return nil
}

func validatePane(problems *[]string, path string, pane Pane) {
	if pane.Split == "" {
		if len(pane.Panes) > 0 {
			*problems = append(*problems, path+".panes requires split")
		}
		if pane.Ratio != nil {
			*problems = append(*problems, path+".ratio requires split")
		}
		validateCommands(problems, path+".commands", pane.Commands)
		return
	}

	if pane.Split != SplitRight && pane.Split != SplitDown {
		*problems = append(*problems, path+".split must be right or down")
	}
	if pane.Ratio != nil && !(*pane.Ratio > 0 && *pane.Ratio < 1) {
		*problems = append(*problems, path+".ratio must be greater than 0 and less than 1")
	}
	if len(pane.Commands) > 0 {
		*problems = append(*problems, path+".commands cannot be used with split")
	}
	if pane.Label != "" {
		*problems = append(*problems, path+".label cannot be used with split")
	}
	if len(pane.Panes) != 2 {
		*problems = append(*problems, path+".panes must contain exactly two panes")
		return
	}
	for pi, child := range pane.Panes {
		validatePane(problems, fmt.Sprintf("%s.panes[%d]", path, pi), child)
	}
}

func validateHooks(problems *[]string, prefix string, hooks Hooks) {
	validateCommands(problems, prefix+"before_start", hooks.BeforeStart)
	validateCommands(problems, prefix+"after_start", hooks.AfterStart)
	validateCommands(problems, prefix+"before_stop", hooks.BeforeStop)
	validateCommands(problems, prefix+"after_stop", hooks.AfterStop)
}

func validateCommands(problems *[]string, path string, commands []string) {
	for i, command := range commands {
		if strings.TrimSpace(command) == "" {
			*problems = append(*problems, fmt.Sprintf("%s[%d] must not be empty", path, i))
		}
	}
}

func (c *Config) resolvePaths(resolver pathResolver, configDir string) error {
	if !resolver.local && !filepath.IsAbs(c.Root) && !isHomePath(c.Root) {
		return errors.New("root must be an absolute or ~ path when using --server")
	}
	root, err := resolver.resolve(configDir, c.Root)
	if err != nil {
		return fmt.Errorf("root: %w", err)
	}
	c.Root = root

	for si := range c.Sessions {
		session := &c.Sessions[si]
		sessionPath := fmt.Sprintf("sessions[%d]", si)
		root, err := resolver.resolve(c.Root, session.Root)
		if err != nil {
			return fmt.Errorf("%s.root: %w", sessionPath, err)
		}
		session.Root = root
		for ti := range session.Tabs {
			path := fmt.Sprintf("%s.tabs[%d]", sessionPath, ti)
			if err := resolveLayout(resolver, session.Root, &session.Tabs[ti].Layout, path); err != nil {
				return err
			}
		}
	}
	return nil
}

func resolveLayout(resolver pathResolver, parent string, layout *Layout, path string) error {
	root, err := resolver.resolve(parent, layout.Root)
	if err != nil {
		return fmt.Errorf("%s.root: %w", path, err)
	}
	layout.Root = root
	for pi := range layout.Panes {
		if err := resolveLayout(resolver, layout.Root, &layout.Panes[pi].Layout, fmt.Sprintf("%s.panes[%d]", path, pi)); err != nil {
			return err
		}
	}
	return nil
}

type pathResolver struct {
	local bool
}

func (r pathResolver) resolve(parent, child string) (string, error) {
	if child == "" {
		return filepath.Clean(parent), nil
	}
	if !r.local {
		return resolveRemote(parent, child)
	}
	expanded, err := expandHome(child)
	if err != nil {
		return "", err
	}
	if filepath.IsAbs(expanded) {
		return filepath.Clean(expanded), nil
	}
	return filepath.Clean(filepath.Join(parent, expanded)), nil
}

// resolveRemote keeps ~ paths relative to the server's home directory. A path
// can't climb out of ~, because the server's home could be anywhere.
func resolveRemote(parent, child string) (string, error) {
	if strings.HasPrefix(child, "~") && !isHomePath(child) {
		return "", fmt.Errorf("unsupported home path %q", child)
	}
	if filepath.IsAbs(child) {
		return filepath.Clean(child), nil
	}
	if !isHomePath(child) {
		if !isHomePath(parent) {
			return filepath.Clean(filepath.Join(parent, child)), nil
		}
		child = parent + "/" + child
	}
	if strings.Contains(child, `\`) {
		return "", fmt.Errorf("home path %q must not contain a backslash with --server", child)
	}
	rest := filepath.Clean(strings.TrimPrefix(strings.TrimPrefix(child, "~"), "/"))
	if rest == ".." || strings.HasPrefix(rest, "../") {
		return "", fmt.Errorf("path %q leaves the home directory", child)
	}
	if rest == "." {
		return "~", nil
	}
	return "~/" + rest, nil
}

func isHomePath(path string) bool {
	return path == "~" || strings.HasPrefix(path, "~/")
}

func expandHome(path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			return home, nil
		}
		return filepath.Join(home, strings.TrimPrefix(path, "~/")), nil
	}
	if strings.HasPrefix(path, "~") {
		return "", fmt.Errorf("unsupported home path %q", path)
	}
	return path, nil
}
