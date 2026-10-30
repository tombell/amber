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

// Pane is a node in a window's layout. A pane without split is a terminal. A
// pane with split divides its space between exactly two child panes; ratio is
// the share given to the first.
type Pane struct {
	Label    string   `yaml:"label,omitempty"`
	Root     string   `yaml:"root,omitempty"`
	Commands []string `yaml:"commands,omitempty"`
	Split    string   `yaml:"split,omitempty"`
	Ratio    *float64 `yaml:"ratio,omitempty"`
	Panes    []Pane   `yaml:"panes,omitempty"`
}

type Window struct {
	Label    string   `yaml:"label"`
	Root     string   `yaml:"root,omitempty"`
	Commands []string `yaml:"commands,omitempty"`
	Split    string   `yaml:"split,omitempty"`
	Ratio    *float64 `yaml:"ratio,omitempty"`
	Panes    []Pane   `yaml:"panes,omitempty"`
}

// Layout returns the window's root layout node.
func (w Window) Layout() Pane {
	return Pane{Root: w.Root, Commands: w.Commands, Split: w.Split, Ratio: w.Ratio, Panes: w.Panes}
}

type Config struct {
	Root        string   `yaml:"root"`
	BeforeStart []string `yaml:"before_start,omitempty"`
	AfterStart  []string `yaml:"after_start,omitempty"`
	BeforeStop  []string `yaml:"before_stop,omitempty"`
	AfterStop   []string `yaml:"after_stop,omitempty"`
	Windows     []Window `yaml:"windows"`
}

// HasHooks reports whether any lifecycle hook is configured.
func (c Config) HasHooks() bool {
	return len(c.BeforeStart)+len(c.AfterStart)+len(c.BeforeStop)+len(c.AfterStop) > 0
}

// Load reads and validates a project config. Local configs expand ~ and resolve
// a relative project root from the config's directory. Configs for a remote
// server must use an absolute project root and no ~, because those paths are
// resolved on another machine.
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
	validateCommands(&problems, "before_start", c.BeforeStart)
	validateCommands(&problems, "after_start", c.AfterStart)
	validateCommands(&problems, "before_stop", c.BeforeStop)
	validateCommands(&problems, "after_stop", c.AfterStop)
	if len(c.Windows) == 0 {
		problems = append(problems, "windows must contain at least one window")
	}

	labels := make(map[string]struct{}, len(c.Windows))
	for wi, window := range c.Windows {
		windowPath := fmt.Sprintf("windows[%d]", wi)
		label := strings.TrimSpace(window.Label)
		if label == "" {
			problems = append(problems, windowPath+".label is required")
		} else if _, exists := labels[label]; exists {
			problems = append(problems, windowPath+".label must be unique")
		} else {
			labels[label] = struct{}{}
		}
		validatePane(&problems, windowPath, window.Layout())
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

func validateCommands(problems *[]string, path string, commands []string) {
	for i, command := range commands {
		if strings.TrimSpace(command) == "" {
			*problems = append(*problems, fmt.Sprintf("%s[%d] must not be empty", path, i))
		}
	}
}

func (c *Config) resolvePaths(resolver pathResolver, configDir string) error {
	if !resolver.local && !filepath.IsAbs(c.Root) {
		return errors.New("root must be an absolute path when using --server")
	}
	root, err := resolver.resolve(configDir, c.Root)
	if err != nil {
		return fmt.Errorf("root: %w", err)
	}
	c.Root = root

	for wi := range c.Windows {
		window := &c.Windows[wi]
		window.Root, err = resolver.resolve(c.Root, window.Root)
		if err != nil {
			return fmt.Errorf("windows[%d].root: %w", wi, err)
		}
		for pi := range window.Panes {
			path := fmt.Sprintf("windows[%d].panes[%d]", wi, pi)
			if err := resolvePane(resolver, window.Root, &window.Panes[pi], path); err != nil {
				return err
			}
		}
	}
	return nil
}

func resolvePane(resolver pathResolver, parent string, pane *Pane, path string) error {
	root, err := resolver.resolve(parent, pane.Root)
	if err != nil {
		return fmt.Errorf("%s.root: %w", path, err)
	}
	pane.Root = root
	for pi := range pane.Panes {
		if err := resolvePane(resolver, pane.Root, &pane.Panes[pi], fmt.Sprintf("%s.panes[%d]", path, pi)); err != nil {
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
	expanded := child
	if strings.HasPrefix(child, "~") {
		if !r.local {
			return "", fmt.Errorf("home path %q is not supported with --server", child)
		}
		var err error
		if expanded, err = expandHome(child); err != nil {
			return "", err
		}
	}
	if filepath.IsAbs(expanded) {
		return filepath.Clean(expanded), nil
	}
	return filepath.Clean(filepath.Join(parent, expanded)), nil
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
