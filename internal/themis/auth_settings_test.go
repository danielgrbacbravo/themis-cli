package themis

import (
	"path/filepath"
	"testing"
)

func TestLoadAuthSettings_MissingFileReturnsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	auth, err := LoadAuthSettings(path)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if auth != (SessionAuthSettings{}) {
		t.Fatalf("expected empty auth settings, got: %#v", auth)
	}
}

func TestSaveAuthSettings_PreservesCookiesAndUser(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	in := SessionState{
		SchemaVersion: 1,
		BaseURL:       "https://themis.housing.rug.nl",
		User: &SessionUser{
			FullName: "Alice Example",
			Email:    "alice@example.com",
		},
		Auth: SessionAuthSettings{
			Username:     "old",
			Password:     "old-secret",
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
		t.Fatalf("seed state: %v", err)
	}

	if err := SaveAuthSettings(path, SessionAuthSettings{
		Username:     "new-user",
		Password:     "new-secret",
		SaveUsername: true,
		SavePassword: true,
	}); err != nil {
		t.Fatalf("save auth settings: %v", err)
	}

	out, err := loadSessionState(path)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if out.User == nil || out.User.Email != "alice@example.com" {
		t.Fatalf("expected user metadata preserved, got: %#v", out.User)
	}
	if len(out.Cookies) != 1 || len(out.Cookies[0].Cookies) != 1 || out.Cookies[0].Cookies[0].Value != "abc" {
		t.Fatalf("expected cookies preserved, got: %#v", out.Cookies)
	}
	if out.Auth.Username != "new-user" || out.Auth.Password != "new-secret" {
		t.Fatalf("unexpected updated auth settings: %#v", out.Auth)
	}
}

func TestSaveAuthSettings_NormalizesDisabledPassword(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	if err := SaveAuthSettings(path, SessionAuthSettings{
		Username:     "alice",
		Password:     "secret",
		SaveUsername: true,
		SavePassword: false,
	}); err != nil {
		t.Fatalf("save auth settings: %v", err)
	}

	auth, err := LoadAuthSettings(path)
	if err != nil {
		t.Fatalf("load auth settings: %v", err)
	}
	if auth.Password != "" {
		t.Fatalf("expected password to be cleared when save_password=false, got: %q", auth.Password)
	}
	if !auth.SaveUsername {
		t.Fatalf("expected save_username=true")
	}
}
