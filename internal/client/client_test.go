package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/bootun/world-cli/internal/version"
)

func TestWriteMemoryRetriesWithSameIdempotencyKeyAndStringInt64(t *testing.T) {
	var calls atomic.Int32
	var seenKeys []string
	var seenBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if r.URL.Path != "/v1/memories" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer wm_k_test" {
			t.Fatalf("Authorization = %q", got)
		}
		seenKeys = append(seenKeys, r.Header.Get("Idempotency-Key"))
		if err := json.NewDecoder(r.Body).Decode(&seenBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if n == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":{"code":"unavailable","message":"busy"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"eventId":"evt_123","deduplicated":false}`))
	}))
	defer srv.Close()

	occurred := Int64(1731920000000)
	c := New(srv.URL, "wm_k_test")
	res, err := c.WriteMemory(context.Background(), WriteMemoryRequest{
		Text:           "Project codename is Atlas",
		OccurredAtMs:   &occurred,
		Source:         "cli",
		Timezone:       "Asia/Shanghai",
		IdempotencyKey: "idem-abc",
	})
	if err != nil {
		t.Fatalf("WriteMemory: %v", err)
	}
	if res.EventID != "evt_123" || res.Deduplicated {
		t.Fatalf("unexpected response: %+v", res)
	}
	if calls.Load() != 2 || seenKeys[0] != "idem-abc" || seenKeys[1] != "idem-abc" {
		t.Fatalf("expected 2 calls with same key, got calls=%d keys=%v", calls.Load(), seenKeys)
	}
	if seenBody["occurredAtMs"] != "1731920000000" {
		t.Fatalf("occurredAtMs must be JSON string, got %#v", seenBody["occurredAtMs"])
	}
}

func TestRecallAndGetEntityAndVerifyAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/memories:recall":
			q := r.URL.Query()
			if q.Get("query") != "Atlas" || q.Get("maxEntities") != "5" || q.Get("asOfValidTimeMs") != "1731920000000" {
				t.Fatalf("unexpected recall query: %v", q)
			}
			_, _ = w.Write([]byte(`{"entities":[{"entityId":"ent_1","canonicalName":"Atlas","entityType":"Project","components":{"status":{"componentType":"Status","schemaVersion":"1","payload":{"state":"active"},"validFromMs":"1731920000000","validToMs":"0","epistemicStatus":"stated_claim"}}}],"memories":[{"eventId":"evt_1","occurredAtMs":"1731920000000","summary":"Project Atlas"}]}`))
		case "/v1/entities/ent_1":
			if r.URL.Query().Get("asOfValidTimeMs") != "1731920000000" {
				t.Fatalf("unexpected entity query: %v", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"entity":{"entityId":"ent_1","canonicalName":"Atlas","entityType":"Project"}}`))
		case "/v1/entities/__world_cli_auth_probe__":
			if r.Header.Get("Authorization") == "Bearer bad" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":{"code":"unauthenticated","message":"invalid key"}}`))
				return
			}
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":"not_found","message":"missing"}}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := New(srv.URL, "good")
	rec, err := c.Recall(context.Background(), RecallRequest{Query: "Atlas", MaxEntities: 5, AsOfValidTimeMs: 1731920000000})
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(rec.Entities) != 1 || rec.Entities[0].Components["status"].ValidFromMs != 1731920000000 {
		t.Fatalf("unexpected recall response: %+v", rec)
	}

	ent, err := c.GetEntity(context.Background(), "ent_1", 1731920000000)
	if err != nil || ent.Entity == nil || ent.Entity.CanonicalName != "Atlas" {
		t.Fatalf("GetEntity = %+v, err=%v", ent, err)
	}

	if err := c.VerifyAuth(context.Background()); err != nil {
		t.Fatalf("VerifyAuth valid key failed: %v", err)
	}

	bad := New(srv.URL, "bad")
	err = bad.VerifyAuth(context.Background())
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "unauthenticated" {
		t.Fatalf("expected unauthenticated error, got %v", err)
	}
}

func TestRetractAndSupersedeMemory(t *testing.T) {
	var seenRetractBody map[string]any
	var seenSupersedeBody map[string]any
	var retractKey, supersedeKey string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/memories/evt_bad:retract":
			retractKey = r.Header.Get("Idempotency-Key")
			_ = json.NewDecoder(r.Body).Decode(&seenRetractBody)
			_, _ = w.Write([]byte(`{"eventId":"evt_bad","status":"retracted","deduplicated":false}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/memories/evt_bad:supersede":
			supersedeKey = r.Header.Get("Idempotency-Key")
			_ = json.NewDecoder(r.Body).Decode(&seenSupersedeBody)
			_, _ = w.Write([]byte(`{"oldEventId":"evt_bad","newEventId":"evt_good","deduplicated":false}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := New(srv.URL, "wm_k_test")
	rRes, err := c.RetractMemory(context.Background(), RetractMemoryRequest{
		EventID:        "evt_bad",
		Reason:         "test noise",
		RedactText:     true,
		IdempotencyKey: "ret-1",
	})
	if err != nil {
		t.Fatalf("RetractMemory: %v", err)
	}
	if rRes.EventID != "evt_bad" || rRes.Status != "retracted" || retractKey != "ret-1" || seenRetractBody["redactText"] != true {
		t.Fatalf("unexpected retract result: %+v body=%v", rRes, seenRetractBody)
	}

	occ := Int64(1731920000000)
	sRes, err := c.SupersedeMemory(context.Background(), SupersedeMemoryRequest{
		EventID:        "evt_bad",
		Text:           "我的工作地点在上海",
		OccurredAtMs:   &occ,
		Timezone:       "Asia/Shanghai",
		Reason:         "fix city",
		RedactOldText:  true,
		IdempotencyKey: "sup-1",
	})
	if err != nil {
		t.Fatalf("SupersedeMemory: %v", err)
	}
	if sRes.OldEventID != "evt_bad" || sRes.NewEventID != "evt_good" || supersedeKey != "sup-1" || seenSupersedeBody["text"] != "我的工作地点在上海" {
		t.Fatalf("unexpected supersede result: %+v body=%v", sRes, seenSupersedeBody)
	}
}

func TestAllRequestsIncludeCLIVersionHeaders(t *testing.T) {
	var seen []struct {
		path    string
		ua      string
		version string
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, struct {
			path    string
			ua      string
			version string
		}{
			path:    r.URL.Path,
			ua:      r.Header.Get("User-Agent"),
			version: r.Header.Get(HeaderCLIVersion),
		})
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/healthz":
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/v1/memories":
			_, _ = w.Write([]byte(`{"eventId":"evt_1","deduplicated":false}`))
		case "/v1/memories:recall":
			_, _ = w.Write([]byte(`{"entities":[],"memories":[]}`))
		case "/v1/entities/ent_1":
			_, _ = w.Write([]byte(`{"entity":{"entityId":"ent_1","canonicalName":"Atlas","entityType":"Project"}}`))
		case "/v1/entities/__world_cli_auth_probe__":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":"not_found","message":"missing"}}`))
		case "/v1/memories/evt_1:retract":
			_, _ = w.Write([]byte(`{"eventId":"evt_1","status":"retracted","deduplicated":false}`))
		case "/v1/memories/evt_1:supersede":
			_, _ = w.Write([]byte(`{"oldEventId":"evt_1","newEventId":"evt_2","deduplicated":false}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := New(srv.URL, "wm_k_test")
	ctx := context.Background()
	if err := c.Health(ctx); err != nil {
		t.Fatalf("Health: %v", err)
	}
	if err := c.VerifyAuth(ctx); err != nil {
		t.Fatalf("VerifyAuth: %v", err)
	}
	if _, err := c.WriteMemory(ctx, WriteMemoryRequest{Text: "hello"}); err != nil {
		t.Fatalf("WriteMemory: %v", err)
	}
	if _, err := c.Recall(ctx, RecallRequest{Query: "hello"}); err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if _, err := c.GetEntity(ctx, "ent_1", 0); err != nil {
		t.Fatalf("GetEntity: %v", err)
	}
	if _, err := c.RetractMemory(ctx, RetractMemoryRequest{EventID: "evt_1"}); err != nil {
		t.Fatalf("RetractMemory: %v", err)
	}
	if _, err := c.SupersedeMemory(ctx, SupersedeMemoryRequest{EventID: "evt_1", Text: "fixed"}); err != nil {
		t.Fatalf("SupersedeMemory: %v", err)
	}

	if len(seen) != 7 {
		t.Fatalf("expected 7 requests, got %d", len(seen))
	}
	wantUA := "world-cli/" + version.Version
	for _, req := range seen {
		if req.version != version.Version {
			t.Errorf("%s: %s = %q, want %q", req.path, HeaderCLIVersion, req.version, version.Version)
		}
		if req.ua != wantUA {
			t.Errorf("%s: User-Agent = %q, want %q", req.path, req.ua, wantUA)
		}
	}

	// Verify explicit WithCLIVersion and WithUserAgent override behavior.
	seen = nil
	custom := New(srv.URL, "wm_k_test", WithCLIVersion("1.2.3"))
	if err := custom.Health(ctx); err != nil {
		t.Fatalf("custom Health: %v", err)
	}
	if len(seen) != 1 || seen[0].version != "1.2.3" || seen[0].ua != "world-cli/1.2.3" {
		t.Fatalf("unexpected custom version headers: %+v", seen)
	}

	seen = nil
	customUA := New(srv.URL, "wm_k_test", WithUserAgent("my-agent/9.9"), WithCLIVersion("1.2.3"))
	if err := customUA.Health(ctx); err != nil {
		t.Fatalf("customUA Health: %v", err)
	}
	if len(seen) != 1 || seen[0].version != "1.2.3" || seen[0].ua != "my-agent/9.9" {
		t.Fatalf("unexpected custom UA headers: %+v", seen)
	}
}
