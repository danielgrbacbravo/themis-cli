package main

import (
	"flag"
	"os"
	"path/filepath"
	"themis-cli/internal/discovery"
	"themis-cli/internal/themis"

	log "github.com/charmbracelet/log"
)

const defaultBaseURL = "https://themis.housing.rug.nl"

func main() {
	logger := log.New(os.Stderr)
	logger.SetReportTimestamp(false)
	logger.SetReportCaller(false)
	logger.SetLevel(log.DebugLevel)

	baseURL := defaultFromEnv("THEMIS_BASE_URL", defaultBaseURL)
	defaultCookiePath := defaultFromEnv("THEMIS_COOKIE_PATH", filepath.Join(mustUserHomeDir(), ".config", "themis", "cookie.txt"))
	defaultCourseURL := os.Getenv("THEMIS_COURSE_URL")

	flag.StringVar(&baseURL, "base-url", baseURL, "Themis base URL")
	cookiePath := flag.String("cookie-path", defaultCookiePath, "Path to cookie file")
	courseURL := flag.String("course-url", defaultCourseURL, "Course/assignment URL root to crawl")
	depth := flag.Int("depth", 1, "Assignment tree depth to crawl")
	flag.Parse()

	if *courseURL == "" {
		logger.Fatal("missing course URL; set --course-url or THEMIS_COURSE_URL")
		return
	}

	session, err := themis.NewSession(baseURL, *cookiePath)
	if err != nil {
		logger.Fatal(err)
		return
	}

	userData, err := session.GetUserData()
	if err != nil {
		logger.Fatal(err)
		return
	}

	logger.Info(
		"UserData",
		"name", userData.FullName,
		"email", userData.Email,
		"lastLoggedIn", userData.LastLoggedIn,
		"firstLoggedIn", userData.FirstLoggedIn,
	)

	discoveryService := discovery.NewService(baseURL)
	rootNode := discovery.BuildRootAssignmentNode("root", *courseURL, logger)
	_, err = discoveryService.PullAssignmentsAndBuildTree(session.Client, *courseURL, rootNode, *depth, logger)
	if err != nil {
		logger.Fatal(err)
		return
	}
}

func defaultFromEnv(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func mustUserHomeDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	return homeDir
}
