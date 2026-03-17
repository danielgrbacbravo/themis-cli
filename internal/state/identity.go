package state

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
)

// CanonicalizeURL enforces contract URL identity for persisted nodes.
func CanonicalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("URL is empty")
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}
	if !parsed.IsAbs() {
		return "", fmt.Errorf("URL must be absolute")
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", fmt.Errorf("invalid URL scheme: %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("URL must include host")
	}

	parsed.RawQuery = ""
	parsed.Fragment = ""
	if parsed.Path != "/" {
		parsed.Path = strings.TrimRight(parsed.Path, "/")
	}

	return parsed.String(), nil
}

func NodeIDFromCanonicalURL(canonicalURL string) string {
	sum := sha256.Sum256([]byte(canonicalURL))
	return "url:" + hex.EncodeToString(sum[:])
}

func NodeIDFromURL(rawURL string) (string, string, error) {
	canonicalURL, err := CanonicalizeURL(rawURL)
	if err != nil {
		return "", "", err
	}
	return NodeIDFromCanonicalURL(canonicalURL), canonicalURL, nil
}
