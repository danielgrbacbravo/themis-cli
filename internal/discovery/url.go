package discovery

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var testFilePattern = regexp.MustCompile(`^\d+\.(in|out)$`)

// NormalizeTestsBaseURL normalizes either a tests directory URL or a specific
// test file URL into a canonical tests directory URL ending at "/%40tests".
func NormalizeTestsBaseURL(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", fmt.Errorf("tests URL is empty")
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid tests URL: %w", err)
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", fmt.Errorf("invalid URL scheme: %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("tests URL must include host")
	}

	parsed.RawQuery = ""
	parsed.Fragment = ""

	segments := strings.Split(strings.Trim(parsed.EscapedPath(), "/"), "/")
	testsIndex := -1
	for i := range segments {
		if segments[i] == "%40tests" || segments[i] == "@tests" {
			testsIndex = i
		}
	}
	if testsIndex == -1 {
		return "", fmt.Errorf("URL does not contain tests directory")
	}

	afterTests := segments[testsIndex+1:]
	if len(afterTests) > 1 {
		return "", fmt.Errorf("invalid tests URL path")
	}
	if len(afterTests) == 1 && !testFilePattern.MatchString(afterTests[0]) {
		return "", fmt.Errorf("invalid test file name: %q", afterTests[0])
	}

	baseSegments := append([]string{}, segments[:testsIndex+1]...)
	baseSegments[len(baseSegments)-1] = "%40tests"
	basePath := "/" + strings.Join(baseSegments, "/")

	return parsed.Scheme + "://" + parsed.Host + basePath, nil
}
