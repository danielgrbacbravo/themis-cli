package auth

import (
	// system packages
	"fmt"
	"net/http"
	"net/url"

	// third party packages
	"github.com/PuerkitoBio/goquery"
)

// getCsrfToken retrieves the CSRF token from the provided goquery.Document.
// It searches for an input element with the name '_csrf' and returns its value.
// If the CSRF token is not found, it returns an error.
func GetCsrfToken(doc *goquery.Document) (string, error) {
	csrfToken, exists := doc.Find("input[name='_csrf']").Attr("value")
	if !exists {
		return "", fmt.Errorf("CSRF token not found")
	}
	return csrfToken, nil
}

// Login performs a login operation using the provided http.Client, route, and loginData.
// It sends a POST request to the specified route with the login data and returns the updated http.Client and an error, if any.
// If the login is successful, the returned http.Client will be updated with the necessary authentication information.
// If the login fails, an error is returned along with the corresponding status code.
func Login(client http.Client, route string, loginData url.Values) (http.Client, error) {
	resp, err := client.PostForm(route, loginData)

	if err != nil {
		fmt.Println("Error logging in:", err)
		return client, err
	}
	defer resp.Body.Close()

	// Check if login was successful
	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Login failed, status code = %d\n", resp.StatusCode)
		return client, fmt.Errorf("Login failed, status code = %d\n", resp.StatusCode)
	}
	fmt.Println("Login successful")

	return client, nil
}
