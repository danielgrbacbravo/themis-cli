package main

// system packages

import (
	// local packages
	"os"
	"themis-cli/auth"
	"themis-cli/client"
	"themis-cli/config"
	"themis-cli/tree"

	log "github.com/charmbracelet/log"
)

const (
	baseURL     = "https://themis.housing.rug.nl"
	loginRoute  = "/log/in"
	userDataUrl = "/user"
	coursesUrl  = "/courses"
)

func main() {
	logger := log.New(os.Stderr)
	logger.SetReportTimestamp(false)
	logger.SetReportCaller(false)
	logger.SetLevel(log.DebugLevel)

	httpClient, err := client.InitializeClient()

	if err != nil {
		logger.Fatal(err)
		return
	} else {
		logger.Debug("httpClient initialized 🔥 :", "objectinfo", httpClient)
	}

	// the goquery document represents the parsed HTML document of the login page (baseURL + loginRoute)
	document, err := client.GetLoginPage(httpClient, baseURL, loginRoute)
	if err != nil {
		logger.Fatal(err)
		return
	} else {
		logger.Debug("document initialized 🔥 :", "objectinfo", document)
	}

	// get csrfToken from login page
	csrfToken, err := auth.GetCsrfToken(document)
	if err != nil {
		logger.Fatal(err)
		return
	} else {
		logger.Debug("csrfToken 🍪 :", "objectinfo", csrfToken)
	}

	// generate loginData from ENV variables and csrfToken
	// loginData is a url.Values object that contains the login data for the Themis login form
	loginData, err := config.GenerateLoginURLValuesFromENV(csrfToken)
	if err != nil {
		logger.Fatal(err)
		return
	} else {
		logger.Debug("LoginData 🔒 :", "user", loginData.Get("user"), "password", loginData.Get("password"))
	}

	// login to Themis and get the authenticated http.Client
	httpClient, err = auth.Login(httpClient, baseURL+loginRoute, loginData)
	if err != nil {
		logger.Fatal(err)
		return
	} else {
		logger.Debug("httpClient authenticated 😎 :", "objectinfo", httpClient)
	}
	// get user data

	name := client.GetFullName(&httpClient, baseURL)
	email := client.GetEmail(&httpClient, baseURL)
	lastLoggedIn := client.GetLastLoggedIn(&httpClient, baseURL)
	firstLoggedIn := client.GetFirstLoggedIn(&httpClient, baseURL)

	logger.Info("UserData 🥸 :", "name", name, "email", email, "lastLoggedIn", lastLoggedIn, "firstLoggedIn", firstLoggedIn)

	URL := "https://themis.housing.rug.nl/course/2023-2024/progfun/"
	rootNode := tree.BuildRootAssignmentNode("root", URL, logger)
	rootNode, err = tree.PullAssignmentsFromThemisAndBuildTree(&httpClient, URL, rootNode, 0, logger)
	if err != nil {
		log.Fatal(err)
		return
	}
}
