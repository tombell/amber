package rex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tombell/amber/internal/output"
)

const TerminalFlavor = "com.superlogical.terminal.shell"

// Exit statuses documented by the rex CLI.
const (
	exitNotFound    = 3
	exitUnreachable = 4
)

type Session struct {
	ID    string `json:"session_id"`
	Label string `json:"label"`
}

// Node is one node of a session.create layout: either a block or a split.
type Node struct {
	Block *Block `json:"block,omitempty"`
	Split *Split `json:"split,omitempty"`
}

type Block struct {
	Flavor  string       `json:"flavor"`
	Label   string       `json:"label,omitempty"`
	Options BlockOptions `json:"options"`
}

type BlockOptions struct {
	Cwd          string `json:"cwd,omitempty"`
	InitialInput string `json:"initial_input,omitempty"`
}

const (
	DirectionHorizontal = "horizontal"
	DirectionVertical   = "vertical"
)

// Split places Before left of After (horizontal) or above it (vertical). Ratio
// is the share of the space Before gets.
type Split struct {
	Direction string   `json:"direction"`
	Ratio     *float64 `json:"ratio,omitempty"`
	Before    Node     `json:"before"`
	After     Node     `json:"after"`
}

type Window struct {
	Label  string `json:"window_label"`
	Layout Node   `json:"layout"`
}

type CreateSession struct {
	Label   string   `json:"label"`
	Windows []Window `json:"initial_windows"`
}

type CreatedSession struct {
	ID      string `json:"session_id"`
	Windows []struct {
		ID string `json:"window_id"`
	} `json:"initial_windows"`
}

type Client struct {
	binary        string
	server        string
	client        string
	logger        *log.Logger
	commandRunner func(context.Context, []byte, []string) ([]byte, error)
}

// New returns a client for the rex CLI. server selects a host label or address
// (rex -S) and client selects the app that UI actions go to (rex -C). Either
// may be empty to use rex's defaults.
func New(server, client string, logger *log.Logger) Client {
	c := Client{binary: binary(), server: server, client: client, logger: logger}
	c.commandRunner = c.exec
	return c
}

func binary() string {
	if dir := os.Getenv("REX_BIN_DIR"); dir != "" {
		return filepath.Join(dir, "rex")
	}
	return "rex"
}

func (c Client) ListSessions(ctx context.Context) ([]Session, error) {
	out, err := c.run(ctx, nil, "ls", "--json")
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	var response struct {
		Sessions []Session `json:"sessions"`
	}
	if err := json.Unmarshal(out, &response); err != nil {
		return nil, fmt.Errorf("parse session list: %w", err)
	}
	return response.Sessions, nil
}

// FindSession returns the session with the given label. Rex labels are not
// unique, so more than one match is an error rather than a guess.
func (c Client) FindSession(ctx context.Context, label string) (Session, bool, error) {
	sessions, err := c.ListSessions(ctx)
	if err != nil {
		return Session{}, false, err
	}
	var matches []Session
	for _, session := range sessions {
		if session.Label == label {
			matches = append(matches, session)
		}
	}
	switch len(matches) {
	case 0:
		return Session{}, false, nil
	case 1:
		return matches[0], true, nil
	default:
		return Session{}, false, fmt.Errorf("%d sessions are labelled %q", len(matches), label)
	}
}

// CreateSession creates the session and every window in one request. Rex
// creates either all of it or none of it.
func (c Client) CreateSession(ctx context.Context, request CreateSession) (CreatedSession, error) {
	payload, err := json.Marshal(request)
	if err != nil {
		return CreatedSession{}, fmt.Errorf("encode session %q: %w", request.Label, err)
	}
	out, err := c.run(ctx, payload, "api", "call", "session.create", "-")
	if err != nil {
		return CreatedSession{}, fmt.Errorf("create session %q: %w", request.Label, err)
	}
	var created CreatedSession
	if err := json.Unmarshal(out, &created); err != nil {
		return CreatedSession{}, fmt.Errorf("parse created session: %w", err)
	}
	return created, nil
}

func (c Client) FocusWindow(ctx context.Context, sessionID, windowID string) error {
	if _, err := c.run(ctx, nil, "focus", "-s", sessionID, windowID); err != nil {
		return fmt.Errorf("focus window %s: %w", windowID, err)
	}
	return nil
}

func (c Client) KillSession(ctx context.Context, sessionID string) error {
	if _, err := c.run(ctx, nil, "kill", sessionID); err != nil {
		return fmt.Errorf("kill session %s: %w", sessionID, err)
	}
	return nil
}

// SelectSession asks a connected Rex app to show the session.
func (c Client) SelectSession(ctx context.Context, sessionID string) error {
	args := []string{"do", "session.select", "session_id=" + sessionID}
	if c.client != "" {
		args = append([]string{"-C", c.client}, args...)
	}
	if _, err := c.run(ctx, nil, args...); err != nil {
		return fmt.Errorf("show session %s: %w", sessionID, err)
	}
	return nil
}

func (c Client) run(ctx context.Context, stdin []byte, args ...string) ([]byte, error) {
	if c.server != "" {
		args = append([]string{"-S", c.server}, args...)
	}
	if c.logger != nil {
		c.logger.Printf("%s %s", c.binary, formatArguments(args))
	}
	return c.commandRunner(ctx, stdin, args)
}

func (c Client) exec(ctx context.Context, stdin []byte, args []string) ([]byte, error) {
	command := exec.CommandContext(ctx, c.binary, args...)
	if stdin != nil {
		command.Stdin = bytes.NewReader(stdin)
	}
	var stdout bytes.Buffer
	var stderr output.Tail
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	if err == nil {
		return stdout.Bytes(), nil
	}

	message := strings.TrimSpace(stderr.String())
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		switch exitErr.ExitCode() {
		case exitNotFound:
			err = errors.New("target not found or ambiguous")
		case exitUnreachable:
			err = errors.New("Rex server unreachable")
		}
	}
	if message != "" {
		return nil, fmt.Errorf("%w: %s", err, message)
	}
	return nil, err
}

func formatArguments(args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		if arg == "" || strings.ContainsAny(arg, " \t\n\"'\\$") {
			quoted[i] = strconv.Quote(arg)
		} else {
			quoted[i] = arg
		}
	}
	return strings.Join(quoted, " ")
}
