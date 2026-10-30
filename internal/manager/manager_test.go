package manager

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/tombell/amber/internal/config"
	"github.com/tombell/amber/internal/rex"
)

type fakeRex struct {
	events   *[]string
	sessions []rex.Session
	windows  int
}

func (f *fakeRex) FindSession(_ context.Context, label string) (rex.Session, bool, error) {
	*f.events = append(*f.events, "find "+label)
	for _, session := range f.sessions {
		if session.Label == label {
			return session, true, nil
		}
	}
	return rex.Session{}, false, nil
}

func (f *fakeRex) CreateSession(_ context.Context, request rex.CreateSession) (rex.CreatedSession, error) {
	*f.events = append(*f.events, fmt.Sprintf("create %s windows=%d", request.Label, len(request.Windows)))
	created := rex.CreatedSession{ID: "session:new"}
	for i := range request.Windows {
		created.Windows = append(created.Windows, struct {
			ID string `json:"window_id"`
		}{ID: fmt.Sprintf("window:%d", i+1)})
	}
	return created, nil
}

func (f *fakeRex) FocusWindow(_ context.Context, sessionID, windowID string) error {
	*f.events = append(*f.events, "focus "+sessionID+" "+windowID)
	return nil
}

func (f *fakeRex) KillSession(_ context.Context, sessionID string) error {
	*f.events = append(*f.events, "kill "+sessionID)
	return nil
}

type fakeShell struct {
	events *[]string
	fail   string
}

func (s fakeShell) Run(_ context.Context, project, sessionID, root, command string) error {
	*s.events = append(*s.events, fmt.Sprintf("hook %s session=%q %s", root, sessionID, command))
	if command == s.fail {
		return errors.New("hook failed")
	}
	return nil
}

func project() *config.Config {
	return &config.Config{
		Root:        "/project",
		BeforeStart: []string{"before start"},
		AfterStart:  []string{"after start"},
		BeforeStop:  []string{"before stop"},
		AfterStop:   []string{"after stop"},
		Windows:     []config.Window{{Label: "server"}, {Label: "editor"}},
	}
}

func TestStartRunsHooksAroundOneCreateAndFocusesFirstWindow(t *testing.T) {
	var events []string
	manager := New(&fakeRex{events: &events}, fakeShell{events: &events})

	id, err := manager.Start(context.Background(), "demo", project())
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if id != "session:new" {
		t.Errorf("Start() id = %q", id)
	}
	want := []string{
		"find demo",
		`hook /project session="" before start`,
		"create demo windows=2",
		"focus session:new window:1",
		`hook /project session="session:new" after start`,
	}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events:\n%s\nwant:\n%s", strings.Join(events, "\n"), strings.Join(want, "\n"))
	}
}

func TestStartLeavesExistingSessionAlone(t *testing.T) {
	var events []string
	client := &fakeRex{events: &events, sessions: []rex.Session{{ID: "session:old", Label: "demo"}}}
	manager := New(client, fakeShell{events: &events})

	id, err := manager.Start(context.Background(), "demo", project())
	if err != nil || id != "session:old" {
		t.Fatalf("Start() = %q, %v", id, err)
	}
	if want := []string{"find demo"}; !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %q, want %q", events, want)
	}
}

func TestStartDoesNotCreateSessionWhenBeforeStartFails(t *testing.T) {
	var events []string
	manager := New(&fakeRex{events: &events}, fakeShell{events: &events, fail: "before start"})

	if _, err := manager.Start(context.Background(), "demo", project()); err == nil {
		t.Fatal("Start() error = nil")
	}
	if slices.ContainsFunc(events, func(event string) bool { return strings.HasPrefix(event, "create") }) {
		t.Fatalf("session created after failed hook: %q", events)
	}
}

func TestStopKeepsSessionWhenBeforeStopFails(t *testing.T) {
	var events []string
	client := &fakeRex{events: &events, sessions: []rex.Session{{ID: "session:old", Label: "demo"}}}
	manager := New(client, fakeShell{events: &events, fail: "before stop"})

	if err := manager.Stop(context.Background(), "demo", project()); err == nil {
		t.Fatal("Stop() error = nil")
	}
	if slices.Contains(events, "kill session:old") {
		t.Fatalf("session killed after failed hook: %q", events)
	}
}

func TestStopRunsHooksAroundKill(t *testing.T) {
	var events []string
	client := &fakeRex{events: &events, sessions: []rex.Session{{ID: "session:old", Label: "demo"}}}
	manager := New(client, fakeShell{events: &events})

	if err := manager.Stop(context.Background(), "demo", project()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	want := []string{
		"find demo",
		`hook /project session="session:old" before stop`,
		"kill session:old",
		`hook /project session="" after stop`,
	}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events:\n%s\nwant:\n%s", strings.Join(events, "\n"), strings.Join(want, "\n"))
	}
}

func TestLayoutMapsSplitsAndCommands(t *testing.T) {
	ratio := 0.6
	cfg := &config.Config{Windows: []config.Window{{
		Label: "server",
		Split: config.SplitRight,
		Ratio: &ratio,
		Panes: []config.Pane{
			{Label: "app", Root: "/srv", Commands: []string{"make", "go run ."}},
			{Split: config.SplitDown, Panes: []config.Pane{{Root: "/a"}, {Root: "/b"}}},
		},
	}}}

	layout := Layout("demo", cfg).Windows[0].Layout
	if layout.Split == nil || layout.Split.Direction != rex.DirectionHorizontal || *layout.Split.Ratio != 0.6 {
		t.Fatalf("root split = %+v", layout.Split)
	}
	app := layout.Split.Before.Block
	if app.Label != "app" || app.Options.Cwd != "/srv" || app.Options.InitialInput != "make\ngo run .\n" {
		t.Errorf("app block = %+v", app)
	}
	nested := layout.Split.After.Split
	if nested.Direction != rex.DirectionVertical || nested.Ratio != nil || nested.After.Block.Options.Cwd != "/b" {
		t.Errorf("nested split = %+v", nested)
	}
	if nested.Before.Block.Options.InitialInput != "" {
		t.Errorf("pane without commands has input %q", nested.Before.Block.Options.InitialInput)
	}
}

func TestLayoutChangesIntoHomePathsFromTheShell(t *testing.T) {
	cfg := &config.Config{Windows: []config.Window{
		{Label: "app", Root: "~/Code/it's here", Commands: []string{"make"}},
		{Label: "home", Root: "~"},
	}}

	request := Layout("demo", cfg)
	app := request.Windows[0].Layout.Block.Options
	if app.Cwd != "" || app.InitialInput != "cd ~/'Code/it'\\''s here'\nmake\n" {
		t.Errorf("app options = %+v", app)
	}
	home := request.Windows[1].Layout.Block.Options
	if home.Cwd != "" || home.InitialInput != "" {
		t.Errorf("home options = %+v", home)
	}
}

func TestEnvironmentReplacesCallerRexContext(t *testing.T) {
	env := Environment([]string{"PATH=/bin", "REX_SESSION=mine", "REX_BLOCK=mine", "REX_SERVER=studio"}, "demo", "session:new")
	want := []string{"PATH=/bin", "REX_SERVER=studio", "AMBER_PROJECT=demo", "REX_SESSION=session:new"}
	if !reflect.DeepEqual(env, want) {
		t.Errorf("Environment() = %q, want %q", env, want)
	}
	if env := Environment([]string{"REX_SESSION=mine"}, "demo", ""); slices.Contains(env, "REX_SESSION=mine") {
		t.Errorf("Environment() kept caller session: %q", env)
	}
}
