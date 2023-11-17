package client

import (
	"fmt"
	"log"
	"net/http"
	"net/http/cookiejar"

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
