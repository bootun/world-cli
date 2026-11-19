package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
)

func (a *App) runEntity(ctx context.Context, args []string, defaults commonFlags) int {
	if len(args) == 0 || isHelp(args[0]) {
		a.printEntityUsage(a.stdout)
		if len(args) == 0 {
			return exitUsage
		}
		return exitOK
	}
	if args[0] != "get" {
		fmt.Fprintf(a.stderr, "unknown entity subcommand %q\n", args[0])
		a.printEntityUsage(a.stderr)
		return exitUsage
	}

	var common commonFlags
	var asOf string
	var tzFlag string

	fs := newFlagSet("world entity get", a.stderr)
	registerCommonFlags(fs, &common, defaults)
	fs.StringVar(&asOf, "as-of", "", "query entity valid at this time (RFC3339, YYYY-MM-DD, or unix ms)")
	fs.StringVar(&tzFlag, "timezone", "", "IANA timezone used for formatting/parsing dates")

	vflags := commonValueFlags()
	vflags["as-of"] = true
	vflags["timezone"] = true

	if err := fs.Parse(reorderArgs(args[1:], vflags)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(a.stderr, "usage: world entity get <entity_id> [flags]")
		return exitUsage
	}
	entityID := fs.Arg(0)

	res, err := a.store.Resolve(common.profile, common.endpoint, common.apiKey)
	if err != nil {
		return a.fail(err)
	}
	if res.APIKey == "" {
		return a.unauthFail()
	}

	tz, err := ResolveTimezone(tzFlag, res.Timezone)
	if err != nil {
		return a.usageFail(err)
	}
	asOfMs, err := ParseTimeMs(asOf, tz)
	if err != nil {
		return a.usageFail(err)
	}

	c := a.newClient(res.Endpoint, res.APIKey)
	out, err := c.GetEntity(ctx, entityID, asOfMs)
	if err != nil {
		return a.fail(err)
	}

	if common.jsonOut || res.Output == "json" {
		_ = writeJSON(a.stdout, out)
		return exitOK
	}
	if out.Entity == nil {
		fmt.Fprintln(a.stdout, "Entity not found.")
		return exitNotFound
	}
	printEntity(a.stdout, *out.Entity, tz)
	return exitOK
}
