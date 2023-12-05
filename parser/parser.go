package parser

import (
	"fmt"
	"net/http"
	"strings"
	"themis-cli/models"
	"time"

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

func GetDatesFromAssignmentPage(client *http.Client, AssignmentPageURL string) (models.AssignmentDate, error) {
	// Initialize an empty model
	dates := models.AssignmentDate{}

	// Send an HTTP GET request to get the assignment page
	resp, err := client.Get(AssignmentPageURL)
	if err != nil {
		return dates, fmt.Errorf("error fetching assignment page: %v", err)
	}
	defer resp.Body.Close()

	// Check HTTP response status
	if resp.StatusCode != http.StatusOK {
		return dates, fmt.Errorf("receiving non-OK response status %s", resp.Status)
	}

	// Create a new goquery document from the HTTP response body
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return dates, fmt.Errorf("error reading document: %v", err)
	}

	// Parse the start date .cfg-line .cfg-key .tip[data-title]
	startDate, err := findDateInTooltip(doc, ".cfg-line:contains('Start:') .tip[data-title]")
	if err != nil {
		return dates, err
	}

	// Parse the deadline date .cfg-line .cfg-key .tip[data-title]
	dueDate, err := findDateInTooltip(doc, ".cfg-line:contains('Deadline:') .tip[data-title]")
	if err != nil {
		return dates, err
	}

	// Parse the end date .cfg-line .cfg-key .tip[data-title]
	endDate, err := findDateInTooltip(doc, ".cfg-line:contains('End:') .tip[data-title]")
	if err != nil {
		return dates, err
	}

	dates = models.AssignmentDate{
		StartDate: startDate,
		DueDate:   dueDate,
		EndDate:   endDate,
	}

	return dates, nil
}

// findDateInTooltip finds the date within a tooltip element's data-title attribute
func findDateInTooltip(doc *goquery.Document, selector string) (time.Time, error) {
	var parsedDate time.Time
	timeLayout := "Mon Jan 02 2006 15:04:05 GMT-0700"

	tooltip := doc.Find(selector).First()
	dateString, exists := tooltip.Attr("data-title")
	if !exists {
		return parsedDate, fmt.Errorf("no tooltip with date found")
	}

	// Extract just the date part from the complex string
	dateParts := strings.Split(dateString, " ")
	dateString = strings.Join(dateParts[:6], " ")

	// Parse the dateString into Go's time.Time
	parsedDate, err := time.Parse(timeLayout, dateString)
	if err != nil {
		return parsedDate, fmt.Errorf("error parsing date: %v", err)
	}

	return parsedDate, nil
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
	// TODO: read the dates from the inside the scanned assignement

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
