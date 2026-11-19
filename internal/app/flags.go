package app

import (
	"flag"
	"io"
	"strings"
)

// reorderArgs moves known flags before positional arguments so callers can place
// flags anywhere on the command line (e.g. `world remember "hi" --json`).
func reorderArgs(args []string, valueFlags map[string]bool) []string {
	var flags []string
	var pos []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			pos = append(pos, args[i:]...)
			break
		}
		if strings.HasPrefix(arg, "-") && len(arg) > 1 {
			flags = append(flags, arg)
			if !strings.Contains(arg, "=") {
				name := strings.TrimLeft(arg, "-")
				if valueFlags[name] && i+1 < len(args) {
					i++
					flags = append(flags, args[i])
				}
			}
			continue
		}
		pos = append(pos, arg)
	}
	return append(flags, pos...)
}

type commonFlags struct {
	profile  string
	endpoint string
	apiKey   string
	jsonOut  bool
}

func registerCommonFlags(fs *flag.FlagSet, dst *commonFlags, defaults commonFlags) {
	*dst = defaults
	fs.StringVar(&dst.profile, "profile", defaults.profile, "config profile name")
	fs.StringVar(&dst.endpoint, "endpoint", defaults.endpoint, "API base URL")
	fs.StringVar(&dst.apiKey, "api-key", defaults.apiKey, "API key override")
	fs.BoolVar(&dst.jsonOut, "json", defaults.jsonOut, "output JSON")
}

func commonValueFlags() map[string]bool {
	return map[string]bool{
		"profile":  true,
		"endpoint": true,
		"api-key":  true,
	}
}

func newFlagSet(name string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	return fs
}
