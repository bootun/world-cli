package app

import (
	"errors"
	"flag"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/bootun/world-cli/internal/config"
)

func (a *App) runConfig(args []string, defaults commonFlags) int {
	if len(args) == 0 || isHelp(args[0]) {
		a.printConfigUsage(a.stdout)
		if len(args) == 0 {
			return exitUsage
		}
		return exitOK
	}
	sub := args[0]
	rest := args[1:]
	switch sub {
	case "list":
		return a.runConfigList(rest, defaults)
	case "get":
		return a.runConfigGet(rest, defaults)
	case "set":
		return a.runConfigSet(rest, defaults)
	default:
		fmt.Fprintf(a.stderr, "unknown config subcommand %q\n", sub)
		a.printConfigUsage(a.stderr)
		return exitUsage
	}
}

func (a *App) runConfigList(args []string, defaults commonFlags) int {
	var common commonFlags
	fs := newFlagSet("world config list", a.stderr)
	registerCommonFlags(fs, &common, defaults)
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
	payload := map[string]any{
		"profile":    res.ProfileName,
		"endpoint":   res.Endpoint,
		"consoleUrl": res.ConsoleURL,
		"apiKey":     config.MaskKey(res.APIKey, res.KeyPrefix),
		"timezone":   res.Timezone,
		"source":     res.Source,
		"output":     res.Output,
		"configPath": res.ConfigPath,
	}
	if common.jsonOut || res.Output == "json" {
		_ = writeJSON(a.stdout, payload)
		return exitOK
	}
	fmt.Fprintf(a.stdout, "profile     = %s\n", res.ProfileName)
	fmt.Fprintf(a.stdout, "endpoint    = %s\n", res.Endpoint)
	fmt.Fprintf(a.stdout, "console-url = %s\n", res.ConsoleURL)
	fmt.Fprintf(a.stdout, "api-key     = %s\n", firstNonEmpty(config.MaskKey(res.APIKey, res.KeyPrefix), "(unset)"))
	fmt.Fprintf(a.stdout, "timezone    = %s\n", firstNonEmpty(res.Timezone, "(auto: "+DetectTimezone()+")"))
	fmt.Fprintf(a.stdout, "source      = %s\n", firstNonEmpty(res.Source, "(unset)"))
	fmt.Fprintf(a.stdout, "output      = %s\n", res.Output)
	return exitOK
}

func (a *App) runConfigGet(args []string, defaults commonFlags) int {
	var common commonFlags
	fs := newFlagSet("world config get", a.stderr)
	registerCommonFlags(fs, &common, defaults)
	if err := fs.Parse(reorderArgs(args, commonValueFlags())); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(a.stderr, "usage: world config get <key>")
		return exitUsage
	}
	key := fs.Arg(0)
	res, err := a.store.Resolve(common.profile, common.endpoint, common.apiKey)
	if err != nil {
		return a.fail(err)
	}
	var val string
	switch key {
	case "profile":
		val = res.ProfileName
	case "endpoint":
		val = res.Endpoint
	case "console-url":
		val = res.ConsoleURL
	case "api-key":
		val = config.MaskKey(res.APIKey, res.KeyPrefix)
	case "timezone":
		val = firstNonEmpty(res.Timezone, DetectTimezone())
	case "source":
		val = res.Source
	case "output":
		val = res.Output
	default:
		return a.usageFail(fmt.Errorf("unknown config key %q", key))
	}
	if common.jsonOut {
		_ = writeJSON(a.stdout, map[string]string{"key": key, "value": val})
		return exitOK
	}
	fmt.Fprintln(a.stdout, val)
	return exitOK
}

func (a *App) runConfigSet(args []string, defaults commonFlags) int {
	var common commonFlags
	fs := newFlagSet("world config set", a.stderr)
	registerCommonFlags(fs, &common, defaults)
	if err := fs.Parse(reorderArgs(args, commonValueFlags())); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(a.stderr, "usage: world config set <key> <value>")
		return exitUsage
	}
	key, val := fs.Arg(0), strings.TrimSpace(fs.Arg(1))

	f, err := a.store.Load()
	if err != nil {
		return a.fail(err)
	}
	target := firstNonEmpty(common.profile, f.ActiveProfile, config.DefaultProfile)
	p := f.Profiles[target]

	switch key {
	case "profile":
		if val == "" {
			return a.usageFail(errors.New("profile name cannot be empty"))
		}
		f.ActiveProfile = val
		if _, ok := f.Profiles[val]; !ok {
			f.Profiles[val] = config.Profile{}
		}
	case "endpoint":
		if err := validateHTTPURL(val); err != nil {
			return a.usageFail(err)
		}
		p.Endpoint = strings.TrimRight(val, "/")
		f.Profiles[target] = p
	case "console-url":
		if err := validateHTTPURL(val); err != nil {
			return a.usageFail(err)
		}
		p.ConsoleURL = strings.TrimRight(val, "/")
		f.Profiles[target] = p
	case "timezone":
		if val != "" {
			if _, err := time.LoadLocation(val); err != nil {
				return a.usageFail(fmt.Errorf("invalid IANA timezone %q: %w", val, err))
			}
		}
		p.Timezone = val
		f.Profiles[target] = p
	case "source":
		p.Source = val
		f.Profiles[target] = p
	case "output":
		if val != "text" && val != "json" {
			return a.usageFail(fmt.Errorf("output must be 'text' or 'json', got %q", val))
		}
		p.Output = val
		f.Profiles[target] = p
	default:
		return a.usageFail(fmt.Errorf("unknown or unsupported config key %q (supported: profile, endpoint, console-url, timezone, source, output)", key))
	}

	if err := a.store.Save(f); err != nil {
		return a.fail(err)
	}
	if common.jsonOut {
		_ = writeJSON(a.stdout, map[string]string{"profile": target, "key": key, "value": val})
		return exitOK
	}
	fmt.Fprintf(a.stdout, "Updated %s = %s (profile %q)\n", key, val, target)
	return exitOK
}

func validateHTTPURL(raw string) error {
	u, err := url.ParseRequestURI(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("invalid HTTP(S) URL %q", raw)
	}
	return nil
}
