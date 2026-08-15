package config

import (
	"strconv"
	"strings"

	"github.com/sandeepv/apilens/internal/security"
)

// parseSize parses simple human sizes: "5MB", "512KB", "1048576", "1GB".
// Unrecognized input falls back to the product default rather than erroring
// out of a run — a malformed size is a config detail, not worth failing the
// whole suite over in v1.
func parseSize(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return security.DefaultMaxResponseSize
	}
	upper := strings.ToUpper(s)
	multiplier := int64(1)
	numPart := upper
	switch {
	case strings.HasSuffix(upper, "GB"):
		multiplier = 1024 * 1024 * 1024
		numPart = strings.TrimSuffix(upper, "GB")
	case strings.HasSuffix(upper, "MB"):
		multiplier = 1024 * 1024
		numPart = strings.TrimSuffix(upper, "MB")
	case strings.HasSuffix(upper, "KB"):
		multiplier = 1024
		numPart = strings.TrimSuffix(upper, "KB")
	case strings.HasSuffix(upper, "B"):
		numPart = strings.TrimSuffix(upper, "B")
	}
	n, err := strconv.ParseInt(strings.TrimSpace(numPart), 10, 64)
	if err != nil {
		return security.DefaultMaxResponseSize
	}
	return n * multiplier
}
