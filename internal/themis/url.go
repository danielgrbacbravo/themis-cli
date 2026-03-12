package themis

import (
	"fmt"
	"net/url"
	"strings"
)

// NormalizeBaseURL validates and canonicalizes a Themis base URL.
// It enforces http/https, requires a host, strips query/fragment, and removes a trailing slash.
func NormalizeBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("base URL is empty")
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid base URL: %w", err)
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", fmt.Errorf("invalid base URL scheme: %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("base URL must include host")
	}

	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.Path = strings.TrimRight(parsed.Path, "/")

	return parsed.String(), nil
}
