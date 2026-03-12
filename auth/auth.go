package auth

import (
	// system packages
	"fmt"
	"net/http"
	"os"
	"strings"
)

// LoadCookiesFromFile reads a cookie header string from file and returns parsed HTTP cookies.
// Expected format example: "name1=value1; name2=value2".
func LoadCookiesFromFile(path string) ([]*http.Cookie, error) {
	rawCookie, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cookieString := strings.TrimSpace(string(rawCookie))
	if cookieString == "" {
		return nil, fmt.Errorf("cookie file is empty: %s", path)
	}

	cookiePairs := strings.Split(cookieString, ";")
	cookies := make([]*http.Cookie, 0, len(cookiePairs))

	for _, pair := range cookiePairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}

		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
			return nil, fmt.Errorf("invalid cookie pair in %s: %q", path, pair)
		}

		cookies = append(cookies, &http.Cookie{
			Name:  strings.TrimSpace(parts[0]),
			Value: strings.TrimSpace(parts[1]),
		})
	}

	if len(cookies) == 0 {
		return nil, fmt.Errorf("no valid cookies found in %s", path)
	}

	return cookies, nil
}
