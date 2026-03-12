package discovery

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type TestCase struct {
	Index  int    `json:"index"`
	InURL  string `json:"in_url"`
	OutURL string `json:"out_url"`
}

type DownloadedTestCase struct {
	Index   int    `json:"index"`
	InPath  string `json:"in_path"`
	OutPath string `json:"out_path"`
}

type ListOptions struct {
	Start     int
	Max       int
	MaxMisses int
}

func defaultListOptions() ListOptions {
	return ListOptions{
		Start:     1,
		Max:       200,
		MaxMisses: 5,
	}
}

func ListTestCases(client *http.Client, testsURL string) (string, []TestCase, error) {
	return ListTestCasesWithOptions(client, testsURL, defaultListOptions())
}

func ListTestCasesWithOptions(client *http.Client, testsURL string, options ListOptions) (string, []TestCase, error) {
	if options.Start < 1 {
		return "", nil, fmt.Errorf("--start must be >= 1")
	}
	if options.Max < 1 {
		return "", nil, fmt.Errorf("--max must be >= 1")
	}
	if options.MaxMisses < 1 {
		return "", nil, fmt.Errorf("--max-misses must be >= 1")
	}

	baseTestsURL, err := NormalizeTestsBaseURL(testsURL)
	if err != nil {
		return "", nil, err
	}

	testCases := make([]TestCase, 0)
	current := options.Start
	probed := 0
	consecutiveMisses := 0

	for probed < options.Max && consecutiveMisses < options.MaxMisses {
		inURL := fmt.Sprintf("%s/%d.in", baseTestsURL, current)
		outURL := fmt.Sprintf("%s/%d.out", baseTestsURL, current)

		inExists, err := rawFileExists(client, inURL)
		if err != nil {
			return "", nil, err
		}
		outExists, err := rawFileExists(client, outURL)
		if err != nil {
			return "", nil, err
		}

		if inExists && outExists {
			testCases = append(testCases, TestCase{
				Index:  current,
				InURL:  inURL,
				OutURL: outURL,
			})
			consecutiveMisses = 0
		} else {
			consecutiveMisses++
		}

		current++
		probed++
	}

	return baseTestsURL, testCases, nil
}

func FetchTestCases(client *http.Client, testsURL string, targetDir string) (string, []DownloadedTestCase, error) {
	baseTestsURL, testCases, err := ListTestCases(client, testsURL)
	if err != nil {
		return "", nil, err
	}

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return "", nil, fmt.Errorf("error creating target dir: %w", err)
	}

	downloaded := make([]DownloadedTestCase, 0, len(testCases))
	for _, tc := range testCases {
		inRawURL := tc.InURL + "?raw=true"
		outRawURL := tc.OutURL + "?raw=true"

		inBytes, err := downloadRawFile(client, inRawURL)
		if err != nil {
			return "", nil, fmt.Errorf("failed downloading %s: %w", tc.InURL, err)
		}
		outBytes, err := downloadRawFile(client, outRawURL)
		if err != nil {
			return "", nil, fmt.Errorf("failed downloading %s: %w", tc.OutURL, err)
		}

		inPath := filepath.Join(targetDir, fmt.Sprintf("%d.in", tc.Index))
		outPath := filepath.Join(targetDir, fmt.Sprintf("%d.out", tc.Index))
		if err := os.WriteFile(inPath, inBytes, 0o644); err != nil {
			return "", nil, fmt.Errorf("failed writing %s: %w", inPath, err)
		}
		if err := os.WriteFile(outPath, outBytes, 0o644); err != nil {
			return "", nil, fmt.Errorf("failed writing %s: %w", outPath, err)
		}

		downloaded = append(downloaded, DownloadedTestCase{
			Index:   tc.Index,
			InPath:  inPath,
			OutPath: outPath,
		})
	}

	return baseTestsURL, downloaded, nil
}

func ListTestNumbers(testCases []TestCase) []int {
	numbers := make([]int, 0, len(testCases))
	for _, tc := range testCases {
		numbers = append(numbers, tc.Index)
	}
	return numbers
}

func rawFileExists(client *http.Client, fileURL string) (bool, error) {
	body, statusCode, err := getRawFile(client, fileURL+"?raw=true")
	if err != nil {
		return false, err
	}

	if statusCode == http.StatusNotFound || statusCode == http.StatusForbidden || statusCode == http.StatusUnauthorized {
		return false, nil
	}
	if statusCode >= http.StatusBadRequest {
		return false, fmt.Errorf("request failed with status %d for %s", statusCode, fileURL)
	}

	if isHTMLDocument(body) {
		return false, nil
	}

	return true, nil
}

func downloadRawFile(client *http.Client, rawURL string) ([]byte, error) {
	body, statusCode, err := getRawFile(client, rawURL)
	if err != nil {
		return nil, err
	}
	if statusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("request failed with status %d", statusCode)
	}
	if isHTMLDocument(body) {
		return nil, fmt.Errorf("received HTML instead of raw test file")
	}
	return body, nil
}

func getRawFile(client *http.Client, rawURL string) ([]byte, int, error) {
	resp, err := client.Get(rawURL)
	if err != nil {
		return nil, 0, fmt.Errorf("error fetching %s: %w", rawURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("error reading %s: %w", rawURL, err)
	}
	return body, resp.StatusCode, nil
}

func isHTMLDocument(body []byte) bool {
	trimmed := strings.TrimSpace(strings.ToLower(string(body)))
	return strings.HasPrefix(trimmed, "<!doctype html") || strings.HasPrefix(trimmed, "<html")
}
