package app

import (
	"fmt"
	"io"
	"strings"

	"github.com/bootun/world-cli/internal/hooks"
)

func (a *App) runHook(args []string, defaults commonFlags) int {
	if len(args) == 0 || isHelp(args[0]) {
		a.printHookUsage(a.stdout)
		if len(args) == 0 {
			return exitUsage
		}
		return exitOK
	}

	switch args[0] {
	case "install":
		return a.runHookInstall(args[1:], defaults)
	case "uninstall":
		return a.runHookUninstall(args[1:], defaults)
	case "status":
		return a.runHookStatus(args[1:], defaults)
	case "session-start":
		return a.runHookEvent("session-start", args[1:], defaults)
	case "stop":
		return a.runHookEvent("stop", args[1:], defaults)
	default:
		fmt.Fprintf(a.stderr, "unknown hook subcommand %q\n\n", args[0])
		a.printHookUsage(a.stderr)
		return exitUsage
	}
}

func (a *App) resolveHookDirs() (string, string, error) {
	home, err := a.homeDir()
	if err != nil {
		return "", "", fmt.Errorf("resolve home directory: %w", err)
	}
	work, err := a.workDir()
	if err != nil {
		return "", "", fmt.Errorf("resolve working directory: %w", err)
	}
	return home, work, nil
}

func (a *App) runHookInstall(args []string, defaults commonFlags) int {
	fs := newFlagSet("hook install", a.stderr)
	var common commonFlags
	registerCommonFlags(fs, &common, defaults)
	var target, scope, commandBin string
	fs.StringVar(&target, "target", hooks.TargetAuto, "target harness: auto, all, antigravity, claude, codex (comma-separated)")
	fs.StringVar(&scope, "scope", hooks.ScopeUser, "install scope: user or project")
	fs.StringVar(&commandBin, "command", hooks.DefaultCommandBin, "world binary command to register")

	valueFlags := commonValueFlags()
	valueFlags["target"] = true
	valueFlags["scope"] = true
	valueFlags["command"] = true
	if err := fs.Parse(reorderArgs(args, valueFlags)); err != nil {
		return exitUsage
	}
	if fs.NArg() > 0 {
		return a.usageFail(fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " ")))
	}

	home, work, err := a.resolveHookDirs()
	if err != nil {
		return a.fail(err)
	}
	results, err := hooks.Install(hooks.ManageOptions{
		Target:     target,
		Scope:      scope,
		CommandBin: commandBin,
		HomeDir:    home,
		WorkDir:    work,
	})
	if err != nil {
		return a.fail(err)
	}

	if common.jsonOut {
		if results == nil {
			results = []hooks.TargetStatus{}
		}
		if err := writeJSON(a.stdout, map[string]any{"installed": results}); err != nil {
			return a.fail(err)
		}
		return exitOK
	}

	if len(results) == 0 {
		fmt.Fprintln(a.stdout, "No supported AI harness directories detected automatically.")
		fmt.Fprintln(a.stdout, "Specify one explicitly, e.g.: world hook install --target claude|codex|antigravity|all")
		return exitOK
	}

	hasCodex := false
	for _, r := range results {
		state := "installed"
		if !r.Changed {
			state = "already up to date"
		}
		fmt.Fprintf(a.stdout, "✓ %-11s (%s): %s -> %s\n", r.Target, r.Scope, state, r.ConfigPath)
		if r.Target == hooks.TargetCodex {
			hasCodex = true
		}
	}
	if hasCodex {
		fmt.Fprintln(a.stdout, "Note: Codex may ask you to trust newly added hooks via /hooks.")
	}
	return exitOK
}

func (a *App) runHookUninstall(args []string, defaults commonFlags) int {
	fs := newFlagSet("hook uninstall", a.stderr)
	var common commonFlags
	registerCommonFlags(fs, &common, defaults)
	var target, scope string
	fs.StringVar(&target, "target", hooks.TargetAuto, "target harness: auto, all, antigravity, claude, codex (comma-separated)")
	fs.StringVar(&scope, "scope", hooks.ScopeUser, "install scope: user or project")

	valueFlags := commonValueFlags()
	valueFlags["target"] = true
	valueFlags["scope"] = true
	if err := fs.Parse(reorderArgs(args, valueFlags)); err != nil {
		return exitUsage
	}
	if fs.NArg() > 0 {
		return a.usageFail(fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " ")))
	}

	home, work, err := a.resolveHookDirs()
	if err != nil {
		return a.fail(err)
	}
	results, err := hooks.Uninstall(hooks.ManageOptions{
		Target:  target,
		Scope:   scope,
		HomeDir: home,
		WorkDir: work,
	})
	if err != nil {
		return a.fail(err)
	}

	if common.jsonOut {
		if results == nil {
			results = []hooks.TargetStatus{}
		}
		if err := writeJSON(a.stdout, map[string]any{"uninstalled": results}); err != nil {
			return a.fail(err)
		}
		return exitOK
	}

	if len(results) == 0 {
		fmt.Fprintln(a.stdout, "No installed World Machine hooks found.")
		return exitOK
	}
	for _, r := range results {
		state := "removed"
		if !r.Changed {
			state = "not installed"
		}
		fmt.Fprintf(a.stdout, "✓ %-11s (%s): %s -> %s\n", r.Target, r.Scope, state, r.ConfigPath)
	}
	return exitOK
}

func (a *App) runHookStatus(args []string, defaults commonFlags) int {
	fs := newFlagSet("hook status", a.stderr)
	var common commonFlags
	registerCommonFlags(fs, &common, defaults)
	var target, scope string
	fs.StringVar(&target, "target", hooks.TargetAuto, "target harness: auto, all, antigravity, claude, codex (comma-separated)")
	fs.StringVar(&scope, "scope", hooks.ScopeUser, "install scope: user or project")

	valueFlags := commonValueFlags()
	valueFlags["target"] = true
	valueFlags["scope"] = true
	if err := fs.Parse(reorderArgs(args, valueFlags)); err != nil {
		return exitUsage
	}
	if fs.NArg() > 0 {
		return a.usageFail(fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " ")))
	}

	home, work, err := a.resolveHookDirs()
	if err != nil {
		return a.fail(err)
	}
	results, err := hooks.Status(hooks.ManageOptions{
		Target:  target,
		Scope:   scope,
		HomeDir: home,
		WorkDir: work,
	})
	if err != nil {
		return a.fail(err)
	}

	if common.jsonOut {
		if err := writeJSON(a.stdout, map[string]any{"targets": results}); err != nil {
			return a.fail(err)
		}
		return exitOK
	}

	for _, r := range results {
		status := "not installed"
		if r.SessionStartInstalled && r.StopInstalled {
			status = "installed (SessionStart, Stop)"
		} else if r.SessionStartInstalled || r.StopInstalled {
			status = fmt.Sprintf("partial (SessionStart=%t, Stop=%t)", r.SessionStartInstalled, r.StopInstalled)
		}
		det := ""
		if r.Detected {
			det = " [detected]"
		}
		fmt.Fprintf(a.stdout, "%-11s (%s)%s: %s\n  %s\n", r.Target, r.Scope, det, status, r.ConfigPath)
	}
	return exitOK
}

func (a *App) runHookEvent(event string, args []string, defaults commonFlags) int {
	fs := newFlagSet("hook "+event, a.stderr)
	var common commonFlags
	registerCommonFlags(fs, &common, defaults)
	var dialect string
	fs.StringVar(&dialect, "dialect", hooks.DialectAuto, "hook payload dialect: auto, antigravity, claude, codex")

	valueFlags := commonValueFlags()
	valueFlags["dialect"] = true
	if err := fs.Parse(reorderArgs(args, valueFlags)); err != nil {
		return exitUsage
	}

	raw, _ := io.ReadAll(a.stdin)

	authenticated := false
	if resolved, err := a.store.Resolve(common.profile, common.endpoint, common.apiKey); err == nil {
		authenticated = strings.TrimSpace(resolved.APIKey) != ""
	}

	var (
		out []byte
		err error
	)
	switch event {
	case "session-start":
		out, err = hooks.HandleSessionStart(raw, dialect, authenticated)
	case "stop":
		out, err = hooks.HandleStop(raw, dialect, authenticated)
	default:
		out = []byte("{}\n")
	}
	if err != nil {
		_, _ = a.stdout.Write([]byte("{}\n"))
		return exitOK
	}
	_, _ = a.stdout.Write(out)
	return exitOK
}

func (a *App) printHookUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  world hook <subcommand> [flags]

Subcommands:
  install        Install SessionStart and Stop hooks for AI harnesses (antigravity, claude, codex)
  uninstall      Remove World Machine hooks from AI harness configurations
  status         Show hook installation status across supported AI harnesses
  session-start  Handle SessionStart hook callback (invoked by AI harness via stdin/stdout)
  stop           Handle Stop hook callback (invoked by AI harness via stdin/stdout)

Examples:
  world hook install
  world hook install --target claude,codex
  world hook install --target antigravity --scope project
  world hook status
  world hook uninstall
`)
}
