package version

// Version is the semantic version of world-cli.
// It can be overridden at build time via:
//
//	-ldflags "-X github.com/bootun/world-cli/internal/version.Version=x.y.z"
var Version = "0.1.0"
