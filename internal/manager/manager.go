package manager

import (
	"context"
	"errors"
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

// Start creates each of the project's sessions that doesn't exist yet, and
// returns the ID of the project's first session.
//
// The project's before_start hooks run once if any session is missing. Each
// missing session then runs its own before_start hooks and is created in one
// request. If any of that fails, Start kills the sessions it created, so the
// project's sessions are built completely or not at all. The after_start hooks
// run once every session exists: each new session's first, then the project's.
func (m Manager) Start(ctx context.Context, project string, cfg *config.Config) (string, error) {
	if err := cfg.Validate(); err != nil {
		return "", err
	}

	ids := make([]string, len(cfg.Sessions))
	var missing []int
	for i, session := range cfg.Sessions {
		existing, found, err := m.rex.FindSession(ctx, session.Label)
		if err != nil {
			return "", fmt.Errorf("check session %q: %w", session.Label, err)
		}
		if found {
			ids[i] = existing.ID
		} else {
			missing = append(missing, i)
		}
	}
	if len(missing) == 0 {
		return ids[0], nil
	}

	if err := m.runHooks(ctx, project, "", cfg.Root, cfg.BeforeStart); err != nil {
		return "", fmt.Errorf("run project before_start: %w", err)
	}
	var created []string
	for _, i := range missing {
		id, err := m.create(ctx, project, cfg.Sessions[i])
		if id != "" {
			ids[i] = id
			created = append(created, id)
		}
		if err != nil {
			return "", m.rollback(ctx, created, err)
		}
	}

	for _, i := range missing {
		session := cfg.Sessions[i]
		if err := m.runHooks(ctx, project, ids[i], session.Root, session.AfterStart); err != nil {
			return "", fmt.Errorf("run session %q after_start: %w", session.Label, err)
		}
	}
	if err := m.runHooks(ctx, project, "", cfg.Root, cfg.AfterStart); err != nil {
		return "", fmt.Errorf("run project after_start: %w", err)
	}
	return ids[0], nil
}

func (m Manager) create(ctx context.Context, project string, session config.Session) (string, error) {
	if err := m.runHooks(ctx, project, "", session.Root, session.BeforeStart); err != nil {
		return "", fmt.Errorf("run session %q before_start: %w", session.Label, err)
	}
	created, err := m.rex.CreateSession(ctx, Layout(session))
	if err != nil {
		return "", err
	}
	// Rex makes the last initial window active; start on the first like the config reads.
	if len(created.Windows) > 1 {
		if err := m.rex.FocusWindow(ctx, created.ID, created.Windows[0].ID); err != nil {
			return created.ID, err
		}
	}
	return created.ID, nil
}

// rollback kills the sessions a failed start created, newest first.
func (m Manager) rollback(ctx context.Context, created []string, cause error) error {
	errs := []error{cause}
	for i := len(created) - 1; i >= 0; i-- {
		if err := m.rex.KillSession(ctx, created[i]); err != nil {
			errs = append(errs, fmt.Errorf("roll back: %w", err))
		}
	}
	return errors.Join(errs...)
}

// Stop kills each of the project's running sessions. The project's
// before_stop hooks run once first, and its after_stop hooks once at the end.
// Each session runs its own before_stop and after_stop hooks around its kill.
func (m Manager) Stop(ctx context.Context, project string, cfg *config.Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}

	type running struct {
		id      string
		session config.Session
	}
	var sessions []running
	for _, session := range cfg.Sessions {
		existing, found, err := m.rex.FindSession(ctx, session.Label)
		if err != nil {
			return fmt.Errorf("check session %q: %w", session.Label, err)
		}
		if found {
			sessions = append(sessions, running{id: existing.ID, session: session})
		}
	}
	if len(sessions) == 0 {
		return nil
	}

	if err := m.runHooks(ctx, project, "", cfg.Root, cfg.BeforeStop); err != nil {
		return fmt.Errorf("run project before_stop: %w", err)
	}
	for _, r := range sessions {
		if err := m.runHooks(ctx, project, r.id, r.session.Root, r.session.BeforeStop); err != nil {
			return fmt.Errorf("run session %q before_stop: %w", r.session.Label, err)
		}
		if err := m.rex.KillSession(ctx, r.id); err != nil {
			return err
		}
		if err := m.runHooks(ctx, project, "", r.session.Root, r.session.AfterStop); err != nil {
			return fmt.Errorf("run session %q after_stop: %w", r.session.Label, err)
		}
	}
	if err := m.runHooks(ctx, project, "", cfg.Root, cfg.AfterStop); err != nil {
		return fmt.Errorf("run project after_stop: %w", err)
	}
	return nil
}

// Layout converts a session config into a single session.create request. Each
// tab becomes a Rex window.
func Layout(session config.Session) rex.CreateSession {
	request := rex.CreateSession{Label: session.Label, Windows: make([]rex.Window, len(session.Tabs))}
	for i, tab := range session.Tabs {
		request.Windows[i] = rex.Window{Label: tab.Label, Layout: layoutNode(config.Pane{Layout: tab.Layout})}
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
