package main

// system packages

import (

	// local packages
	"log"
	"themis-cli/auth"
	"themis-cli/client"
	"themis-cli/config"
)

const (
	baseURL     = "https://themis.housing.rug.nl"
	loginRoute  = "/log/in"
	userDataUrl = "/user"
	coursesUrl  = "/courses"
)

func main() {

	httpClient := client.InitializeClient()
	document, err := client.GetLoginPage(httpClient, baseURL, loginRoute)
	if err != nil {
		log.Fatal(err)
		return
	}
	// get csrfToken from goquery document
	csrfToken, err := auth.GetCsrfToken(document)
	if err != nil {
		log.Fatal(err)
		return
	}
	// generate loginData from ENV variables and csrfToken
	// loginData is a url.Values object that contains the login data for the Themis login form
	loginData, err := config.GenerateLoginURLValuesFromENV(csrfToken)
	if err != nil {
		log.Fatal(err)
		return
	}

	httpClient, err = auth.Login(httpClient, baseURL+loginRoute, loginData)
	if err != nil {
		log.Fatal(err)
		return
	}

	// get user data
	name := client.GetFullname(&httpClient, baseURL)
	log.Println(name)
	email := client.GetEmail(&httpClient, baseURL)
	log.Println(email)
	lastLoggedIn := client.GetLastLoggedIn(&httpClient, baseURL)
	log.Println(lastLoggedIn)
	firstLoggedIn := client.GetFirstLoggedIn(&httpClient, baseURL)
	log.Println(firstLoggedIn)

}
