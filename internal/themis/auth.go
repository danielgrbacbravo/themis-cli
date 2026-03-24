package themis

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const (
	loginStartPath = "/log/in/oidc"
	maxAuthSteps   = 32
)

var ErrInvalidCredentials = errors.New("invalid credentials")

type LoginRequest struct {
	Username     string
	Password     string
	TOTP         string
	SaveUsername bool
	SavePassword bool
}

type LoginResult struct {
	User UserData
}

// PerformSSOLogin executes the observed multi-step SSO flow and persists session state.
func PerformSSOLogin(baseURL string, authConfig AuthConfig, req LoginRequest) (LoginResult, error) {
	normalizedBaseURL, err := NormalizeBaseURL(baseURL)
	if err != nil {
		return LoginResult{}, err
	}
	if strings.TrimSpace(req.Username) == "" || strings.TrimSpace(req.Password) == "" {
		return LoginResult{}, fmt.Errorf("%w: username and password are required", ErrMissingCredentials)
	}
	if strings.TrimSpace(req.TOTP) == "" {
		return LoginResult{}, fmt.Errorf("%w: totp is required", ErrMissingCredentials)
	}

	client, err := initializeHTTPClient()
	if err != nil {
		return LoginResult{}, err
	}

	if err := runSSOSequence(client, normalizedBaseURL, req); err != nil {
		return LoginResult{}, err
	}

	session := &Session{BaseURL: normalizedBaseURL, Client: client}
	userData, err := session.ValidateAuthentication()
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "status 401") || strings.Contains(strings.ToLower(err.Error()), "status 403") {
			return LoginResult{}, fmt.Errorf("%w: %v", ErrSessionExpired, err)
		}
		return LoginResult{}, err
	}

	sessionFilePath, err := resolveSessionFilePath(authConfig)
	if err != nil {
		return LoginResult{}, err
	}
	authSettings := SessionAuthSettings{
		SaveUsername: req.SaveUsername || req.SavePassword,
		SavePassword: req.SavePassword,
	}
	if authSettings.SaveUsername {
		authSettings.Username = strings.TrimSpace(req.Username)
	}
	if authSettings.SavePassword {
		authSettings.Password = req.Password
	}
	user := &SessionUser{
		FullName:      userData.FullName,
		Email:         userData.Email,
		FirstLoggedIn: userData.FirstLoggedIn,
		LastLoggedIn:  userData.LastLoggedIn,
	}
	if err := session.SaveState(sessionFilePath, authSettings, user, time.Now().UTC()); err != nil {
		return LoginResult{}, err
	}

	return LoginResult{User: userData}, nil
}

func runSSOSequence(client *http.Client, baseURL string, req LoginRequest) error {
	resp, err := newAuthRequest(client, http.MethodGet, strings.TrimRight(baseURL, "/")+loginStartPath, "", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	lastStage := ""
	for i := 0; i < maxAuthSteps; i++ {
		doc, err := goquery.NewDocumentFromReader(resp.Body)
		if err != nil {
			return fmt.Errorf("parse auth page: %w", err)
		}
		next, stage, stageErr := nextAuthAction(doc, resp.Request.URL, req)
		if stageErr == nil && next == nil {
			return nil
		}
		if stageErr != nil {
			return stageErr
		}

		lastStage = stage
		resp.Body.Close()
		resp, err = newAuthRequest(client, next.method, next.targetURL, next.referer, next.values)
		if err != nil {
			return err
		}
	}

	if lastStage == "mfa" {
		return fmt.Errorf("%w: mfa challenge did not complete", ErrInvalidMFA)
	}
	if lastStage == "credentials" {
		return fmt.Errorf("%w: credential challenge did not complete", ErrInvalidCredentials)
	}
	return fmt.Errorf("%w: login sequence did not complete", ErrNotAuthenticated)
}

type authAction struct {
	method    string
	targetURL string
	referer   string
	values    url.Values
}

func nextAuthAction(doc *goquery.Document, pageURL *url.URL, req LoginRequest) (*authAction, string, error) {
	form := doc.Find("form").First()
	if form.Length() == 0 {
		return nil, "", nil
	}

	actionURL, values, err := buildFormRequest(form, pageURL)
	if err != nil {
		return nil, "", err
	}

	if hasInput(form, "Ecom_User_ID") && hasInput(form, "Ecom_Password") {
		values.Set("Ecom_User_ID", strings.TrimSpace(req.Username))
		values.Set("Ecom_Password", req.Password)
		values.Set("option", "credential")
		return &authAction{
			method:    http.MethodPost,
			targetURL: actionURL.String(),
			referer:   pageURL.String(),
			values:    values,
		}, "credentials", nil
	}

	if hasInput(form, "nffc") {
		values.Set("nffc", strings.TrimSpace(req.TOTP))
		values.Set("option", "credential")
		return &authAction{
			method:    http.MethodPost,
			targetURL: actionURL.String(),
			referer:   pageURL.String(),
			values:    values,
		}, "mfa", nil
	}

	method := strings.ToUpper(strings.TrimSpace(form.AttrOr("method", "GET")))
	if method != http.MethodGet {
		method = http.MethodPost
	}
	return &authAction{
		method:    method,
		targetURL: actionURL.String(),
		referer:   pageURL.String(),
		values:    values,
	}, "relay", nil
}

func buildFormRequest(form *goquery.Selection, pageURL *url.URL) (*url.URL, url.Values, error) {
	action := strings.TrimSpace(form.AttrOr("action", ""))
	targetURL := pageURL
	if action != "" {
		parsedAction, err := pageURL.Parse(action)
		if err != nil {
			return nil, nil, fmt.Errorf("parse form action %q: %w", action, err)
		}
		targetURL = parsedAction
	}
	values := url.Values{}
	form.Find("input").Each(func(_ int, input *goquery.Selection) {
		name := strings.TrimSpace(input.AttrOr("name", ""))
		if name == "" {
			return
		}
		values.Set(name, input.AttrOr("value", ""))
	})
	return targetURL, values, nil
}

func hasInput(form *goquery.Selection, name string) bool {
	return form.Find(fmt.Sprintf(`input[name="%s"]`, name)).Length() > 0
}

func newAuthRequest(client *http.Client, method string, rawURL string, referer string, values url.Values) (*http.Response, error) {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = http.MethodGet
	}

	var req *http.Request
	var err error
	switch method {
	case http.MethodPost:
		req, err = http.NewRequest(http.MethodPost, rawURL, strings.NewReader(values.Encode()))
		if err != nil {
			return nil, fmt.Errorf("build auth post request: %w", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	default:
		targetURL, parseErr := url.Parse(rawURL)
		if parseErr != nil {
			return nil, fmt.Errorf("parse auth get url %q: %w", rawURL, parseErr)
		}
		query := targetURL.Query()
		for key, list := range values {
			query.Del(key)
			for _, value := range list {
				query.Add(key, value)
			}
		}
		targetURL.RawQuery = query.Encode()
		req, err = http.NewRequest(http.MethodGet, targetURL.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("build auth get request: %w", err)
		}
	}

	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	if strings.TrimSpace(referer) != "" {
		req.Header.Set("Referer", referer)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute auth request: %w", err)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return nil, fmt.Errorf("%w: auth endpoint returned status %d", ErrNotAuthenticated, resp.StatusCode)
		}
		return nil, fmt.Errorf("auth endpoint returned status %d", resp.StatusCode)
	}
	return resp, nil
}
