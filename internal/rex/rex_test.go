package rex

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

type call struct {
	args  []string
	stdin string
}

func fakeClient(server, client string, responses map[string]string, calls *[]call) Client {
	c := Client{binary: "rex", server: server, client: client}
	c.commandRunner = func(_ context.Context, stdin []byte, args []string) ([]byte, error) {
		*calls = append(*calls, call{args: args, stdin: string(stdin)})
		for prefix, response := range responses {
			if strings.Contains(strings.Join(args, " "), prefix) {
				return []byte(response), nil
			}
		}
		return nil, nil
	}
	return c
}

func TestCreateSessionSendsLayoutInRexAPIShape(t *testing.T) {
	var calls []call
	client := fakeClient("studio", "", map[string]string{
		"session.create": `{"session_id":"session:1","initial_windows":[{"window_id":"window:1"}]}`,
	}, &calls)
	ratio := 0.6

	created, err := client.CreateSession(context.Background(), CreateSession{
		Label: "demo",
		Windows: []Window{{
			Label: "server",
			Layout: Node{Split: &Split{
				Direction: DirectionHorizontal,
				Ratio:     &ratio,
				Before:    Node{Block: &Block{Flavor: TerminalFlavor, Label: "app", Options: BlockOptions{Cwd: "/srv", InitialInput: "go run .\n"}}},
				After:     Node{Block: &Block{Flavor: TerminalFlavor, Options: BlockOptions{Cwd: "/srv"}}},
			}},
		}},
	})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if created.ID != "session:1" || created.Windows[0].ID != "window:1" {
		t.Errorf("created = %+v", created)
	}

	if want := []string{"-S", "studio", "api", "call", "session.create", "-"}; !reflect.DeepEqual(calls[0].args, want) {
		t.Errorf("args = %q, want %q", calls[0].args, want)
	}
	var got, want any
	if err := json.Unmarshal([]byte(calls[0].stdin), &got); err != nil {
		t.Fatal(err)
	}
	json.Unmarshal([]byte(`{
		"label": "demo",
		"initial_windows": [{
			"window_label": "server",
			"layout": {"split": {
				"direction": "horizontal",
				"ratio": 0.6,
				"before": {"block": {"flavor": "com.superlogical.terminal.shell", "label": "app", "options": {"cwd": "/srv", "initial_input": "go run .\n"}}},
				"after": {"block": {"flavor": "com.superlogical.terminal.shell", "options": {"cwd": "/srv"}}}
			}}
		}]
	}`), &want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("payload = %s", calls[0].stdin)
	}
}

func TestFindSessionRejectsDuplicateLabels(t *testing.T) {
	var calls []call
	client := fakeClient("", "", map[string]string{
		"ls --json": `{"sessions":[{"session_id":"s1","label":"demo"},{"session_id":"s2","label":"demo"},{"session_id":"s3","label":"other"}]}`,
	}, &calls)

	if _, _, err := client.FindSession(context.Background(), "demo"); err == nil {
		t.Fatal("FindSession(demo) error = nil, want duplicate label error")
	}
	session, found, err := client.FindSession(context.Background(), "other")
	if err != nil || !found || session.ID != "s3" {
		t.Fatalf("FindSession(other) = %+v, %v, %v", session, found, err)
	}
	if _, found, err := client.FindSession(context.Background(), "missing"); err != nil || found {
		t.Fatalf("FindSession(missing) found = %v, err = %v", found, err)
	}
}

func TestSelectSessionTargetsClient(t *testing.T) {
	var calls []call
	client := fakeClient("studio", "macbook", nil, &calls)
	if err := client.SelectSession(context.Background(), "session:1"); err != nil {
		t.Fatal(err)
	}
	want := []string{"-S", "studio", "-C", "macbook", "do", "session.select", "session_id=session:1"}
	if !reflect.DeepEqual(calls[0].args, want) {
		t.Errorf("args = %q, want %q", calls[0].args, want)
	}
}
