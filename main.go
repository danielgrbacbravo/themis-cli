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
	// TODO: get login data from config instead of hardcoding
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
}
