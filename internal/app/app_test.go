package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bootun/world-cli/internal/client"
	"github.com/bootun/world-cli/internal/config"
	"github.com/bootun/world-cli/internal/version"
)

func newTestApp(t *testing.T, stdin string) (*App, *bytes.Buffer, *bytes.Buffer, *config.Store) {
	t.Helper()
	store := config.NewStore(filepath.Join(t.TempDir(), "world", "config.json"))
	var out, errBuf bytes.Buffer
	application := New(store, strings.NewReader(stdin), &out, &errBuf)
	return application, &out, &errBuf, store
}

func TestRememberRecallAndEntityFlow(t *testing.T) {
	var seenRemember map[string]any
	var seenIdemKey string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get(client.HeaderCLIVersion); got != version.Version {
			t.Errorf("%s: %s = %q, want %q", r.URL.Path, client.HeaderCLIVersion, got, version.Version)
		}
		if got, want := r.Header.Get("User-Agent"), "world-cli/"+version.Version; got != want {
			t.Errorf("%s: User-Agent = %q, want %q", r.URL.Path, got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/healthz":
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/v1/entities/__world_cli_auth_probe__":
			if r.Header.Get("Authorization") != "Bearer wm_k_secret123456" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":{"code":"unauthenticated","message":"bad key"}}`))
				return
			}
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":"not_found","message":"missing"}}`))
		case "/v1/memories":
			seenIdemKey = r.Header.Get("Idempotency-Key")
			_ = json.NewDecoder(r.Body).Decode(&seenRemember)
			_, _ = w.Write([]byte(`{"eventId":"evt_001","deduplicated":false}`))
		case "/v1/memories:recall":
			if r.URL.Query().Get("query") != "Atlas" || r.URL.Query().Get("asOfValidTimeMs") == "" {
				t.Fatalf("unexpected recall query: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"entities":[{"entityId":"ent_1","canonicalName":"Atlas","entityType":"Project"}],"memories":[{"eventId":"evt_001","occurredAtMs":"1731920000000","summary":"Project Atlas"}]}`))
		case "/v1/entities/ent_1":
			_, _ = w.Write([]byte(`{"entity":{"entityId":"ent_1","canonicalName":"Atlas","entityType":"Project"}}`))
		case "/v1/memories/evt_001:retract":
			_, _ = w.Write([]byte(`{"eventId":"evt_001","status":"retracted","deduplicated":false}`))
		case "/v1/memories/evt_001:supersede":
			_, _ = w.Write([]byte(`{"oldEventId":"evt_001","newEventId":"evt_002","deduplicated":false}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	a, out, errBuf, _ := newTestApp(t, "")
	code := a.Run(context.Background(), []string{"auth", "login", "--endpoint", srv.URL, "--api-key", "wm_k_secret123456", "--json"})
	if code != exitOK {
		t.Fatalf("login failed (%d): %s", code, errBuf.String())
	}
	if strings.Contains(out.String(), "wm_k_secret123456") {
		t.Fatalf("login output leaked full key: %s", out.String())
	}

	out.Reset()
	errBuf.Reset()
	code = a.Run(context.Background(), []string{"auth", "status", "--json"})
	if code != exitOK {
		t.Fatalf("status failed (%d): %s %s", code, out.String(), errBuf.String())
	}
	if !strings.Contains(out.String(), `"validKey"`) && !strings.Contains(out.String(), `"authenticated": true`) {
		t.Fatalf("expected authenticated=true in status output: %s", out.String())
	}

	out.Reset()
	errBuf.Reset()
	code = a.Run(context.Background(), []string{
		"remember", "Project codename is Atlas",
		"--source", "cli-test",
		"--timezone", "Asia/Shanghai",
		"--occurred-at", "2026-11-18",
		"--meta", "env=dev",
		"--json",
	})
	if code != exitOK {
		t.Fatalf("remember failed (%d): %s", code, errBuf.String())
	}
	if seenIdemKey == "" {
		t.Fatal("expected automatic Idempotency-Key header")
	}
	if seenRemember["timezone"] != "Asia/Shanghai" || seenRemember["source"] != "cli-test" {
		t.Fatalf("unexpected remember payload: %#v", seenRemember)
	}
	if !strings.Contains(out.String(), `"eventId": "evt_001"`) || !strings.Contains(out.String(), `"deduplicated": false`) {
		t.Fatalf("unexpected remember output: %s", out.String())
	}

	out.Reset()
	code = a.Run(context.Background(), []string{"recall", "Atlas", "--as-of", "2026-11-18", "--timezone", "Asia/Shanghai", "--json"})
	if code != exitOK {
		t.Fatalf("recall failed (%d): %s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), `"canonicalName": "Atlas"`) || !strings.Contains(out.String(), `"eventId": "evt_001"`) {
		t.Fatalf("unexpected recall output: %s", out.String())
	}

	out.Reset()
	code = a.Run(context.Background(), []string{"entity", "get", "ent_1", "--json"})
	if code != exitOK {
		t.Fatalf("entity get failed (%d): %s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), `"entityId": "ent_1"`) {
		t.Fatalf("unexpected entity output: %s", out.String())
	}

	out.Reset()
	code = a.Run(context.Background(), []string{"forget", "evt_001", "--reason", "test", "--redact", "--json"})
	if code != exitOK {
		t.Fatalf("forget failed (%d): %s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), `"status": "retracted"`) {
		t.Fatalf("unexpected forget output: %s", out.String())
	}

	out.Reset()
	code = a.Run(context.Background(), []string{"fix", "evt_001", "Project codename is Helios", "--json"})
	if code != exitOK {
		t.Fatalf("fix failed (%d): %s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), `"newEventId": "evt_002"`) {
		t.Fatalf("unexpected fix output: %s", out.String())
	}
}

func TestRememberFromStdinAndConfigSetGet(t *testing.T) {
	var seenText string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		seenText, _ = body["text"].(string)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"eventId":"evt_stdin","deduplicated":false}`))
	}))
	defer srv.Close()

	a, out, errBuf, _ := newTestApp(t, "Piped memory content\n")
	if code := a.Run(context.Background(), []string{"config", "set", "timezone", "Asia/Shanghai"}); code != exitOK {
		t.Fatalf("config set timezone failed: %s", errBuf.String())
	}
	if code := a.Run(context.Background(), []string{"config", "set", "endpoint", srv.URL}); code != exitOK {
		t.Fatalf("config set endpoint failed: %s", errBuf.String())
	}
	if code := a.Run(context.Background(), []string{"auth", "login", "--api-key", "wm_k_stdin"}); code != exitOK {
		t.Fatalf("login failed: %s", errBuf.String())
	}
	out.Reset()
	if code := a.Run(context.Background(), []string{"remember", "-"}); code != exitOK {
		t.Fatalf("remember stdin failed: %s", errBuf.String())
	}
	if seenText != "Piped memory content" {
		t.Fatalf("seenText = %q, want Piped memory content", seenText)
	}
	if strings.TrimSpace(out.String()) != "Recorded memory (evt_stdin)" {
		t.Fatalf("expected 'Recorded memory (evt_stdin)' in stdout, got %q", out.String())
	}
}

func TestBrowserCallbackLogin(t *testing.T) {
	a, out, errBuf, store := newTestApp(t, "")
	a.openBrowser = func(string) error { return nil }

	done := make(chan struct{})
	a.onAuthReady = func(authURL, callbackURL, state string) {
		go func() {
			defer close(done)
			// Verify CORS preflight including Private Network Access header.
			preReq, _ := http.NewRequest(http.MethodOptions, callbackURL, nil)
			preReq.Header.Set("Origin", "https://console.worldmachine.dev")
			preRes, err := http.DefaultClient.Do(preReq)
			if err != nil {
				t.Errorf("preflight error: %v", err)
				return
			}
			_ = preRes.Body.Close()
			if preRes.Header.Get("Access-Control-Allow-Private-Network") != "true" {
				t.Errorf("missing PNA header: %v", preRes.Header)
			}

			payload, _ := json.Marshal(map[string]string{
				"secret":    "wm_k_browser_callback_secret",
				"state":     state,
				"email":     "dev@example.com",
				"keyName":   "world-cli-mac",
				"keyPrefix": "wm_k_bro",
			})
			req, _ := http.NewRequest(http.MethodPost, callbackURL, bytes.NewReader(payload))
			req.Header.Set("Origin", "https://console.worldmachine.dev")
			req.Header.Set("Content-Type", "application/json")
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Errorf("callback post error: %v", err)
				return
			}
			_ = res.Body.Close()
			if res.StatusCode != http.StatusOK {
				t.Errorf("callback status = %d", res.StatusCode)
			}
		}()
	}

	code := a.Run(context.Background(), []string{"auth", "login", "--timeout", (5 * time.Second).String(), "--json"})
	<-done
	if code != exitOK {
		t.Fatalf("callback login failed (%d): %s %s", code, out.String(), errBuf.String())
	}
	f, err := store.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := f.Profiles[config.DefaultProfile]
	if p.APIKey != "wm_k_browser_callback_secret" || p.Email != "dev@example.com" || p.KeyName != "world-cli-mac" {
		t.Fatalf("saved profile mismatch: %+v", p)
	}
}
