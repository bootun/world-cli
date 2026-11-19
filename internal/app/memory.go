package app

import (
	"context"
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/bootun/world-cli/internal/client"
)

type kvList []string

func (k *kvList) String() string { return strings.Join(*k, ",") }
func (k *kvList) Set(v string) error {
	*k = append(*k, v)
	return nil
}

func (a *App) runRemember(ctx context.Context, args []string, defaults commonFlags) int {
	var common commonFlags
	var source string
	var tzFlag string
	var occurredAt string
	var idempotencyKey string
	var metaFlags kvList

	fs := newFlagSet("world remember", a.stderr)
	registerCommonFlags(fs, &common, defaults)
	fs.StringVar(&source, "source", "", "origin label (e.g. cli, agent, notes)")
	fs.StringVar(&tzFlag, "timezone", "", "IANA timezone (defaults to config or local timezone)")
	fs.StringVar(&occurredAt, "occurred-at", "", "occurrence time (RFC3339, YYYY-MM-DD[ HH:MM[:SS]], or unix ms)")
	fs.StringVar(&idempotencyKey, "idempotency-key", "", "explicit Idempotency-Key header")
	fs.Var(&metaFlags, "meta", "extra metadata as key=value (repeatable)")

	vflags := commonValueFlags()
	vflags["source"] = true
	vflags["timezone"] = true
	vflags["occurred-at"] = true
	vflags["idempotency-key"] = true
	vflags["meta"] = true

	if err := fs.Parse(reorderArgs(args, vflags)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}

	text, err := a.readTextArgOrStdin(fs.Args())
	if err != nil {
		return a.usageFail(err)
	}

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

	var occurredPtr *client.Int64
	if strings.TrimSpace(occurredAt) != "" {
		ms, err := ParseTimeMs(occurredAt, tz)
		if err != nil {
			return a.usageFail(err)
		}
		v := client.Int64(ms)
		occurredPtr = &v
	}

	metadata, err := parseMetadata(metaFlags)
	if err != nil {
		return a.usageFail(err)
	}

	if source == "" {
		source = firstNonEmpty(res.Source, "cli")
	}
	if idempotencyKey == "" {
		idempotencyKey = newUUIDv4()
	}

	c := a.newClient(res.Endpoint, res.APIKey)
	out, err := c.WriteMemory(ctx, client.WriteMemoryRequest{
		Text:           text,
		OccurredAtMs:   occurredPtr,
		Source:         source,
		Metadata:       metadata,
		Timezone:       tz,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		return a.fail(err)
	}

	if common.jsonOut || res.Output == "json" {
		_ = writeJSON(a.stdout, out)
		return exitOK
	}
	fmt.Fprint(a.stdout, "Recorded memory")
	if out.Deduplicated {
		fmt.Fprint(a.stdout, " (deduplicated)")
	}
	fmt.Fprintln(a.stdout)
	return exitOK
}

func (a *App) runRecall(ctx context.Context, args []string, defaults commonFlags) int {
	var common commonFlags
	var asOf string
	var tzFlag string
	var maxEntities uint
	var maxEvents uint

	fs := newFlagSet("world recall", a.stderr)
	registerCommonFlags(fs, &common, defaults)
	fs.StringVar(&asOf, "as-of", "", "query world state valid at this time (RFC3339, YYYY-MM-DD, or unix ms)")
	fs.StringVar(&tzFlag, "timezone", "", "IANA timezone used for formatting/parsing dates")
	fs.UintVar(&maxEntities, "max-entities", 0, "maximum number of entities to return")
	fs.UintVar(&maxEvents, "max-events", 0, "maximum number of raw memories to return")

	vflags := commonValueFlags()
	vflags["as-of"] = true
	vflags["timezone"] = true
	vflags["max-entities"] = true
	vflags["max-events"] = true

	if err := fs.Parse(reorderArgs(args, vflags)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	query := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if query == "" {
		return a.usageFail(errors.New("usage: world recall \"<query>\" [flags]"))
	}

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
	out, err := c.Recall(ctx, client.RecallRequest{
		Query:           query,
		MaxEntities:     uint32(maxEntities),
		MaxEvents:       uint32(maxEvents),
		AsOfValidTimeMs: asOfMs,
	})
	if err != nil {
		return a.fail(err)
	}

	if common.jsonOut || res.Output == "json" {
		_ = writeJSON(a.stdout, out)
		return exitOK
	}
	printRecall(a.stdout, out, tz)
	return exitOK
}

func (a *App) readTextArgOrStdin(pos []string) (string, error) {
	if len(pos) > 0 {
		joined := strings.Join(pos, " ")
		if joined != "-" {
			trimmed := strings.TrimSpace(joined)
			if trimmed == "" {
				return "", errors.New("memory text cannot be empty")
			}
			return trimmed, nil
		}
	}
	if len(pos) == 0 && a.stdinIsTerminal() {
		return "", errors.New("usage: world remember \"<text>\" [flags] (or pipe text via stdin)")
	}
	raw, err := io.ReadAll(io.LimitReader(a.stdin, 1<<20))
	if err != nil {
		return "", err
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return "", errors.New("memory text cannot be empty")
	}
	return trimmed, nil
}

func (a *App) stdinIsTerminal() bool {
	f, ok := a.stdin.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func parseMetadata(items []string) (map[string]any, error) {
	if len(items) == 0 {
		return nil, nil
	}
	out := make(map[string]any, len(items))
	for _, item := range items {
		k, v, ok := strings.Cut(item, "=")
		k = strings.TrimSpace(k)
		if !ok || k == "" {
			return nil, fmt.Errorf("invalid --meta %q (expected key=value)", item)
		}
		out[k] = v
	}
	return out, nil
}

func newUUIDv4() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
