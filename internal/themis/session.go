package themis

import (
	"fmt"
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

type UserData struct {
	FullName      string
	Email         string
	FirstLoggedIn string
	LastLoggedIn  string
}

func NewSession(baseURL string, cookiePath string) (*Session, error) {
	client, err := initializeHTTPClient()
	if err != nil {
		return nil, err
	}

	parsedBaseURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}

	cookies, err := loadCookiesFromFile(cookiePath)
	if err != nil {
		return nil, err
	}

	client.Jar.SetCookies(parsedBaseURL, cookies)

	return &Session{
		BaseURL: baseURL,
		Client:  client,
	}, nil
}

func (s *Session) GetUserData() (UserData, error) {
	doc, err := s.getDataFromUserPage()
	if err != nil {
		return UserData{}, err
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

func (s *Session) getDataFromUserPage() (*goquery.Document, error) {
	resp, err := s.Client.Get(s.BaseURL + userDataRoute)
	if err != nil {
		return nil, fmt.Errorf("error fetching user data page: %w", err)
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error parsing user data page: %w", err)
	}
	return doc, nil
}

func initializeHTTPClient() (*http.Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}

	return &http.Client{Jar: jar}, nil
}

func loadCookiesFromFile(path string) ([]*http.Cookie, error) {
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

func trimDate(value string) string {
	if len(value) < 15 {
		return value
	}
	return value[:15]
}
