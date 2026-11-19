package app

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func DetectTimezone() string {
	if tz := strings.TrimSpace(os.Getenv("TZ")); tz != "" && tz != "Local" {
		if _, err := time.LoadLocation(tz); err == nil {
			return tz
		}
	}
	if link, err := os.Readlink("/etc/localtime"); err == nil {
		if idx := strings.Index(link, "zoneinfo/"); idx >= 0 {
			tz := strings.TrimPrefix(link[idx:], "zoneinfo/")
			if tz != "" {
				if _, err := time.LoadLocation(tz); err == nil {
					return tz
				}
			}
		}
	}
	name := time.Local.String()
	if name != "" && name != "Local" {
		if _, err := time.LoadLocation(name); err == nil {
			return name
		}
	}
	return "UTC"
}

func ResolveTimezone(explicit, configured string) (string, error) {
	tz := strings.TrimSpace(explicit)
	if tz == "" {
		tz = strings.TrimSpace(configured)
	}
	if tz == "" {
		tz = DetectTimezone()
	}
	if _, err := time.LoadLocation(tz); err != nil {
		return "", fmt.Errorf("invalid IANA timezone %q: %w", tz, err)
	}
	return tz, nil
}

func ParseTimeMs(raw string, tzName string) (int64, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, nil
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		// Treat 13+ digit unix timestamps as milliseconds; 10-digit timestamps as seconds.
		if n >= 1_000_000_000_000 || n <= -1_000_000_000_000 {
			return n, nil
		}
		return n * 1000, nil
	}
	loc := time.UTC
	if tzName != "" {
		loaded, err := time.LoadLocation(tzName)
		if err != nil {
			return 0, fmt.Errorf("invalid timezone %q: %w", tzName, err)
		}
		loc = loaded
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UnixMilli(), nil
		}
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02 15:04", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return t.UnixMilli(), nil
		}
	}
	return 0, fmt.Errorf("unsupported time format %q (use RFC3339, YYYY-MM-DD[ HH:MM[:SS]], or unix ms)", s)
}
