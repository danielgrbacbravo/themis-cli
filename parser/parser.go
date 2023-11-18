package parser

import (
	"fmt"
	"net/http"
	"strings"

	// goquery
	"github.com/PuerkitoBio/goquery"
)

const (
	baseURL = "https://themis.housing.rug.nl"
)

func extractCourseData(doc *goquery.Document) []map[string]string {
	var courses []map[string]string

	doc.Find("ul.nav-list li").Each(func(i int, s *goquery.Selection) {
		course := make(map[string]string)
		anchor := s.Find("span.ass-link a")
		courseName := anchor.Text()

		href, exists := anchor.Attr("href")
		if exists {
			course["name"] = strings.TrimSpace(courseName)
			course["url"] = baseURL + href
			courses = append(courses, course)
		}
	})

	return courses
}

func GetAssignmentsOnPage(client *http.Client, URL string) ([]map[string]string, error) {
	var assignments []map[string]string

	resp, err := client.Get(URL)
	if err != nil {
		return nil, fmt.Errorf("error fetching page: %v", err)
	}
	defer resp.Body.Close()

	// Parse HTML
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error parsing assignments page: %v", err)
	}
	// Find the assignments in the HTML
	doc.Find("div.subsec.round.shade.ass-children ul.round li").Each(func(i int, s *goquery.Selection) {
		assignment := make(map[string]string)
		anchor := s.Find("span.ass-link a")
		assignmentName := anchor.Text()

		href, exists := anchor.Attr("href")
		if exists {
			assignment["name"] = strings.TrimSpace(assignmentName)
			assignment["url"] = baseURL + href
			assignments = append(assignments, assignment)
		}
	})
	return assignments, nil
}

func doesContainAssignmentsOnPage(client *http.Client, URL string) (isLeafNode bool, err error) {
	exists := true
	resp, err := client.Get(URL)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return false, err
	}
	doc.Find("div.subsec.round.shade.ass-children ul.round li").Each(func(i int, s *goquery.Selection) {
		anchor := s.Find("span.ass-link a")
		_, exists = anchor.Attr("href")
	})
	return exists, nil
}
