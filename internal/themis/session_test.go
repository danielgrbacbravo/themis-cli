package themis

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveCookies_PrefersCookieFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "cookie.txt")
	if err := os.WriteFile(filePath, []byte("session=file-cookie"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("THEMIS_TEST_COOKIE", "session=env-cookie")

	cookies, source, err := resolveCookies(AuthConfig{
		CookieFile:        filePath,
		CookieEnv:         "THEMIS_TEST_COOKIE",
		DefaultCookiePath: filepath.Join(tmpDir, "missing.txt"),
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if source != "cookie-file" {
		t.Fatalf("unexpected source: %s", source)
	}
	if len(cookies) != 1 || cookies[0].Value != "file-cookie" {
		t.Fatalf("unexpected cookies: %#v", cookies)
	}
}

func TestResolveCookies_FallsBackToCookieEnv(t *testing.T) {
	tmpDir := t.TempDir()
	badFile := filepath.Join(tmpDir, "bad-cookie.txt")
	if err := os.WriteFile(badFile, []byte("not-a-cookie"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("THEMIS_TEST_COOKIE", "session=env-cookie")

	cookies, source, err := resolveCookies(AuthConfig{
		CookieFile:        badFile,
		CookieEnv:         "THEMIS_TEST_COOKIE",
		DefaultCookiePath: filepath.Join(tmpDir, "missing.txt"),
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if source != "cookie-env" {
		t.Fatalf("unexpected source: %s", source)
	}
	if len(cookies) != 1 || cookies[0].Value != "env-cookie" {
		t.Fatalf("unexpected cookies: %#v", cookies)
	}
}

func TestResolveCookies_FallsBackToDefaultPath(t *testing.T) {
	tmpDir := t.TempDir()
	defaultPath := filepath.Join(tmpDir, "default-cookie.txt")
	if err := os.WriteFile(defaultPath, []byte("session=default-cookie"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("THEMIS_TEST_COOKIE", "")

	cookies, source, err := resolveCookies(AuthConfig{
		CookieFile:        filepath.Join(tmpDir, "missing.txt"),
		CookieEnv:         "THEMIS_TEST_COOKIE",
		DefaultCookiePath: defaultPath,
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if source != "default-path" {
		t.Fatalf("unexpected source: %s", source)
	}
	if len(cookies) != 1 || cookies[0].Value != "default-cookie" {
		t.Fatalf("unexpected cookies: %#v", cookies)
	}
}

func TestResolveCookies_ErrorWhenNoValidSource(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("THEMIS_TEST_COOKIE", "")

	_, _, err := resolveCookies(AuthConfig{
		CookieFile:        filepath.Join(tmpDir, "missing.txt"),
		CookieEnv:         "THEMIS_TEST_COOKIE",
		DefaultCookiePath: filepath.Join(tmpDir, "also-missing.txt"),
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	msg := err.Error()
	if !strings.Contains(msg, "--cookie-file") {
		t.Fatalf("expected cookie-file error details, got: %s", msg)
	}
	if !strings.Contains(msg, "--cookie-env") {
		t.Fatalf("expected cookie-env error details, got: %s", msg)
	}
	if !strings.Contains(msg, "default path") {
		t.Fatalf("expected default path error details, got: %s", msg)
	}
}
