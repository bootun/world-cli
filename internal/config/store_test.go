package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreSaveLoadPermissionsAndResolve(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "world", "config.json")
	store := NewStore(path)

	f, err := store.Load()
	if err != nil {
		t.Fatalf("Load missing file: %v", err)
	}
	if f.ActiveProfile != DefaultProfile {
		t.Fatalf("ActiveProfile = %q, want %q", f.ActiveProfile, DefaultProfile)
	}

	f.ActiveProfile = "work"
	f.Profiles["work"] = Profile{
		Endpoint: "https://api.example.com/",
		APIKey:   "wm_k_1234567890abcdef",
		Timezone: "Asia/Shanghai",
		Output:   "json",
	}
	if err := store.Save(f); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("file mode = %o, want 600", mode)
	}

	res, err := store.Resolve("", "", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.ProfileName != "work" || res.Endpoint != "https://api.example.com" || res.APIKey != "wm_k_1234567890abcdef" || res.Output != "json" {
		t.Fatalf("unexpected resolved: %+v", res)
	}

	t.Setenv("WORLD_API_KEY", "env-key-override")
	res, err = store.Resolve("", "http://127.0.0.1:9090/", "")
	if err != nil {
		t.Fatalf("Resolve with overrides: %v", err)
	}
	if res.APIKey != "env-key-override" || res.Endpoint != "http://127.0.0.1:9090" {
		t.Fatalf("overrides not applied: %+v", res)
	}
}

func TestMaskKey(t *testing.T) {
	if got := MaskKey("wm_k_1234567890", "wm_k_123"); got != "wm_k_123••••••" {
		t.Fatalf("MaskKey with prefix = %q", got)
	}
	if got := MaskKey("wm_k_abcdefghijklmnop", ""); got != "wm_k_a••••••op" {
		t.Fatalf("MaskKey default = %q", got)
	}
	if got := MaskKey("", ""); got != "" {
		t.Fatalf("MaskKey empty = %q", got)
	}
}
