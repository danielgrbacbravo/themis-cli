package themis

import (
	"errors"
	"os"
	"strings"
)

// LoadAuthSettings loads persisted global auth preferences/credentials.
// Missing session files are treated as empty settings.
func LoadAuthSettings(sessionFile string) (SessionAuthSettings, error) {
	path, err := resolveSessionFilePath(AuthConfig{SessionFile: sessionFile})
	if err != nil {
		return SessionAuthSettings{}, err
	}

	state, err := loadSessionState(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return SessionAuthSettings{}, nil
		}
		return SessionAuthSettings{}, err
	}

	return normalizeAuthSettings(state.Auth), nil
}

// SaveAuthSettings persists global auth preferences/credentials while preserving
// all non-auth session fields (including cookies and user metadata).
func SaveAuthSettings(sessionFile string, auth SessionAuthSettings) error {
	path, err := resolveSessionFilePath(AuthConfig{SessionFile: sessionFile})
	if err != nil {
		return err
	}

	state, err := loadSessionState(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		state = SessionState{
			SchemaVersion: sessionFileSchemaVersion,
			Cookies:       []SessionCookieScope{},
		}
	}

	if state.SchemaVersion == 0 {
		state.SchemaVersion = sessionFileSchemaVersion
	}
	if state.Cookies == nil {
		state.Cookies = []SessionCookieScope{}
	}

	state.Auth = normalizeAuthSettings(auth)
	return SaveSessionState(path, state)
}

func normalizeAuthSettings(auth SessionAuthSettings) SessionAuthSettings {
	auth.Username = strings.TrimSpace(auth.Username)
	if auth.SavePassword {
		auth.SaveUsername = true
	}
	if !auth.SaveUsername {
		auth.Username = ""
		auth.SavePassword = false
		auth.Password = ""
	}
	if !auth.SavePassword {
		auth.Password = ""
	}
	return auth
}
