package client

import (
	"fmt"
	"log"
	"net/http"
	"net/http/cookiejar"
	"strings"

	// goquery
	"github.com/PuerkitoBio/goquery"
)

// initializeClient initializes and returns an http.Client with a cookie jar.
// The cookie jar is used to store and manage cookies for subsequent requests.
// Returns:
//   - http.Client: The initialized http.Client with the cookie jar.
func InitializeClient() http.Client {
	// Initialize cookie jar
	jar, err := cookiejar.New(nil)
	if err != nil {
		log.Fatal(err)
	}

	// Initialize client
	client := http.Client{
		Jar: jar,
	}

	return client
}

// GetLoginPage sends a GET request to the specified baseURL + loginRoute and returns the parsed HTML document and any error encountered.
// It takes an http.Client, baseURL string, and loginRoute string as parameters.
// The returned *goquery.Document represents the parsed HTML document.
// If an error occurs during the request or parsing, it is returned as the second value.
func GetLoginPage(client http.Client, baseURL string, loginRoute string) (*goquery.Document, error) {
	resp, err := client.Get(baseURL + loginRoute)
	if err != nil {
		fmt.Println("Error fetching login page:", err)
		return nil, err
	}
	defer resp.Body.Close()

	// Parse HTML and get CSRF token
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		fmt.Println("Error parsing login page:", err)
		return nil, err
	}
	return doc, nil
}

// GetDataFromUserPage sends a GET request to the specified baseURL + userDataUrl and returns the parsed HTML document and any error encountered.
// It takes an http.Client, baseURL string, and userDataUrl string as parameters.
// The returned *goquery.Document represents the parsed HTML document.
func GetDataFromUserPage(client *http.Client, baseURL string, userDataUrl string) (*goquery.Document, error) {
	resp, err := client.Get(baseURL + userDataUrl)
	if err != nil {
		fmt.Println("Error fetching user data page:", err)
		return nil, err
	}
	defer resp.Body.Close()

	// Parse HTML and get CSRF token
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		fmt.Println("Error parsing user data page:", err)
		return nil, err
	}
	return doc, nil
}

// GetFullname retrieves the full name from the provided goquery.Document.
// It parses the document and extracts the full name from the relevant section.
// The full name is returned as a string.
func GetFullName(client *http.Client, baseURL string) string {
	const userDataUrl = "/user"
	doc, err := GetDataFromUserPage(client, baseURL, userDataUrl)
	if err != nil {
		log.Fatal(err)
	}
	userData := make(map[string]string)
	doc.Find("section.border.accent div.cfg-container div.cfg-line").Each(func(i int, s *goquery.Selection) {
		key := s.Find("span.cfg-key").Text()
		value := s.Find("span.cfg-val").Text()
		userData[strings.TrimSpace(key)] = strings.TrimSpace(value)
	})
	return userData["Full name:"]
}

// It iterates over the sections, finds the key-value pairs, and stores them in a map.
// The last name is then retrieved from the map and returned as a string.
func getLastName(client *http.Client, baseURL string) string {
	const userDataUrl = "/user"
	doc, err := GetDataFromUserPage(client, baseURL, userDataUrl)
	if err != nil {
		log.Fatal(err)
	}
	userData := make(map[string]string)
	doc.Find("section.border.accent div.cfg-container div.cfg-line").Each(func(i int, s *goquery.Selection) {
		key := s.Find("span.cfg-key").Text()
		value := s.Find("span.cfg-val").Text()
		userData[strings.TrimSpace(key)] = strings.TrimSpace(value)
	})
	return userData["Last name:"]
}

// getInital retrieves the value of the "Initial" key from the provided goquery.Document.
// It parses the document and extracts key-value pairs from specific sections and returns the value associated with the "Initial" key.
// The extracted key-value pairs are stored in a map[string]string called userData.
// The function returns an empty string if the "Initial" key is not found in the document.
func getInital(client *http.Client, baseURL string) string {
	const userDataUrl = "/user"
	doc, err := GetDataFromUserPage(client, baseURL, userDataUrl)
	if err != nil {
		log.Fatal(err)
	}
	userData := make(map[string]string)
	doc.Find("section.border.accent div.cfg-container div.cfg-line").Each(func(i int, s *goquery.Selection) {
		key := s.Find("span.cfg-key").Text()
		value := s.Find("span.cfg-val").Text()
		userData[strings.TrimSpace(key)] = strings.TrimSpace(value)
	})
	return userData["Initials:"]
}

// getEmail extracts the email from the given goquery.Document.
// It iterates over the sections, finds the key-value pairs, and stores them in a map.
// Finally, it returns the value associated with the "Email" key in the map
func GetEmail(client *http.Client, baseURL string) string {
	const userDataUrl = "/user"
	doc, err := GetDataFromUserPage(client, baseURL, userDataUrl)
	if err != nil {
		log.Fatal(err)
	}
	userData := make(map[string]string)
	// find all divs with class cfg-line
	doc.Find("section.border.accent div.cfg-container div.cfg-line").Each(func(i int, s *goquery.Selection) {
		key := s.Find("span.cfg-key").Text()
		value := s.Find("span.cfg-val").Text()
		// trim spaces from key and value
		userData[strings.TrimSpace(key)] = strings.TrimSpace(value)
	})
	return userData["Email:"]
}

// getFirstLoginDate retrieves the first login date from the provided goquery.Document.
// It parses the document and extracts the first login date value from the relevant section.
// The extracted value is returned as a string.
func GetFirstLoggedIn(client *http.Client, baseURL string) string {
	const userDataUrl = "/user"
	doc, err := GetDataFromUserPage(client, baseURL, userDataUrl)
	if err != nil {
		log.Fatal(err)
	}
	userData := make(map[string]string)
	doc.Find("section.border.accent div.cfg-container div.cfg-line").Each(func(i int, s *goquery.Selection) {
		key := s.Find("span.cfg-key").Text()
		value := s.Find("span.cfg-val").Text()
		// trim spaces from key and value
		userData[strings.TrimSpace(key)] = strings.TrimSpace(value)
	})
	// note that the date in the fromat Sat Nov 18 2023 11:54:03 GMT+0100
	// returns the first 15 characters of the string (Sat Nov 18 2023)

	output := userData["First login:"]
	return output[:15]
}

// getLastLoginDate retrieves the last login date from the provided goquery.Document.
// It parses the document and extracts the last login date value from the relevant section.
// The extracted value is returned as a string.
func GetLastLoggedIn(client *http.Client, baseURL string) string {
	const userDataUrl = "/user"
	doc, err := GetDataFromUserPage(client, baseURL, userDataUrl)
	if err != nil {
		log.Fatal(err)
	}
	userData := make(map[string]string)
	doc.Find("section.border.accent div.cfg-container div.cfg-line").Each(func(i int, s *goquery.Selection) {
		key := s.Find("span.cfg-key").Text()
		value := s.Find("span.cfg-val").Text()
		userData[strings.TrimSpace(key)] = strings.TrimSpace(value)
	})
	output := userData["Last login:"]
	return output[:15]

}
