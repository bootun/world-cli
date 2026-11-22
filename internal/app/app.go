package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/bootun/world-cli/internal/client"
	"github.com/bootun/world-cli/internal/config"
	"github.com/bootun/world-cli/internal/version"
)

const (
	exitOK       = 0
	exitError    = 1
	exitUsage    = 2
	exitUnauth   = 3
	exitNotFound = 4
)

type App struct {
	store       *config.Store
	stdin       io.Reader
	stdout      io.Writer
	stderr      io.Writer
	newClient   func(endpoint, apiKey string) *client.Client
	openBrowser browserOpener
	onAuthReady func(authURL, callbackURL, state string)
	homeDir     func() (string, error)
	workDir     func() (string, error)
	execPath    func() (string, error)
	lookPath    func(string) (string, error)
}

func New(store *config.Store, stdin io.Reader, stdout, stderr io.Writer) *App {
	return &App{
		store:  store,
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
		newClient: func(endpoint, apiKey string) *client.Client {
			return client.New(endpoint, apiKey, client.WithCLIVersion(version.Version))
		},
		openBrowser: defaultBrowserOpener,
		homeDir:     os.UserHomeDir,
		workDir:     os.Getwd,
		execPath:    os.Executable,
		lookPath:    exec.LookPath,
	}
}

func Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	path, err := config.DefaultPath()
	if err != nil {
		fmt.Fprintf(stderr, "error resolving config path: %v\n", err)
		return exitError
	}
	return New(config.NewStore(path), stdin, stdout, stderr).Run(ctx, args)
}

func (a *App) Run(ctx context.Context, args []string) int {
	defaults, rest, helpRequested, err := parseLeadingGlobalFlags(args)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return exitUsage
	}
	if helpRequested || len(rest) == 0 {
		a.printRootUsage(a.stdout)
		if len(rest) == 0 && !helpRequested {
			return exitUsage
		}
		return exitOK
	}

	cmd := rest[0]
	subArgs := rest[1:]

	switch cmd {
	case "version", "--version", "-v":
		jsonOut := defaults.jsonOut
		for _, arg := range subArgs {
			if arg == "--json" {
				jsonOut = true
			}
		}
		if jsonOut {
			_ = writeJSON(a.stdout, map[string]string{"version": version.Version})
		} else {
			fmt.Fprintf(a.stdout, "world %s\n", version.Version)
		}
		return exitOK
	case "help":
		a.printRootUsage(a.stdout)
		return exitOK
	case "auth":
		return a.runAuth(ctx, subArgs, defaults)
	case "config":
		return a.runConfig(subArgs, defaults)
	case "remember":
		return a.runRemember(ctx, subArgs, defaults)
	case "recall":
		return a.runRecall(ctx, subArgs, defaults)
	case "forget", "retract":
		return a.runForget(ctx, subArgs, defaults)
	case "fix", "supersede":
		return a.runFix(ctx, subArgs, defaults)
	case "entity":
		return a.runEntity(ctx, subArgs, defaults)
	case "hook":
		return a.runHook(subArgs, defaults)
	default:
		fmt.Fprintf(a.stderr, "unknown command %q\n\n", cmd)
		a.printRootUsage(a.stderr)
		return exitUsage
	}
}

func parseLeadingGlobalFlags(args []string) (commonFlags, []string, bool, error) {
	var opts commonFlags
	i := 0
	for i < len(args) {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") || arg == "-" || arg == "--" {
			break
		}
		if arg == "-h" || arg == "--help" {
			return opts, nil, true, nil
		}
		if arg == "--version" || arg == "-v" {
			return opts, args[i:], false, nil
		}
		if arg == "--json" {
			opts.jsonOut = true
			i++
			continue
		}
		name, val, hasEq := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		switch name {
		case "profile", "endpoint", "api-key":
			if !hasEq {
				if i+1 >= len(args) {
					return opts, nil, false, fmt.Errorf("flag --%s requires a value", name)
				}
				val = args[i+1]
				i += 2
			} else {
				i++
			}
			switch name {
			case "profile":
				opts.profile = val
			case "endpoint":
				opts.endpoint = val
			case "api-key":
				opts.apiKey = val
			}
		default:
			return opts, nil, false, fmt.Errorf("unknown global flag %q", arg)
		}
	}
	return opts, args[i:], false, nil
}

func (a *App) fail(err error) int {
	fmt.Fprintf(a.stderr, "error: %v\n", err)
	return classifyError(err)
}

func (a *App) usageFail(err error) int {
	fmt.Fprintf(a.stderr, "error: %v\n", err)
	return exitUsage
}

func (a *App) unauthFail() int {
	fmt.Fprintln(a.stderr, "error: not authenticated; run `world auth login` or set WORLD_API_KEY")
	return exitUnauth
}

func classifyError(err error) int {
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		switch {
		case apiErr.HTTPStatus == http.StatusUnauthorized || apiErr.Code == "unauthenticated":
			return exitUnauth
		case apiErr.HTTPStatus == http.StatusNotFound || apiErr.Code == "not_found":
			return exitNotFound
		case apiErr.HTTPStatus == http.StatusBadRequest || apiErr.Code == "invalid_argument":
			return exitUsage
		}
	}
	return exitError
}

func isHelp(arg string) bool {
	return arg == "-h" || arg == "--help" || arg == "help"
}

func (a *App) printRootUsage(w io.Writer) {
	fmt.Fprint(w, `world - World Machine CLI

Usage:
  world <command> [flags]

Commands:
  remember "<text>"       Record a memory into World Machine
  recall "<query>"        Recall entities and memories
  forget <event_id>       Retract a mistaken memory and roll back its derived state
  fix <event_id> "<text>" Replace a mistaken memory with corrected text
  entity get <id>         Fetch one entity and its components
  auth <subcommand>       Manage login credentials (login, status, logout)
  config <subcommand>     Manage local configuration (list, get, set)
  hook <subcommand>       Install and manage AI harness lifecycle hooks
  version                 Show CLI version

Global Flags:
  --profile <name>        Use a named configuration profile
  --endpoint <url>        Override API endpoint
  --api-key <key>         Override API key
  --json                  Output machine-readable JSON

Examples:
  world auth login
  world auth login --api-key "$WORLD_API_KEY"
  world remember "Alice moved to Tokyo yesterday." --source notes
  world recall "Where does Alice live?" --json
  world recall "Where did Alice live?" --as-of "2026-01-01" --json
  world forget <event_id> --reason "test data"
  world fix <event_id> "My workplace is Shanghai." --json
`)
}

func (a *App) printAuthUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  world auth <subcommand> [flags]

Subcommands:
  login     Sign in via browser authorization or --api-key
  status    Verify local configuration, connectivity, and key status
  logout    Clear saved credentials for the active profile
`)
}

func (a *App) printConfigUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  world config <subcommand> [flags]

Subcommands:
  list                Show resolved configuration
  get <key>           Get one configuration value
  set <key> <value>   Set profile, endpoint, console-url, timezone, source, or output
`)
}

func (a *App) printEntityUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  world entity get <entity_id> [--as-of <time>] [--json]
`)
}
