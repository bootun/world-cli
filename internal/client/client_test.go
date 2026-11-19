package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
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
			_, _ = w.Write([]byte(`{"entities":[{"entityId":"ent_1","canonicalName":"Atlas","entityType":"Project","components":{"status":{"componentType":"Status","schemaVersion":"1","payload":{"state":"active"},"validFromMs":"1731920000000","validToMs":"0","epistemicStatus":"stated_claim"}}}],"memories":[{"eventId":"evt_1","occurredAtMs":"1731920000000","eventType":"note","summary":"Project Atlas"}]}`))
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
