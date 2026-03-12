package main

// system packages

import (
	// local packages
	"net/url"
	"os"
	"path/filepath"
	"themis-cli/auth"
	"themis-cli/client"
	"themis-cli/tree"

	log "github.com/charmbracelet/log"
)

const (
	baseURL = "https://themis.housing.rug.nl"
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

	homeDir, err := os.UserHomeDir()
	if err != nil {
		logger.Fatal(err)
		return
	}
	cookiePath := filepath.Join(homeDir, ".config", "themis", "cookie.txt")

	cookies, err := auth.LoadCookiesFromFile(cookiePath)
	if err != nil {
		logger.Fatal(err)
		return
	} else {
		logger.Debug("cookies loaded 🍪 :", "count", len(cookies))
	}

	parsedBaseURL, err := url.Parse(baseURL)
	if err != nil {
		logger.Fatal(err)
		return
	} else {
		httpClient.Jar.SetCookies(parsedBaseURL, cookies)
		logger.Debug("httpClient authenticated via cookie file 😎 :", "cookiePath", cookiePath)
	}
	// get user data

	name := client.GetFullName(&httpClient, baseURL)
	email := client.GetEmail(&httpClient, baseURL)
	lastLoggedIn := client.GetLastLoggedIn(&httpClient, baseURL)
	firstLoggedIn := client.GetFirstLoggedIn(&httpClient, baseURL)

	logger.Info("UserData 🥸 :", "name", name, "email", email, "lastLoggedIn", lastLoggedIn, "firstLoggedIn", firstLoggedIn)
	URL := "https://themis.housing.rug.nl/course/2025-2026/os/practicals/"
	rootNode := tree.BuildRootAssignmentNode("root", URL, logger)
	rootNode, err = tree.PullAssignmentsFromThemisAndBuildTree(&httpClient, URL, rootNode, 1, logger)
	if err != nil {
		log.Fatal(err)
		return
	}

}
