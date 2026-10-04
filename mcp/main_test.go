package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const testToken = "test-token-with-at-least-thirty-two-characters"

type authTransport struct{ token string }

func (a authTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+a.token)
	return http.DefaultTransport.RoundTrip(r)
}
func testSession(t *testing.T, s *service) *mcp.ClientSession {
	t.Helper()
	handler, err := s.handler(testToken, "dumpling.tailaa113.ts.net:8443")
	if err != nil {
		t.Fatal(err)
	}
	h := httptest.NewServer(handler)
	t.Cleanup(h.Close)
	c := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := c.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: h.URL + "/mcp", HTTPClient: &http.Client{Transport: authTransport{testToken}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}
func TestOnlyLocationToolsAndSafeArguments(t *testing.T) {
	var paths []string
	var arguments [][]string
	s := &service{cli: "findmy", helper: "helper", gate: make(chan struct{}, 1), timeout: time.Second, run: func(_ context.Context, p string, args ...string) ([]byte, error) {
		paths = append(paths, p)
		arguments = append(arguments, append([]string{}, args...))
		return []byte(`[]`), nil
	}}
	session := testSession(t, s)
	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tool := range tools.Tools {
		names[tool.Name] = true
	}
	if len(names) != 7 {
		t.Fatalf("unexpected tools: %v", names)
	}
	for _, name := range []string{"findmy_people", "findmy_person", "findmy_devices", "findmy_device", "findmy_items", "findmy_item", "findmy_status"} {
		if !names[name] {
			t.Fatalf("missing %s", name)
		}
	}
	for _, name := range []string{"findmy_ring", "findmy_phone", "findmy_play_sound"} {
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: map[string]any{}})
		if err == nil && !result.IsError {
			t.Fatalf("action %s was accepted", name)
		}
	}
	if len(arguments) != 0 {
		t.Fatal("an unknown action reached the CLI")
	}
	_, err = session.CallTool(context.Background(), &mcp.CallToolParams{Name: "findmy_devices", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = session.CallTool(context.Background(), &mcp.CallToolParams{Name: "findmy_person", Arguments: map[string]any{"name": "Name; echo unsafe", "zoom": true}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = session.CallTool(context.Background(), &mcp.CallToolParams{Name: "findmy_status", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"devices", "--json", "--no-log"}, {"person", "Name; echo unsafe", "--json", "--zoom"}, {"permissions"}}
	if !reflect.DeepEqual(arguments, want) || !reflect.DeepEqual(paths, []string{"findmy", "findmy", "helper"}) {
		t.Fatalf("unsafe argv: %v %v", paths, arguments)
	}
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "findmy_device", Arguments: map[string]any{"name": "--confirm"}})
	if err != nil || !result.IsError || len(arguments) != 3 {
		t.Fatalf("flag-like name reached CLI: %v %v", result, err)
	}
}
func TestAuthorizationHostsAndBrowserOrigins(t *testing.T) {
	s := &service{}
	h, err := s.handler(testToken, "dumpling.tailaa113.ts.net:8443")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		host, token, origin string
		want                int
	}{
		{"127.0.0.1:8786", "", "", 401},
		{"127.0.0.1:8786", "wrong", "", 401},
		{"evil.example", testToken, "", 403},
		{"dumpling.tailaa113.ts.net:8443", testToken, "", 200},
		{"127.0.0.1:8786", testToken, "https://evil.example", 403},
	} {
		r := httptest.NewRequest("POST", "http://"+tc.host+"/healthz", strings.NewReader(""))
		if tc.token != "" {
			r.Header.Set("Authorization", "Bearer "+tc.token)
		}
		if tc.origin != "" {
			r.Header.Set("Origin", tc.origin)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Errorf("%+v: got %d", tc, w.Code)
		}
	}
}
func TestRequestsSerializeAndCanceledQueueDoesNotRun(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	s := &service{gate: make(chan struct{}, 1), timeout: time.Second, run: func(context.Context, string, ...string) ([]byte, error) {
		calls.Add(1)
		close(started)
		<-release
		return []byte(`[]`), nil
	}}
	done := make(chan struct{})
	go func() { _, _, _ = s.call(context.Background(), "findmy", "devices"); close(done) }()
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	result, _, err := s.call(ctx, "findmy", "people")
	if err != nil || !result.IsError || calls.Load() != 1 {
		t.Fatalf("canceled query ran: %v %v", result, err)
	}
	close(release)
	<-done
}

func TestTimeoutStopsChildHoldingOutputPipe(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := runCommand(ctx, "/bin/sh", "-c", "sleep 10 & wait")
	if err == nil || time.Since(start) > time.Second {
		t.Fatalf("timed-out child survived: %v after %v", err, time.Since(start))
	}
}
