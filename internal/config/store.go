package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const (
	DefaultProfile    = "default"
	DefaultEndpoint   = "https://api.worldmachine.dev"
	DefaultConsoleURL = "https://console.worldmachine.dev"
)

type Profile struct {
	Endpoint   string `json:"endpoint,omitempty"`
	ConsoleURL string `json:"console_url,omitempty"`
	APIKey     string `json:"api_key,omitempty"`
	Email      string `json:"email,omitempty"`
	KeyName    string `json:"key_name,omitempty"`
	KeyPrefix  string `json:"key_prefix,omitempty"`
	Timezone   string `json:"timezone,omitempty"`
	Source     string `json:"source,omitempty"`
	Output     string `json:"output,omitempty"`
}

type File struct {
	ActiveProfile string             `json:"active_profile,omitempty"`
	Profiles      map[string]Profile `json:"profiles,omitempty"`
}

type Resolved struct {
	ProfileName string
	Endpoint    string
	ConsoleURL  string
	APIKey      string
	Email       string
	KeyName     string
	KeyPrefix   string
	Timezone    string
	Source      string
	Output      string
	ConfigPath  string
}

type Store struct {
	path string
}

func DefaultPath() (string, error) {
	if dir := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); dir != "" {
		return filepath.Join(dir, "world", "config.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "world", "config.json"), nil
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

func (s *Store) Path() string {
	return s.path
}

func (s *Store) Load() (File, error) {
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return File{ActiveProfile: DefaultProfile, Profiles: map[string]Profile{}}, nil
	}
	if err != nil {
		return File{}, err
	}
	var f File
	if err := json.Unmarshal(raw, &f); err != nil {
		return File{}, err
	}
	if f.Profiles == nil {
		f.Profiles = map[string]Profile{}
	}
	if f.ActiveProfile == "" {
		f.ActiveProfile = DefaultProfile
	}
	return f, nil
}

func (s *Store) Save(f File) error {
	if f.ActiveProfile == "" {
		f.ActiveProfile = DefaultProfile
	}
	if f.Profiles == nil {
		f.Profiles = map[string]Profile{}
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(dir, "config-*.json.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}
	if err := tmp.Chmod(0o600); err != nil {
		cleanup()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, s.path)
}

func (s *Store) Resolve(profileOverride, endpointOverride, apiKeyOverride string) (Resolved, error) {
	f, err := s.Load()
	if err != nil {
		return Resolved{}, err
	}
	name := strings.TrimSpace(profileOverride)
	if name == "" {
		name = strings.TrimSpace(os.Getenv("WORLD_PROFILE"))
	}
	if name == "" {
		name = f.ActiveProfile
	}
	if name == "" {
		name = DefaultProfile
	}

	p := f.Profiles[name]
	res := Resolved{
		ProfileName: name,
		Endpoint:    DefaultEndpoint,
		ConsoleURL:  DefaultConsoleURL,
		Output:      "text",
		ConfigPath:  s.path,
		Email:       p.Email,
		KeyName:     p.KeyName,
		KeyPrefix:   p.KeyPrefix,
	}
	if p.Endpoint != "" {
		res.Endpoint = p.Endpoint
	}
	if p.ConsoleURL != "" {
		res.ConsoleURL = p.ConsoleURL
	}
	if p.APIKey != "" {
		res.APIKey = p.APIKey
	}
	if p.Timezone != "" {
		res.Timezone = p.Timezone
	}
	if p.Source != "" {
		res.Source = p.Source
	}
	if p.Output != "" {
		res.Output = p.Output
	}

	if v := strings.TrimSpace(os.Getenv("WORLD_ENDPOINT")); v != "" {
		res.Endpoint = v
	}
	if v := strings.TrimSpace(os.Getenv("WORLD_CONSOLE_URL")); v != "" {
		res.ConsoleURL = v
	}
	if v := strings.TrimSpace(os.Getenv("WORLD_API_KEY")); v != "" {
		res.APIKey = v
	} else if v := strings.TrimSpace(os.Getenv("WM_API_KEY")); v != "" {
		res.APIKey = v
	}
	if v := strings.TrimSpace(os.Getenv("WORLD_TIMEZONE")); v != "" {
		res.Timezone = v
	}
	if v := strings.TrimSpace(os.Getenv("WORLD_SOURCE")); v != "" {
		res.Source = v
	}

	if v := strings.TrimSpace(endpointOverride); v != "" {
		res.Endpoint = v
	}
	if v := strings.TrimSpace(apiKeyOverride); v != "" {
		res.APIKey = v
	}
	res.Endpoint = strings.TrimRight(res.Endpoint, "/")
	res.ConsoleURL = strings.TrimRight(res.ConsoleURL, "/")
	return res, nil
}

func MaskKey(key string, prefix string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	if prefix != "" && strings.HasPrefix(key, prefix) {
		return prefix + "••••••"
	}
	if len(key) <= 8 {
		return key[:min(2, len(key))] + "••••••"
	}
	return key[:6] + "••••••" + key[len(key)-2:]
}
