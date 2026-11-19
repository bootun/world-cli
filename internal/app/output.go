package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/bootun/world-cli/internal/client"
)

func writeJSON(w io.Writer, v any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return err
	}
	_, err := w.Write(buf.Bytes())
	return err
}

func formatMs(ms int64, tz string) string {
	if ms == 0 {
		return "-"
	}
	loc := time.UTC
	if tz != "" {
		if loaded, err := time.LoadLocation(tz); err == nil {
			loc = loaded
		}
	}
	return time.UnixMilli(ms).In(loc).Format("2006-01-02 15:04:05 MST")
}

func printEntity(w io.Writer, e client.EntityView, tz string) {
	fmt.Fprintf(w, "Entity: %s (%s)\n", e.CanonicalName, e.EntityID)
	if e.EntityType != "" {
		fmt.Fprintf(w, "Type:   %s\n", e.EntityType)
	}
	if len(e.Aliases) > 0 {
		fmt.Fprintf(w, "Aliases: %s\n", strings.Join(e.Aliases, ", "))
	}
	if len(e.Components) > 0 {
		keys := make([]string, 0, len(e.Components))
		for k := range e.Components {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		fmt.Fprintln(w, "Components:")
		for _, k := range keys {
			c := e.Components[k]
			payloadRaw, _ := json.Marshal(c.Payload)
			fmt.Fprintf(w, "  - %s", c.ComponentType)
			if c.EpistemicStatus != "" {
				fmt.Fprintf(w, " [%s]", c.EpistemicStatus)
			}
			if c.Confidence != nil {
				fmt.Fprintf(w, " (conf=%.2f)", *c.Confidence)
			}
			fmt.Fprintf(w, ": %s", string(payloadRaw))
			if c.ValidFromMs != 0 || c.ValidToMs != 0 {
				fmt.Fprintf(w, " (valid %s .. %s)", formatMs(int64(c.ValidFromMs), tz), formatMs(int64(c.ValidToMs), tz))
			}
			fmt.Fprintln(w)
		}
	}
}

func printRecall(w io.Writer, res client.RecallResponse, tz string) {
	if len(res.Entities) == 0 && len(res.Memories) == 0 {
		fmt.Fprintln(w, "No matching entities or memories.")
		return
	}
	if len(res.Entities) > 0 {
		fmt.Fprintf(w, "Entities (%d):\n", len(res.Entities))
		for i, e := range res.Entities {
			if i > 0 {
				fmt.Fprintln(w)
			}
			printEntity(w, e, tz)
		}
	}
	if len(res.Memories) > 0 {
		if len(res.Entities) > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintf(w, "Memories (%d):\n", len(res.Memories))
		for _, m := range res.Memories {
			fmt.Fprintf(w, "  - [%s] %s", formatMs(int64(m.OccurredAtMs), tz), m.Summary)
			if len(m.AffectedEntityIds) > 0 {
				fmt.Fprintf(w, " [entities: %s]", strings.Join(m.AffectedEntityIds, ", "))
			}
			fmt.Fprintln(w)
		}
	}
}
