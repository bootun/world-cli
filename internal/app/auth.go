package app

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/bootun/world-cli/internal/config"
)

type browserOpener func(string) error

func defaultBrowserOpener(target string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", target)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", target)
	default:
		cmd = exec.Command("xdg-open", target)
	}
	return cmd.Start()
}

func (a *App) runAuth(ctx context.Context, args []string, defaults commonFlags) int {
	if len(args) == 0 || isHelp(args[0]) {
		a.printAuthUsage(a.stdout)
		if len(args) == 0 {
			return exitUsage
		}
		return exitOK
	}
	sub := args[0]
	rest := args[1:]
	switch sub {
	case "login":
		return a.runAuthLogin(ctx, rest, defaults)
	case "status":
		return a.runAuthStatus(ctx, rest, defaults)
	case "logout":
		return a.runAuthLogout(rest, defaults)
	default:
		fmt.Fprintf(a.stderr, "unknown auth subcommand %q\n", sub)
		a.printAuthUsage(a.stderr)
		return exitUsage
	}
}

func (a *App) runAuthLogin(ctx context.Context, args []string, defaults commonFlags) int {
	var common commonFlags
	var noBrowser bool
	var consoleURL string
	var keyName string
	var timeout time.Duration

	fs := newFlagSet("world auth login", a.stderr)
	registerCommonFlags(fs, &common, defaults)
	fs.BoolVar(&noBrowser, "no-browser", false, "print authorization URL instead of opening browser")
	fs.StringVar(&consoleURL, "console-url", "", "console base URL for browser login")
	fs.StringVar(&keyName, "key-name", "", "suggested API key name for browser login")
	fs.DurationVar(&timeout, "timeout", 5*time.Minute, "wait timeout for browser authorization")

	vflags := commonValueFlags()
	vflags["console-url"] = true
	vflags["key-name"] = true
	vflags["timeout"] = true

	if err := fs.Parse(reorderArgs(args, vflags)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(a.stderr, "unexpected argument %q\n", fs.Arg(0))
		return exitUsage
	}

	resolved, err := a.store.Resolve(common.profile, common.endpoint, "")
	if err != nil {
		return a.fail(err)
	}
	if consoleURL != "" {
		resolved.ConsoleURL = strings.TrimRight(strings.TrimSpace(consoleURL), "/")
	}

	if common.apiKey != "" {
		return a.saveLoginResult(resolved.ProfileName, config.Profile{
			Endpoint:   resolved.Endpoint,
			ConsoleURL: resolved.ConsoleURL,
			APIKey:     strings.TrimSpace(common.apiKey),
		}, common.jsonOut)
	}

	if keyName == "" {
		keyName = suggestedKeyName()
	}
	res, err := a.loginWithCallback(ctx, resolved.ConsoleURL, keyName, noBrowser, timeout)
	if err != nil {
		return a.fail(err)
	}
	return a.saveLoginResult(resolved.ProfileName, config.Profile{
		Endpoint:   resolved.Endpoint,
		ConsoleURL: resolved.ConsoleURL,
		APIKey:     res.Secret,
		Email:      res.Email,
		KeyName:    res.KeyName,
		KeyPrefix:  res.KeyPrefix,
	}, common.jsonOut)
}

type callbackPayload struct {
	Secret    string `json:"secret"`
	State     string `json:"state"`
	Email     string `json:"email"`
	KeyName   string `json:"keyName"`
	KeyPrefix string `json:"keyPrefix"`
}

func (a *App) loginWithCallback(ctx context.Context, consoleBase, keyName string, noBrowser bool, timeout time.Duration) (callbackPayload, error) {
	consoleParsed, err := url.Parse(consoleBase)
	if err != nil || consoleParsed.Scheme == "" || consoleParsed.Host == "" {
		return callbackPayload{}, fmt.Errorf("invalid console URL %q", consoleBase)
	}
	allowedOrigin := consoleParsed.Scheme + "://" + consoleParsed.Host

	stateBytes := make([]byte, 24)
	if _, err := rand.Read(stateBytes); err != nil {
		return callbackPayload{}, err
	}
	expectedState := hex.EncodeToString(stateBytes)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return callbackPayload{}, fmt.Errorf("start local callback server: %w", err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port
	callbackURL := fmt.Sprintf("http://127.0.0.1:%d/callback", port)
	authURL := fmt.Sprintf("%s/cli-auth?callback=%s&state=%s&name=%s",
		strings.TrimRight(consoleBase, "/"),
		url.QueryEscape(callbackURL),
		url.QueryEscape(expectedState),
		url.QueryEscape(keyName),
	)

	resultCh := make(chan callbackPayload, 1)
	var once sync.Once

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			if origin != allowedOrigin {
				http.Error(w, "origin not allowed", http.StatusForbidden)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Private-Network", "true")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		raw, err := io.ReadAll(io.LimitReader(r.Body, 16*1024))
		if err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		var payload callbackPayload
		if err := json.Unmarshal(raw, &payload); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if subtle.ConstantTimeCompare([]byte(payload.State), []byte(expectedState)) != 1 {
			http.Error(w, "state mismatch", http.StatusForbidden)
			return
		}
		if strings.TrimSpace(payload.Secret) == "" {
			http.Error(w, "missing secret", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
		once.Do(func() {
			resultCh <- payload
		})
	})

	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		_ = srv.Serve(ln)
	}()
	defer func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	if a.onAuthReady != nil {
		a.onAuthReady(authURL, callbackURL, expectedState)
	}

	if !noBrowser && a.openBrowser != nil {
		if err := a.openBrowser(authURL); err == nil {
			fmt.Fprintf(a.stderr, "Opened browser for authorization. If it did not open, visit:\n%s\n", authURL)
		} else {
			fmt.Fprintf(a.stderr, "Open this URL in your browser to authorize World CLI:\n%s\n", authURL)
		}
	} else {
		fmt.Fprintf(a.stderr, "Open this URL in your browser to authorize World CLI:\n%s\n", authURL)
	}
	fmt.Fprintln(a.stderr, "Waiting for authorization... (or press Ctrl+C and run `world auth login --api-key <key>`)")

	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	select {
	case res := <-resultCh:
		return res, nil
	case <-waitCtx.Done():
		if errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
			return callbackPayload{}, errors.New("authorization timed out; retry or use `world auth login --api-key <key>`")
		}
		return callbackPayload{}, waitCtx.Err()
	}
}

func (a *App) saveLoginResult(profileName string, incoming config.Profile, jsonOut bool) int {
	f, err := a.store.Load()
	if err != nil {
		return a.fail(err)
	}
	existing := f.Profiles[profileName]
	if incoming.Endpoint != "" {
		existing.Endpoint = incoming.Endpoint
	}
	if incoming.ConsoleURL != "" {
		existing.ConsoleURL = incoming.ConsoleURL
	}
	existing.APIKey = incoming.APIKey
	if incoming.Email != "" {
		existing.Email = incoming.Email
	}
	if incoming.KeyName != "" {
		existing.KeyName = incoming.KeyName
	}
	if incoming.KeyPrefix != "" {
		existing.KeyPrefix = incoming.KeyPrefix
	}
	f.Profiles[profileName] = existing
	f.ActiveProfile = profileName
	if err := a.store.Save(f); err != nil {
		return a.fail(err)
	}

	out := map[string]any{
		"authenticated": true,
		"profile":       profileName,
		"endpoint":      existing.Endpoint,
		"apiKey":        config.MaskKey(existing.APIKey, existing.KeyPrefix),
		"configPath":    a.store.Path(),
	}
	if existing.Email != "" {
		out["email"] = existing.Email
	}
	if existing.KeyName != "" {
		out["keyName"] = existing.KeyName
	}

	if jsonOut {
		_ = writeJSON(a.stdout, out)
		return exitOK
	}
	fmt.Fprintf(a.stdout, "Authenticated profile %q (%s)\n", profileName, existing.Endpoint)
	if existing.Email != "" {
		fmt.Fprintf(a.stdout, "Account: %s\n", existing.Email)
	}
	fmt.Fprintf(a.stdout, "API Key: %s\n", config.MaskKey(existing.APIKey, existing.KeyPrefix))
	fmt.Fprintf(a.stdout, "Saved:   %s\n", a.store.Path())
	return exitOK
}

func (a *App) runAuthStatus(ctx context.Context, args []string, defaults commonFlags) int {
	var common commonFlags
	var offline bool

	fs := newFlagSet("world auth status", a.stderr)
	registerCommonFlags(fs, &common, defaults)
	fs.BoolVar(&offline, "offline", false, "only inspect local configuration without network checks")

	if err := fs.Parse(reorderArgs(args, commonValueFlags())); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}

	res, err := a.store.Resolve(common.profile, common.endpoint, common.apiKey)
	if err != nil {
		return a.fail(err)
	}

	status := struct {
		Profile       string `json:"profile"`
		Endpoint      string `json:"endpoint"`
		ConsoleURL    string `json:"consoleUrl"`
		Configured    bool   `json:"configured"`
		APIKey        string `json:"apiKey,omitempty"`
		Email         string `json:"email,omitempty"`
		KeyName       string `json:"keyName,omitempty"`
		Timezone      string `json:"timezone"`
		ConfigPath    string `json:"configPath"`
		Healthy       *bool  `json:"healthy,omitempty"`
		Authenticated *bool  `json:"authenticated,omitempty"`
		Error         string `json:"error,omitempty"`
	}{
		Profile:    res.ProfileName,
		Endpoint:   res.Endpoint,
		ConsoleURL: res.ConsoleURL,
		Configured: res.APIKey != "",
		APIKey:     config.MaskKey(res.APIKey, res.KeyPrefix),
		Email:      res.Email,
		KeyName:    res.KeyName,
		Timezone:   firstNonEmpty(res.Timezone, DetectTimezone()),
		ConfigPath: res.ConfigPath,
	}

	exitCode := exitOK
	if !offline {
		c := a.newClient(res.Endpoint, res.APIKey)
		hErr := c.Health(ctx)
		healthy := hErr == nil
		status.Healthy = &healthy
		if hErr != nil {
			status.Error = hErr.Error()
			exitCode = exitError
		} else if res.APIKey == "" {
			authed := false
			status.Authenticated = &authed
			status.Error = "not logged in; run `world auth login`"
			exitCode = exitUnauth
		} else {
			aErr := c.VerifyAuth(ctx)
			authed := aErr == nil
			status.Authenticated = &authed
			if aErr != nil {
				status.Error = aErr.Error()
				exitCode = classifyError(aErr)
			}
		}
	} else if res.APIKey == "" {
		exitCode = exitUnauth
	}

	if common.jsonOut || res.Output == "json" {
		_ = writeJSON(a.stdout, status)
		return exitCode
	}

	fmt.Fprintf(a.stdout, "Profile:    %s\n", status.Profile)
	fmt.Fprintf(a.stdout, "Endpoint:   %s\n", status.Endpoint)
	if status.Email != "" {
		fmt.Fprintf(a.stdout, "Account:    %s\n", status.Email)
	}
	if status.Configured {
		fmt.Fprintf(a.stdout, "API Key:    %s\n", status.APIKey)
	} else {
		fmt.Fprintln(a.stdout, "API Key:    (not configured)")
	}
	fmt.Fprintf(a.stdout, "Timezone:   %s\n", status.Timezone)
	if status.Healthy != nil {
		fmt.Fprintf(a.stdout, "Reachable:  %t\n", *status.Healthy)
	}
	if status.Authenticated != nil {
		fmt.Fprintf(a.stdout, "Valid Key:  %t\n", *status.Authenticated)
	}
	if status.Error != "" {
		fmt.Fprintf(a.stdout, "Status Err: %s\n", status.Error)
	}
	return exitCode
}

func (a *App) runAuthLogout(args []string, defaults commonFlags) int {
	var common commonFlags
	fs := newFlagSet("world auth logout", a.stderr)
	registerCommonFlags(fs, &common, defaults)
	if err := fs.Parse(reorderArgs(args, commonValueFlags())); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}

	f, err := a.store.Load()
	if err != nil {
		return a.fail(err)
	}
	target := common.profile
	if target == "" {
		target = f.ActiveProfile
	}
	p := f.Profiles[target]
	p.APIKey = ""
	p.Email = ""
	p.KeyName = ""
	p.KeyPrefix = ""
	f.Profiles[target] = p
	if err := a.store.Save(f); err != nil {
		return a.fail(err)
	}

	if common.jsonOut {
		_ = writeJSON(a.stdout, map[string]any{"loggedOut": true, "profile": target})
		return exitOK
	}
	fmt.Fprintf(a.stdout, "Cleared credentials for profile %q.\n", target)
	return exitOK
}

func suggestedKeyName() string {
	host, err := os.Hostname()
	if err != nil || strings.TrimSpace(host) == "" {
		return "world-cli"
	}
	host = strings.TrimSpace(host)
	if idx := strings.IndexByte(host, '.'); idx > 0 {
		host = host[:idx]
	}
	name := "world-cli-" + host
	if len(name) > 64 {
		name = name[:64]
	}
	return name
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
