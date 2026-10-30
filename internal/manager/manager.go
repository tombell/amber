package manager

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/tombell/amber/internal/config"
	"github.com/tombell/amber/internal/output"
	"github.com/tombell/amber/internal/rex"
)

const (
	projectEnvironment = "AMBER_PROJECT"
	sessionEnvironment = "REX_SESSION"
	blockEnvironment   = "REX_BLOCK"
)

type Rex interface {
	FindSession(context.Context, string) (rex.Session, bool, error)
	CreateSession(context.Context, rex.CreateSession) (rex.CreatedSession, error)
	FocusWindow(context.Context, string, string) error
	KillSession(context.Context, string) error
}

type Shell interface {
	Run(ctx context.Context, project, sessionID, root, command string) error
}

type Manager struct {
	rex   Rex
	shell Shell
}

func New(client Rex, shell Shell) Manager {
	return Manager{rex: client, shell: shell}
}

// Start creates the project's session unless a session with its label already
// exists, and returns the session's ID.
func (m Manager) Start(ctx context.Context, project string, cfg *config.Config) (string, error) {
	if err := cfg.Validate(); err != nil {
		return "", err
	}

	existing, found, err := m.rex.FindSession(ctx, project)
	if err != nil {
		return "", fmt.Errorf("check session %q: %w", project, err)
	}
	if found {
		return existing.ID, nil
	}

	if err := m.runHooks(ctx, project, "", cfg.Root, cfg.BeforeStart); err != nil {
		return "", fmt.Errorf("run project before_start: %w", err)
	}
	created, err := m.rex.CreateSession(ctx, Layout(project, cfg))
	if err != nil {
		return "", err
	}
	// Rex makes the last initial window active; start on the first like the config reads.
	if len(created.Windows) > 1 {
		if err := m.rex.FocusWindow(ctx, created.ID, created.Windows[0].ID); err != nil {
			return "", err
		}
	}
	if err := m.runHooks(ctx, project, created.ID, cfg.Root, cfg.AfterStart); err != nil {
		return "", fmt.Errorf("run project after_start: %w", err)
	}
	return created.ID, nil
}

func (m Manager) Stop(ctx context.Context, project string, cfg *config.Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}

	session, found, err := m.rex.FindSession(ctx, project)
	if err != nil {
		return fmt.Errorf("check session %q: %w", project, err)
	}
	if !found {
		return nil
	}
	if err := m.runHooks(ctx, project, session.ID, cfg.Root, cfg.BeforeStop); err != nil {
		return fmt.Errorf("run project before_stop: %w", err)
	}
	if err := m.rex.KillSession(ctx, session.ID); err != nil {
		return err
	}
	if err := m.runHooks(ctx, project, "", cfg.Root, cfg.AfterStop); err != nil {
		return fmt.Errorf("run project after_stop: %w", err)
	}
	return nil
}

// Layout converts a project config into a single session.create request.
func Layout(project string, cfg *config.Config) rex.CreateSession {
	request := rex.CreateSession{Label: project, Windows: make([]rex.Window, len(cfg.Windows))}
	for i, window := range cfg.Windows {
		request.Windows[i] = rex.Window{Label: window.Label, Layout: layoutNode(window.Layout())}
	}
	return request
}

func layoutNode(pane config.Pane) rex.Node {
	if pane.Split == "" {
		cwd, commands := pane.Root, pane.Commands
		// Rex doesn't expand ~ in cwd, and an empty cwd starts in the home
		// directory, so the shell changes into a path under ~ itself.
		if strings.HasPrefix(cwd, "~") {
			if cwd != "~" {
				commands = append([]string{"cd ~/" + shellQuote(strings.TrimPrefix(cwd, "~/"))}, commands...)
			}
			cwd = ""
		}
		var input string
		if len(commands) > 0 {
			input = strings.Join(commands, "\n") + "\n"
		}
		return rex.Node{Block: &rex.Block{
			Flavor:  rex.TerminalFlavor,
			Label:   pane.Label,
			Options: rex.BlockOptions{Cwd: cwd, InitialInput: input},
		}}
	}

	direction := rex.DirectionHorizontal
	if pane.Split == config.SplitDown {
		direction = rex.DirectionVertical
	}
	return rex.Node{Split: &rex.Split{
		Direction: direction,
		Ratio:     pane.Ratio,
		Before:    layoutNode(pane.Panes[0]),
		After:     layoutNode(pane.Panes[1]),
	}}
}

// shellQuote quotes a path so that sh, bash, zsh, and fish all read it the same
// way. Config loading rejects backslashes, which fish treats as escapes inside
// single quotes.
func shellQuote(path string) string {
	safe := path != "" && strings.IndexFunc(path, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("/._-+,:@%", r))
	}) < 0
	if safe {
		return path
	}
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}

func (m Manager) runHooks(ctx context.Context, project, sessionID, root string, hooks []string) error {
	for index, hook := range hooks {
		if err := m.shell.Run(ctx, project, sessionID, root, hook); err != nil {
			return fmt.Errorf("hook %d: %w", index+1, err)
		}
	}
	return nil
}

type DefaultShell struct {
	logger *log.Logger
}

func NewDefaultShell(logger *log.Logger) DefaultShell {
	return DefaultShell{logger: logger}
}

func (s DefaultShell) Run(ctx context.Context, project, sessionID, root, command string) error {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	if s.logger != nil {
		s.logger.Printf("cd %q && %s=%q %s=%q %s -c %q", root, projectEnvironment, project, sessionEnvironment, sessionID, shell, command)
	}

	process := exec.CommandContext(ctx, shell, "-c", command)
	process.Dir = root
	process.Env = Environment(os.Environ(), project, sessionID)
	var tail output.Tail
	process.Stdout = &tail
	process.Stderr = &tail
	if err := process.Run(); err != nil {
		message := strings.TrimSpace(tail.String())
		if message != "" {
			return fmt.Errorf("%w: %s", err, message)
		}
		return err
	}
	return nil
}

// Environment returns the hook environment. It drops the caller's own Rex
// session and block, so rex commands in a hook target the project's session
// once it exists rather than the terminal amber was run from.
func Environment(env []string, project, sessionID string) []string {
	result := make([]string, 0, len(env)+2)
	for _, entry := range env {
		name, _, _ := strings.Cut(entry, "=")
		if name == sessionEnvironment || name == blockEnvironment || name == projectEnvironment {
			continue
		}
		result = append(result, entry)
	}
	result = append(result, projectEnvironment+"="+project)
	if sessionID != "" {
		result = append(result, sessionEnvironment+"="+sessionID)
	}
	return result
}
