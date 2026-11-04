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
	events     *[]string
	sessions   []rex.Session
	failCreate string
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
	if request.Label == f.failCreate {
		return rex.CreatedSession{}, errors.New("create failed")
	}
	created := rex.CreatedSession{ID: "session:" + request.Label}
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
	hooks := func(prefix string) config.Hooks {
		return config.Hooks{
			BeforeStart: []string{prefix + " before start"},
			AfterStart:  []string{prefix + " after start"},
			BeforeStop:  []string{prefix + " before stop"},
			AfterStop:   []string{prefix + " after stop"},
		}
	}
	return &config.Config{
		Root:  "/project",
		Hooks: hooks("project"),
		Sessions: []config.Session{
			{Label: "api", Root: "/project/api", Hooks: hooks("api"), Tabs: []config.Tab{{Label: "server"}, {Label: "editor"}}},
			{Label: "web", Root: "/project/web", Hooks: hooks("web"), Tabs: []config.Tab{{Label: "editor"}}},
		},
	}
}

func assertEvents(t *testing.T, events, want []string) {
	t.Helper()
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events:\n%s\nwant:\n%s", strings.Join(events, "\n"), strings.Join(want, "\n"))
	}
}

func TestStartCreatesEachSessionThenRunsAfterStartHooks(t *testing.T) {
	var events []string
	manager := New(&fakeRex{events: &events}, fakeShell{events: &events})

	id, err := manager.Start(context.Background(), "demo", project())
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if id != "session:api" {
		t.Errorf("Start() id = %q, want first session", id)
	}
	assertEvents(t, events, []string{
		"find api",
		"find web",
		`hook /project session="" project before start`,
		`hook /project/api session="" api before start`,
		"create api windows=2",
		"focus session:api window:1",
		`hook /project/web session="" web before start`,
		"create web windows=1",
		`hook /project/api session="session:api" api after start`,
		`hook /project/web session="session:web" web after start`,
		`hook /project session="" project after start`,
	})
}

func TestStartOnlyCreatesMissingSessions(t *testing.T) {
	var events []string
	client := &fakeRex{events: &events, sessions: []rex.Session{{ID: "session:old", Label: "api"}}}
	manager := New(client, fakeShell{events: &events})

	id, err := manager.Start(context.Background(), "demo", project())
	if err != nil || id != "session:old" {
		t.Fatalf("Start() = %q, %v", id, err)
	}
	if slices.Contains(events, "create api windows=2") || !slices.Contains(events, "create web windows=1") {
		t.Fatalf("events = %q", events)
	}
}

func TestStartLeavesRunningProjectAlone(t *testing.T) {
	var events []string
	client := &fakeRex{events: &events, sessions: []rex.Session{{ID: "session:1", Label: "api"}, {ID: "session:2", Label: "web"}}}
	manager := New(client, fakeShell{events: &events})

	id, err := manager.Start(context.Background(), "demo", project())
	if err != nil || id != "session:1" {
		t.Fatalf("Start() = %q, %v", id, err)
	}
	assertEvents(t, events, []string{"find api", "find web"})
}

func TestStartKillsCreatedSessionsWhenALaterSessionFails(t *testing.T) {
	for name, fail := range map[string]struct{ hook, create string }{
		"before_start hook": {hook: "web before start"},
		"create":            {create: "web"},
	} {
		t.Run(name, func(t *testing.T) {
			var events []string
			manager := New(&fakeRex{events: &events, failCreate: fail.create}, fakeShell{events: &events, fail: fail.hook})

			if _, err := manager.Start(context.Background(), "demo", project()); err == nil {
				t.Fatal("Start() error = nil")
			}
			if !slices.Contains(events, "kill session:api") {
				t.Errorf("created session not killed: %q", events)
			}
			if slices.ContainsFunc(events, func(event string) bool { return strings.Contains(event, "after start") }) {
				t.Errorf("after_start hook ran after failure: %q", events)
			}
		})
	}
}

func TestStopRunsHooksAroundEachRunningSession(t *testing.T) {
	var events []string
	client := &fakeRex{events: &events, sessions: []rex.Session{{ID: "session:web", Label: "web"}}}
	manager := New(client, fakeShell{events: &events})

	if err := manager.Stop(context.Background(), "demo", project()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	assertEvents(t, events, []string{
		"find api",
		"find web",
		`hook /project session="" project before stop`,
		`hook /project/web session="session:web" web before stop`,
		"kill session:web",
		`hook /project/web session="" web after stop`,
		`hook /project session="" project after stop`,
	})
}

func TestStopKeepsSessionsWhenBeforeStopFails(t *testing.T) {
	for _, hook := range []string{"project before stop", "api before stop"} {
		var events []string
		client := &fakeRex{events: &events, sessions: []rex.Session{{ID: "session:api", Label: "api"}}}
		manager := New(client, fakeShell{events: &events, fail: hook})

		if err := manager.Stop(context.Background(), "demo", project()); err == nil {
			t.Fatalf("Stop() with failing %q error = nil", hook)
		}
		if slices.Contains(events, "kill session:api") {
			t.Fatalf("session killed after failed %q: %q", hook, events)
		}
	}
}

func TestLayoutMapsSplitsAndCommands(t *testing.T) {
	ratio := 0.6
	session := config.Session{Label: "demo", Tabs: []config.Tab{{
		Label: "server",
		Layout: config.Layout{
			Split: config.SplitRight,
			Ratio: &ratio,
			Panes: []config.Pane{
				{Label: "app", Layout: config.Layout{Root: "/srv", Commands: []string{"make", "go run ."}}},
				{Layout: config.Layout{Split: config.SplitDown, Panes: []config.Pane{
					{Layout: config.Layout{Root: "/a"}},
					{Layout: config.Layout{Root: "/b"}},
				}}},
			},
		},
	}}}

	layout := Layout(session).Windows[0].Layout
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
	session := config.Session{Label: "demo", Tabs: []config.Tab{
		{Label: "app", Layout: config.Layout{Root: "~/Code/it's here", Commands: []string{"make"}}},
		{Label: "home", Layout: config.Layout{Root: "~"}},
	}}

	request := Layout(session)
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
