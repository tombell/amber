package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tombell/amber/internal/config"
	"github.com/tombell/amber/internal/manager"
	"github.com/tombell/amber/internal/rex"
)

type App struct {
	logger *log.Logger
	stdout io.Writer
	stderr io.Writer
	server string
	client string
}

func New(logger *log.Logger, stdout, stderr io.Writer) App {
	return App{logger: logger, stdout: stdout, stderr: stderr}
}

// WithTarget selects the Rex server (a host label or address) and the Rex app
// that show actions go to. Empty values use rex's defaults.
func (a App) WithTarget(server, client string) App {
	a.server = server
	a.client = client
	return a
}

func (a App) rex() rex.Client {
	return rex.New(a.server, a.client, a.logger)
}

func (a App) Complete(kind string) error {
	switch kind {
	case "projects":
		return a.List()
	case "sessions":
		return a.ListRunningSessions()
	default:
		return errors.New("usage: amber __complete <projects|sessions>")
	}
}

func (a App) List() error {
	dir, err := projectsDirectory()
	if err != nil {
		return fmt.Errorf("resolve projects directory: %w", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read projects directory: %w", err)
	}

	var projects []string
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yml" {
			continue
		}
		projects = append(projects, strings.TrimSuffix(entry.Name(), ".yml"))
	}
	sort.Strings(projects)
	for _, project := range projects {
		fmt.Fprintln(a.stdout, project)
	}
	return nil
}

func (a App) ListRunningSessions() error {
	sessions, err := a.rex().ListSessions(context.Background())
	if err != nil {
		return fmt.Errorf("list running sessions: %w", err)
	}

	labels := make([]string, 0, len(sessions))
	for _, session := range sessions {
		labels = append(labels, session.Label)
	}
	sort.Strings(labels)
	for _, label := range labels {
		if _, err := fmt.Fprintln(a.stdout, label); err != nil {
			return err
		}
	}
	return nil
}

func (a App) Start(names []string, show bool) error {
	if len(names) == 0 {
		return errors.New("usage: amber start [--show] <project names...>")
	}
	if show && len(names) != 1 {
		return errors.New("--show requires exactly one project")
	}

	projects, err := a.loadProjects(names)
	if err != nil {
		return err
	}
	lifecycle := a.manager()
	var sessionID string
	for _, project := range projects {
		if sessionID, err = lifecycle.Start(context.Background(), project.name, project.cfg); err != nil {
			return fmt.Errorf("start project %q: %w", project.name, err)
		}
	}
	if show {
		return a.rex().SelectSession(context.Background(), sessionID)
	}
	return nil
}

func (a App) Show(name string) error {
	client := a.rex()
	session, found, err := client.FindSession(context.Background(), name)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("session %q does not exist", name)
	}
	return client.SelectSession(context.Background(), session.ID)
}

func (a App) Stop(names []string) error {
	if len(names) == 0 {
		return errors.New("usage: amber stop <project names...>")
	}

	projects, err := a.loadProjects(names)
	if err != nil {
		return err
	}
	lifecycle := a.manager()
	for _, project := range projects {
		if err := lifecycle.Stop(context.Background(), project.name, project.cfg); err != nil {
			return fmt.Errorf("stop project %q: %w", project.name, err)
		}
	}
	return nil
}

func (a App) manager() manager.Manager {
	return manager.New(a.rex(), manager.NewDefaultShell(a.logger))
}

type project struct {
	name string
	cfg  *config.Config
}

// loadProjects validates every project before any of them is started or
// stopped. Configs are always read locally. With --server, panes run on that
// server but hooks would still run here, so hooks are rejected until amber
// can run them on the server too.
func (a App) loadProjects(names []string) ([]project, error) {
	dir, err := projectsDirectory()
	if err != nil {
		return nil, fmt.Errorf("resolve projects directory: %w", err)
	}

	local := a.server == ""
	projects := make([]project, 0, len(names))
	var invalid []string
	for _, name := range names {
		if !validProjectName(name) {
			invalid = append(invalid, fmt.Sprintf("%s (invalid project name)", name))
			continue
		}
		cfg, err := config.Load(filepath.Join(dir, name+".yml"), local)
		if err != nil {
			invalid = append(invalid, fmt.Sprintf("%s (%v)", name, err))
			continue
		}
		if !local && cfg.HasHooks() {
			invalid = append(invalid, fmt.Sprintf("%s (hooks are not supported with --server)", name))
			continue
		}
		projects = append(projects, project{name: name, cfg: cfg})
	}
	if len(invalid) > 0 {
		return nil, fmt.Errorf("invalid projects: %s", strings.Join(invalid, ", "))
	}
	return projects, nil
}

func validProjectName(name string) bool {
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

func projectsDirectory() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "amber"), nil
}
