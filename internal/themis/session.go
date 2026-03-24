package themis

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const userDataRoute = "/user"
const sessionFileSchemaVersion = 1

var (
	ErrNotAuthenticated   = errors.New("not authenticated")
	ErrSessionExpired     = errors.New("session expired")
	ErrMissingCredentials = errors.New("missing credentials")
	ErrInvalidMFA         = errors.New("invalid mfa")
)

type Session struct {
	BaseURL string
	Client  *http.Client
}

type AuthConfig struct {
	SessionFile string
}

type UserData struct {
	FullName      string
	Email         string
	FirstLoggedIn string
	LastLoggedIn  string
}

func NewSession(baseURL string, cookiePath string) (*Session, error) {
	return NewSessionWithAuthConfig(baseURL, AuthConfig{SessionFile: cookiePath})
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

	sessionFilePath, err := resolveSessionFilePath(authConfig)
	if err != nil {
		return nil, err
	}

	sessionState, err := loadSessionState(sessionFilePath)
	if err != nil {
		return nil, classifyLoadSessionError(err)
	}

	if err := restoreCookieJar(client.Jar, parsedBaseURL, sessionState); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotAuthenticated, err)
	}

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
		return UserData{}, fmt.Errorf("%w: authentication check failed: no user profile data found", ErrSessionExpired)
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
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, resp.StatusCode, fmt.Errorf("%w: user endpoint returned status %d", ErrSessionExpired, resp.StatusCode)
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

func resolveSessionFilePath(authConfig AuthConfig) (string, error) {
	if strings.TrimSpace(authConfig.SessionFile) != "" {
		return strings.TrimSpace(authConfig.SessionFile), nil
	}

	return DefaultSessionFilePath()
}

func DefaultSessionFilePath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home directory: %w", err)
	}

	return filepath.Join(homeDir, ".config", "themis", "session.json"), nil
}

func loadSessionState(path string) (SessionState, error) {
	rawState, err := os.ReadFile(path)
	if err != nil {
		return SessionState{}, fmt.Errorf("read session file %q: %w", path, err)
	}

	rawTrimmed := strings.TrimSpace(string(rawState))
	if rawTrimmed == "" {
		return SessionState{}, fmt.Errorf("session file is empty: %s", path)
	}

	var sessionState SessionState
	if err := json.Unmarshal([]byte(rawTrimmed), &sessionState); err != nil {
		legacyCookies, parseErr := parseCookieString(rawTrimmed, fmt.Sprintf("legacy session file %q", path))
		if parseErr != nil {
			return SessionState{}, fmt.Errorf("decode session state from %q: %w", path, err)
		}

		sessionState = SessionState{
			SchemaVersion: sessionFileSchemaVersion,
			Cookies: []SessionCookieScope{
				{
					URL:     "",
					Cookies: fromHTTPCookies(legacyCookies),
				},
			},
		}
	}

	if sessionState.SchemaVersion == 0 {
		sessionState.SchemaVersion = sessionFileSchemaVersion
	}
	if sessionState.Cookies == nil {
		sessionState.Cookies = []SessionCookieScope{}
	}
	return sessionState, nil
}

func LoadSessionState(path string) (SessionState, error) {
	if strings.TrimSpace(path) == "" {
		var err error
		path, err = DefaultSessionFilePath()
		if err != nil {
			return SessionState{}, err
		}
	}
	return loadSessionState(path)
}

func SaveSessionState(path string, state SessionState) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("session file path is required")
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create session directory: %w", err)
	}

	if state.SchemaVersion == 0 {
		state.SchemaVersion = sessionFileSchemaVersion
	}
	if state.Cookies == nil {
		state.Cookies = []SessionCookieScope{}
	}

	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode session state: %w", err)
	}
	payload = append(payload, '\n')
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		return fmt.Errorf("write session file %q: %w", path, err)
	}
	return nil
}

func (s *Session) SaveState(path string, auth SessionAuthSettings, user *SessionUser, lastAuthenticatedAt time.Time) error {
	baseURL, err := url.Parse(s.BaseURL)
	if err != nil {
		return fmt.Errorf("parse base URL %q: %w", s.BaseURL, err)
	}

	state := SessionState{
		SchemaVersion:       sessionFileSchemaVersion,
		BaseURL:             s.BaseURL,
		LastAuthenticatedAt: &lastAuthenticatedAt,
		User:                user,
		Auth:                auth,
		Cookies: []SessionCookieScope{
			{
				URL:     s.BaseURL,
				Cookies: fromHTTPCookies(s.Client.Jar.Cookies(baseURL)),
			},
		},
	}

	return SaveSessionState(path, state)
}

func restoreCookieJar(jar http.CookieJar, baseURL *url.URL, state SessionState) error {
	if len(state.Cookies) == 0 {
		return fmt.Errorf("session file has no cookies")
	}

	restored := 0
	for _, scopedCookies := range state.Cookies {
		targetURL := baseURL
		if strings.TrimSpace(scopedCookies.URL) != "" {
			parsed, err := url.Parse(strings.TrimSpace(scopedCookies.URL))
			if err != nil {
				return fmt.Errorf("invalid cookie scope URL %q: %w", scopedCookies.URL, err)
			}
			if parsed.Scheme == "" || parsed.Host == "" {
				return fmt.Errorf("invalid cookie scope URL %q", scopedCookies.URL)
			}
			targetURL = parsed
		}

		cookies := toHTTPCookies(scopedCookies.Cookies)
		if len(cookies) == 0 {
			continue
		}
		jar.SetCookies(targetURL, cookies)
		restored += len(cookies)
	}

	if restored == 0 {
		return fmt.Errorf("session file has no valid cookies")
	}
	return nil
}

type SessionState struct {
	SchemaVersion       int                  `json:"schema_version"`
	BaseURL             string               `json:"base_url,omitempty"`
	LastAuthenticatedAt *time.Time           `json:"last_authenticated_at,omitempty"`
	User                *SessionUser         `json:"user,omitempty"`
	Auth                SessionAuthSettings  `json:"auth"`
	Cookies             []SessionCookieScope `json:"cookies"`
}

type SessionUser struct {
	FullName      string `json:"full_name,omitempty"`
	Email         string `json:"email,omitempty"`
	FirstLoggedIn string `json:"first_logged_in,omitempty"`
	LastLoggedIn  string `json:"last_logged_in,omitempty"`
}

type SessionAuthSettings struct {
	Username     string `json:"username,omitempty"`
	Password     string `json:"password,omitempty"`
	SaveUsername bool   `json:"save_username"`
	SavePassword bool   `json:"save_password"`
}

type SessionCookieScope struct {
	URL     string          `json:"url,omitempty"`
	Cookies []SessionCookie `json:"cookies"`
}

type SessionCookie struct {
	Name     string    `json:"name"`
	Value    string    `json:"value"`
	Path     string    `json:"path,omitempty"`
	Domain   string    `json:"domain,omitempty"`
	Expires  time.Time `json:"expires,omitempty"`
	MaxAge   int       `json:"max_age,omitempty"`
	Secure   bool      `json:"secure,omitempty"`
	HTTPOnly bool      `json:"http_only,omitempty"`
	SameSite int       `json:"same_site,omitempty"`
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

func toHTTPCookies(cookies []SessionCookie) []*http.Cookie {
	result := make([]*http.Cookie, 0, len(cookies))
	for _, cookie := range cookies {
		name := strings.TrimSpace(cookie.Name)
		if name == "" {
			continue
		}
		result = append(result, &http.Cookie{
			Name:     name,
			Value:    cookie.Value,
			Path:     cookie.Path,
			Domain:   cookie.Domain,
			Expires:  cookie.Expires,
			MaxAge:   cookie.MaxAge,
			Secure:   cookie.Secure,
			HttpOnly: cookie.HTTPOnly,
			SameSite: http.SameSite(cookie.SameSite),
		})
	}
	return result
}

func fromHTTPCookies(cookies []*http.Cookie) []SessionCookie {
	result := make([]SessionCookie, 0, len(cookies))
	for _, cookie := range cookies {
		if cookie == nil || strings.TrimSpace(cookie.Name) == "" {
			continue
		}
		result = append(result, SessionCookie{
			Name:     cookie.Name,
			Value:    cookie.Value,
			Path:     cookie.Path,
			Domain:   cookie.Domain,
			Expires:  cookie.Expires,
			MaxAge:   cookie.MaxAge,
			Secure:   cookie.Secure,
			HTTPOnly: cookie.HttpOnly,
			SameSite: int(cookie.SameSite),
		})
	}
	return result
}

func trimDate(value string) string {
	if len(value) < 15 {
		return value
	}
	return value[:15]
}

func classifyLoadSessionError(err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: %v", ErrNotAuthenticated, err)
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "session file is empty") || strings.Contains(msg, "session file has no cookies") {
		return fmt.Errorf("%w: %v", ErrNotAuthenticated, err)
	}
	return err
}
