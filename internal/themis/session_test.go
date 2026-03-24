package themis

import (
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadSessionState_JSON(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "session.json")
	content := `{
  "schema_version": 1,
  "base_url": "https://themis.housing.rug.nl",
  "auth": {
    "username": "alice",
    "password": "secret",
    "save_username": true,
    "save_password": true
  },
  "cookies": [
    {
      "url": "https://themis.housing.rug.nl",
      "cookies": [
        {"name": "session", "value": "abc"}
      ]
    }
  ]
}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	state, err := loadSessionState(path)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if state.SchemaVersion != 1 {
		t.Fatalf("unexpected schema version: %d", state.SchemaVersion)
	}
	if state.Auth.Username != "alice" {
		t.Fatalf("unexpected username: %q", state.Auth.Username)
	}
	if len(state.Cookies) != 1 {
		t.Fatalf("unexpected cookie scopes: %#v", state.Cookies)
	}
	if len(state.Cookies[0].Cookies) != 1 || state.Cookies[0].Cookies[0].Value != "abc" {
		t.Fatalf("unexpected cookies: %#v", state.Cookies[0].Cookies)
	}
}

func TestLoadSessionState_LegacyCookieString(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "session.json")
	if err := os.WriteFile(path, []byte("session=abc; csrftoken=def"), 0o644); err != nil {
		t.Fatal(err)
	}

	state, err := loadSessionState(path)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(state.Cookies) != 1 {
		t.Fatalf("unexpected cookie scopes: %#v", state.Cookies)
	}
	if len(state.Cookies[0].Cookies) != 2 {
		t.Fatalf("unexpected cookie count: %#v", state.Cookies[0].Cookies)
	}
}

func TestRestoreCookieJar_ScopedCookies(t *testing.T) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}

	baseURL, err := url.Parse("https://themis.housing.rug.nl")
	if err != nil {
		t.Fatal(err)
	}

	state := SessionState{
		SchemaVersion: 1,
		Cookies: []SessionCookieScope{
			{
				URL: "https://themis.housing.rug.nl",
				Cookies: []SessionCookie{
					{Name: "session", Value: "abc"},
				},
			},
		},
	}

	if err := restoreCookieJar(jar, baseURL, state); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	cookies := jar.Cookies(baseURL)
	if len(cookies) != 1 || cookies[0].Value != "abc" {
		t.Fatalf("unexpected restored cookies: %#v", cookies)
	}
}

func TestSaveSessionState_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "session.json")
	now := time.Now().UTC().Truncate(time.Second)

	in := SessionState{
		SchemaVersion:       1,
		BaseURL:             "https://themis.housing.rug.nl",
		LastAuthenticatedAt: &now,
		User: &SessionUser{
			FullName: "Alice Example",
			Email:    "alice@example.com",
		},
		Auth: SessionAuthSettings{
			Username:     "alice",
			Password:     "secret",
			SaveUsername: true,
			SavePassword: true,
		},
		Cookies: []SessionCookieScope{
			{
				URL: "https://themis.housing.rug.nl",
				Cookies: []SessionCookie{
					{Name: "session", Value: "abc"},
				},
			},
		},
	}

	if err := SaveSessionState(path, in); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	out, err := loadSessionState(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if out.BaseURL != in.BaseURL {
		t.Fatalf("unexpected base URL: %q", out.BaseURL)
	}
	if out.Auth.Username != in.Auth.Username || out.Auth.Password != in.Auth.Password {
		t.Fatalf("unexpected auth payload: %#v", out.Auth)
	}
	if len(out.Cookies) != 1 || len(out.Cookies[0].Cookies) != 1 || out.Cookies[0].Cookies[0].Value != "abc" {
		t.Fatalf("unexpected cookies: %#v", out.Cookies)
	}
}

func TestSessionSaveState_WritesBaseURLCookieScope(t *testing.T) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	baseURL, err := url.Parse("https://themis.housing.rug.nl")
	if err != nil {
		t.Fatal(err)
	}
	jar.SetCookies(baseURL, toHTTPCookies([]SessionCookie{{Name: "session", Value: "abc"}}))

	s := &Session{
		BaseURL: "https://themis.housing.rug.nl",
		Client:  &http.Client{Jar: jar},
	}

	path := filepath.Join(t.TempDir(), "session.json")
	now := time.Now().UTC()
	if err := s.SaveState(path, SessionAuthSettings{Username: "alice"}, nil, now); err != nil {
		t.Fatalf("save state failed: %v", err)
	}

	state, err := loadSessionState(path)
	if err != nil {
		t.Fatalf("load state failed: %v", err)
	}
	if len(state.Cookies) != 1 || state.Cookies[0].URL != "https://themis.housing.rug.nl" {
		t.Fatalf("unexpected cookie scopes: %#v", state.Cookies)
	}
}
