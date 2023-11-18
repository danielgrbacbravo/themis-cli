package main

// system packages

import (
	// local packages
	"log"
	"themis-cli/auth"
	"themis-cli/client"
	"themis-cli/config"
	"themis-cli/parser"
	"themis-cli/tree"
)

const (
	baseURL     = "https://themis.housing.rug.nl"
	loginRoute  = "/log/in"
	userDataUrl = "/user"
	coursesUrl  = "/courses"
)

func main() {

	httpClient := client.InitializeClient()
	// the goquery document represents the parsed HTML document of the login page (baseURL + loginRoute)
	document, err := client.GetLoginPage(httpClient, baseURL, loginRoute)
	if err != nil {
		log.Fatal(err)
		return
	}
	// get csrfToken from login page
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

	// login to Themis and get the authenticated http.Client
	httpClient, err = auth.Login(httpClient, baseURL+loginRoute, loginData)
	if err != nil {
		log.Fatal(err)
		return
	}

	// get user data
	name := client.GetFullName(&httpClient, baseURL)
	log.Default().Println(name)
	email := client.GetEmail(&httpClient, baseURL)
	log.Default().Println(email)
	lastLoggedIn := client.GetLastLoggedIn(&httpClient, baseURL)
	log.Default().Println(lastLoggedIn)
	firstLoggedIn := client.GetFirstLoggedIn(&httpClient, baseURL)
	log.Default().Println(firstLoggedIn)

	courses, err := parser.GetAssignmentsOnPage(&httpClient, baseURL)
	if err != nil {
		log.Fatal(err)
		return
	}
	courseNodes := make([]*tree.AssignmentNode, 0)
	for i, course := range courses {
		courseNodes = append(courseNodes, tree.BuildRootAssignmentNode(course["name"], course["url"]))
		assignments, err := parser.GetAssignmentsOnPage(&httpClient, course["url"])

		if err != nil {
			log.Fatal(err)
			return
		}
		for _, assignment := range assignments {
			if i >= len(courseNodes) {
				log.Println("Index out of range, skipping iteration")
				continue
			}
			log.Default().Println(i)
			currentAssignmentNode := tree.BuildAssignmentNode(courseNodes[i], assignment["name"], assignment["url"])
			courseNodes[i].AppendChild(currentAssignmentNode)
			subAssigments, err := parser.GetAssignmentsOnPage(&httpClient, assignment["url"])

			if err != nil {
				log.Fatal(err)
				return
			}

			for _, subAssignment := range subAssigments {
				currentSubAssignmentNode := tree.BuildAssignmentNode(currentAssignmentNode, subAssignment["name"], subAssignment["url"])
				currentAssignmentNode.AppendChild(currentSubAssignmentNode)
				activity, err := parser.GetAssignmentsOnPage(&httpClient, subAssignment["url"])
				if err != nil {
					log.Fatal(err)
					return
				}

				for _, activity := range activity {
					currentActivityNode := tree.BuildAssignmentNode(currentSubAssignmentNode, activity["name"], activity["url"])
					currentSubAssignmentNode.AppendChild(currentActivityNode)
				}
			}
		}
	}
}
