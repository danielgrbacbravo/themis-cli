package themis

import (
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

const userDataRoute = "/user"

type Session struct {
	BaseURL string
	Client  *http.Client
}

type AuthConfig struct {
	CookieFile        string
	CookieEnv         string
	DefaultCookiePath string
}

type UserData struct {
	FullName      string
	Email         string
	FirstLoggedIn string
	LastLoggedIn  string
}

func NewSession(baseURL string, cookiePath string) (*Session, error) {
	return NewSessionWithAuthConfig(baseURL, AuthConfig{CookieFile: cookiePath})
}

func NewSessionWithAuthConfig(baseURL string, authConfig AuthConfig) (*Session, error) {
	normalizedBaseURL, err := NormalizeBaseURL(baseURL)
	if err != nil {
		return nil, err
	}

	client, err := initializeHTTPClient()
	if err != nil {
		return nil, err
	}

	parsedBaseURL, err := url.Parse(normalizedBaseURL)
	if err != nil {
		return nil, err
	}

	cookies, _, err := resolveCookies(authConfig)
	if err != nil {
		return nil, err
	}

	client.Jar.SetCookies(parsedBaseURL, cookies)

	return &Session{
		BaseURL: normalizedBaseURL,
		Client:  client,
	}, nil
}

func (s *Session) GetUserData() (UserData, error) {
	doc, statusCode, err := s.getDataFromUserPage()
	if err != nil {
		return UserData{}, err
	}
	if statusCode != http.StatusOK {
		return UserData{}, fmt.Errorf("user endpoint returned status %d", statusCode)
	}

	userData := make(map[string]string)
	doc.Find("section.border.accent div.cfg-container div.cfg-line").Each(func(i int, sel *goquery.Selection) {
		key := strings.TrimSpace(sel.Find("span.cfg-key").Text())
		value := strings.TrimSpace(sel.Find("span.cfg-val").Text())
		userData[key] = value
	})

	return UserData{
		FullName:      userData["Full name:"],
		Email:         userData["Email:"],
		FirstLoggedIn: trimDate(userData["First login:"]),
		LastLoggedIn:  trimDate(userData["Last login:"]),
	}, nil
}

func (s *Session) CheckBaseURLAccess() error {
	resp, err := s.Client.Get(s.BaseURL)
	if err != nil {
		return fmt.Errorf("error accessing base URL: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("base URL returned status %d", resp.StatusCode)
	}
	return nil
}

func (s *Session) ValidateAuthentication() (UserData, error) {
	userData, err := s.GetUserData()
	if err != nil {
		return UserData{}, err
	}
	if userData.FullName == "" {
		return UserData{}, fmt.Errorf("authentication check failed: no user profile data found")
	}
	return userData, nil
}

func (s *Session) getDataFromUserPage() (*goquery.Document, int, error) {
	resp, err := s.Client.Get(s.BaseURL + userDataRoute)
	if err != nil {
		return nil, 0, fmt.Errorf("error fetching user data page: %w", err)
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("error parsing user data page: %w", err)
	}
	return doc, resp.StatusCode, nil
}

func initializeHTTPClient() (*http.Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}

	return &http.Client{Jar: jar}, nil
}

func loadCookiesFromFile(path string) (string, error) {
	rawCookie, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	cookieString := strings.TrimSpace(string(rawCookie))
	if cookieString == "" {
		return "", fmt.Errorf("cookie file is empty: %s", path)
	}
	return cookieString, nil
}

func parseCookieString(cookieString string, source string) ([]*http.Cookie, error) {
	cookiePairs := strings.Split(cookieString, ";")
	cookies := make([]*http.Cookie, 0, len(cookiePairs))

	for _, pair := range cookiePairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}

		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
			return nil, fmt.Errorf("invalid cookie pair in %s: %q", source, pair)
		}

		cookies = append(cookies, &http.Cookie{
			Name:  strings.TrimSpace(parts[0]),
			Value: strings.TrimSpace(parts[1]),
		})
	}

	if len(cookies) == 0 {
		return nil, fmt.Errorf("no valid cookies found in %s", source)
	}

	return cookies, nil
}

func resolveCookies(authConfig AuthConfig) ([]*http.Cookie, string, error) {
	attemptErrors := make([]string, 0, 3)

	cookieFile := strings.TrimSpace(authConfig.CookieFile)
	if cookieFile != "" {
		cookieString, err := loadCookiesFromFile(cookieFile)
		if err != nil {
			attemptErrors = append(attemptErrors, fmt.Sprintf("--cookie-file %q: %v", cookieFile, err))
		} else {
			cookies, parseErr := parseCookieString(cookieString, fmt.Sprintf("file %q", cookieFile))
			if parseErr != nil {
				attemptErrors = append(attemptErrors, fmt.Sprintf("--cookie-file %q: %v", cookieFile, parseErr))
			} else {
				return cookies, "cookie-file", nil
			}
		}
	}

	cookieEnv := strings.TrimSpace(authConfig.CookieEnv)
	if cookieEnv != "" {
		cookieString := strings.TrimSpace(os.Getenv(cookieEnv))
		if cookieString == "" {
			attemptErrors = append(attemptErrors, fmt.Sprintf("--cookie-env %q: environment variable is unset or empty", cookieEnv))
		} else {
			cookies, err := parseCookieString(cookieString, fmt.Sprintf("env %q", cookieEnv))
			if err != nil {
				attemptErrors = append(attemptErrors, fmt.Sprintf("--cookie-env %q: %v", cookieEnv, err))
			} else {
				return cookies, "cookie-env", nil
			}
		}
	}

	defaultCookiePath := strings.TrimSpace(authConfig.DefaultCookiePath)
	if defaultCookiePath != "" {
		cookieString, err := loadCookiesFromFile(defaultCookiePath)
		if err != nil {
			attemptErrors = append(attemptErrors, fmt.Sprintf("default path %q: %v", defaultCookiePath, err))
		} else {
			cookies, parseErr := parseCookieString(cookieString, fmt.Sprintf("default file %q", defaultCookiePath))
			if parseErr != nil {
				attemptErrors = append(attemptErrors, fmt.Sprintf("default path %q: %v", defaultCookiePath, parseErr))
			} else {
				return cookies, "default-path", nil
			}
		}
	}

	if len(attemptErrors) == 0 {
		return nil, "", fmt.Errorf("no valid cookie source configured; provide --cookie-file, --cookie-env, or a default cookie path")
	}

	return nil, "", fmt.Errorf("no valid cookie source available: %s", strings.Join(attemptErrors, "; "))
}

func trimDate(value string) string {
	if len(value) < 15 {
		return value
	}
	return value[:15]
}
