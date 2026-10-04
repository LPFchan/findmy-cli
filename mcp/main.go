// findmy-mcp serves the installed Accessibility CLI over private Streamable HTTP.
package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type runner func(context.Context, string, ...string) ([]byte, error)
type service struct {
	cli, helper string
	run         runner
	gate        chan struct{}
	timeout     time.Duration
}
type lookup struct {
	Name string `json:"name" jsonschema:"Person, device, or item name; case-insensitive partial names are supported"`
	Zoom bool   `json:"zoom,omitempty" jsonschema:"Select the matching row to read its accessible detail pane"`
}

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	listen := flag.String("listen", "127.0.0.1:8786", "HTTP listen address")
	cli := flag.String("cli", filepath.Join(home, ".local/bin/findmy"), "Installed CLI path")
	helper := flag.String("helper", filepath.Join(home, ".local/bin/findmy-helper"), "Installed AX helper path")
	tokenFile := flag.String("token-file", "", "File containing a private bearer token (required)")
	publicHost := flag.String("public-host", "", "Exact HTTPS proxy host, including its port")
	flag.Parse()
	if *tokenFile == "" {
		log.Fatal("--token-file is required")
	}
	raw, err := os.ReadFile(*tokenFile)
	if err != nil {
		log.Fatal(err)
	}
	token := strings.TrimSpace(string(raw))
	if len(token) < 32 {
		log.Fatal("bearer token must be at least 32 characters")
	}
	s := &service{cli: *cli, helper: *helper, run: runCommand, gate: make(chan struct{}, 1), timeout: 45 * time.Second}
	handler, err := s.handler(token, *publicHost)
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	server := &http.Server{Addr: *listen, Handler: handler, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: time.Minute, BaseContext: func(net.Listener) context.Context { return ctx }}
	shutdownDone := make(chan struct{})
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
		close(shutdownDone)
	}()
	log.Printf("Find My MCP listening on %s; location tools only", *listen)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
	<-shutdownDone
}

func runCommand(ctx context.Context, path string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// The CLI starts a Swift helper; cancellation must stop both before another
	// request switches Find My's UI.
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if err == syscall.ESRCH {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.WaitDelay = 2 * time.Second
	return cmd.CombinedOutput()
}

func (s *service) call(ctx context.Context, path string, args ...string) (*mcp.CallToolResult, any, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	select {
	case s.gate <- struct{}{}:
		defer func() { <-s.gate }()
	case <-ctx.Done():
		return failure("Location query timed out while waiting for Find My"), nil, nil
	}
	out, err := s.run(ctx, path, args...)
	if err != nil {
		message := strings.TrimSpace(string(out))
		if message == "" {
			message = err.Error()
		}
		return failure(message), nil, nil
	}
	if !json.Valid(out) {
		return failure("Find My returned invalid JSON"), nil, nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(out)}}}, nil, nil
}
func failure(message string) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: message}}}
}
func validName(name string) error {
	if strings.TrimSpace(name) == "" || len(name) > 200 || strings.HasPrefix(name, "-") || strings.ContainsAny(name, "\x00\r\n") {
		return fmt.Errorf("name must be 1–200 bytes, without control characters or a leading dash")
	}
	return nil
}
func (s *service) handler(token, publicHost string) (http.Handler, error) {
	server := mcp.NewServer(&mcp.Implementation{Name: "findmy", Version: "1.0.0"}, nil)
	annotations := &mcp.ToolAnnotations{ReadOnlyHint: true}
	for _, kind := range []string{"people", "devices", "items"} {
		mcp.AddTool(server, &mcp.Tool{Name: "findmy_" + kind, Description: "Read the " + kind + " already visible to this Mac's Apple ID in Find My. Returns location and staleness; does not ring devices or record history.", Annotations: annotations}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
			return s.call(ctx, s.cli, kind, "--json", "--no-log")
		})
	}
	for _, kind := range []string{"person", "device", "item"} {
		mcp.AddTool(server, &mcp.Tool{Name: "findmy_" + kind, Description: "Look up one " + kind + " already visible to this Mac's Apple ID in Find My. Returns location and staleness; does not ring devices.", Annotations: annotations}, func(ctx context.Context, _ *mcp.CallToolRequest, in lookup) (*mcp.CallToolResult, any, error) {
			name := strings.TrimSpace(in.Name)
			if err := validName(name); err != nil {
				return failure(err.Error()), nil, nil
			}
			if in.Zoom && kind == "item" {
				return failure("item zoom is not supported"), nil, nil
			}
			args := []string{kind, name, "--json"}
			if in.Zoom && kind != "item" {
				args = append(args, "--zoom")
			}
			return s.call(ctx, s.cli, args...)
		})
	}
	mcp.AddTool(server, &mcp.Tool{Name: "findmy_status", Description: "Check the installed helper's Accessibility permission without opening Find My or reading locations.", Annotations: annotations}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		return s.call(ctx, s.helper, "permissions")
	})
	// Our explicit Host allowlist below covers the private HTTPS reverse proxy,
	// whose Host header is intentionally not localhost.
	transport := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, DisableLocalhostProtection: true, MaxRequestBodyBytes: 64 << 10, PropagateRequestCancellation: true})
	mux := http.NewServeMux()
	mux.Handle("/mcp", transport)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"ringing":false}`))
	})
	protection := http.NewCrossOriginProtection()
	if publicHost != "" {
		if err := protection.AddTrustedOrigin("https://" + publicHost); err != nil {
			return nil, err
		}
	}
	protected := protection.Handler(mux)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		ip := net.ParseIP(host)
		if host != "localhost" && (ip == nil || !ip.IsLoopback()) && (publicHost == "" || r.Host != publicHost) {
			http.Error(w, "unrecognized host", http.StatusForbidden)
			return
		}
		auth := r.Header.Get("Authorization")
		if subtle.ConstantTimeCompare([]byte(auth), []byte("Bearer "+token)) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		protected.ServeHTTP(w, r)
	}), nil
}
