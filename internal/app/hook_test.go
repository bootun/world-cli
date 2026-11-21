package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bootun/world-cli/internal/config"
)

func TestHookCLIEndToEnd(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".gemini", "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}

	a, out, errBuf, store := newTestApp(t, "")
	a.homeDir = func() (string, error) { return home, nil }
	a.workDir = func() (string, error) { return work, nil }
	a.execPath = func() (string, error) { return "/Users/dev/.gvm/bin/world", nil }

	// Install auto-detected targets using resolved absolute executable path.
	code := a.Run(context.Background(), []string{"hook", "install", "--json"})
	if code != exitOK {
		t.Fatalf("hook install failed (%d): %s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), `"target": "antigravity"`) || !strings.Contains(out.String(), `"target": "claude"`) {
		t.Fatalf("expected antigravity and claude installed, got: %s", out.String())
	}
	agyHooksRaw, err := os.ReadFile(filepath.Join(home, ".gemini", "config", "hooks.json"))
	if err != nil || !strings.Contains(string(agyHooksRaw), "/Users/dev/.gvm/bin/world hook session-start") {
		t.Fatalf("expected absolute world binary path in hooks.json, got %q (err=%v)", string(agyHooksRaw), err)
	}

	// Status shows installed.
	out.Reset()
	errBuf.Reset()
	code = a.Run(context.Background(), []string{"hook", "status"})
	if code != exitOK {
		t.Fatalf("hook status failed (%d): %s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "installed (SessionStart, Stop)") {
		t.Fatalf("unexpected hook status output: %s", out.String())
	}

	// Unauthenticated session-start silently returns {}.
	out.Reset()
	errBuf.Reset()
	a.stdin = strings.NewReader(`{"hookEventName":"SessionStart"}`)
	code = a.Run(context.Background(), []string{"hook", "session-start"})
	if code != exitOK || strings.TrimSpace(out.String()) != "{}" {
		t.Fatalf("expected empty JSON when unauthenticated, got code=%d out=%q err=%q", code, out.String(), errBuf.String())
	}

	// Save API key, then verify authenticated session-start and stop callbacks.
	if err := store.Save(config.File{
		ActiveProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {APIKey: "wm_k_test_key"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	a.stdin = strings.NewReader(`{"hookEventName":"SessionStart","conversationId":"c1"}`)
	code = a.Run(context.Background(), []string{"hook", "session-start"})
	if code != exitOK {
		t.Fatalf("authenticated session-start failed (%d): %s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), `"injectSteps"`) || !strings.Contains(out.String(), "world recall") {
		t.Fatalf("unexpected session-start output: %s", out.String())
	}

	out.Reset()
	a.stdin = strings.NewReader(`{"hook_event_name":"Stop","stop_hook_active":false,"prompt":"我们决定把上线时间调整到下周三，并由张三负责最终验收。","last_assistant_message":"明白，已经记录上线时间调整和验收分工。"}`)
	code = a.Run(context.Background(), []string{"hook", "stop"})
	if code != exitOK {
		t.Fatalf("authenticated stop failed (%d): %s", code, errBuf.String())
	}
	var stopResp map[string]any
	if err := json.Unmarshal(out.Bytes(), &stopResp); err != nil {
		t.Fatalf("invalid stop JSON: %v (%s)", err, out.String())
	}
	if stopResp["decision"] != "block" {
		t.Fatalf("expected block decision, got %v", stopResp)
	}

	// Uninstall cleans up.
	out.Reset()
	code = a.Run(context.Background(), []string{"hook", "uninstall", "--json"})
	if code != exitOK {
		t.Fatalf("hook uninstall failed (%d): %s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), `"uninstalled"`) {
		t.Fatalf("unexpected uninstall output: %s", out.String())
	}
}
