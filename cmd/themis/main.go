package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"themis-cli/internal/discovery"
	"themis-cli/internal/themis"
)

const defaultBaseURL = "https://themis.housing.rug.nl"

type commonFlags struct {
	baseURL           string
	cookieFile        string
	cookieEnv         string
	defaultCookiePath string
	jsonOutput        bool
}

type commandResult struct {
	Status        string `json:"status"`
	BaseURL       string `json:"base_url,omitempty"`
	Tests         []int  `json:"tests"`
	Downloaded    int    `json:"downloaded"`
	Files         any    `json:"files"`
	Assignments   any    `json:"assignments,omitempty"`
	Error         string `json:"error,omitempty"`
	Authenticated bool   `json:"authenticated,omitempty"`
	User          any    `json:"user,omitempty"`
	TestsBaseURL  string `json:"tests_base_url,omitempty"`
	TargetDir     string `json:"target_dir,omitempty"`
}

func main() {
	if len(os.Args) < 2 {
		fail(fmt.Errorf("missing subcommand"), wantsJSON(os.Args[1:]), "")
	}

	switch os.Args[1] {
	case "check":
		runCheck(os.Args[2:])
	case "list":
		runList(os.Args[2:])
	case "fetch":
		runFetch(os.Args[2:])
	case "-h", "--help", "help":
		printUsage()
	default:
		fail(fmt.Errorf("unknown subcommand: %s", os.Args[1]), wantsJSON(os.Args[2:]), "")
	}
}

func runCheck(args []string) {
	jsonRequested := wantsJSON(args)
	fs := newFlagSet("check")
	common := addCommonFlags(fs)
	if err := fs.Parse(args); err != nil {
		fail(err, jsonRequested, "")
	}

	session, err := themis.NewSessionWithAuthConfig(common.baseURL, themis.AuthConfig{
		CookieFile:        common.cookieFile,
		CookieEnv:         common.cookieEnv,
		DefaultCookiePath: common.defaultCookiePath,
	})
	if err != nil {
		fail(err, common.jsonOutput, common.baseURL)
	}

	if err := session.CheckBaseURLAccess(); err != nil {
		fail(err, common.jsonOutput, session.BaseURL)
	}

	userData, err := session.ValidateAuthentication()
	if err != nil {
		fail(err, common.jsonOutput, session.BaseURL)
	}

	if common.jsonOutput {
		writeJSON(commandResult{
			Status:        "ok",
			BaseURL:       session.BaseURL,
			Tests:         []int{},
			Downloaded:    0,
			Files:         []any{},
			Authenticated: true,
			User:          userData,
		})
		return
	}

	fmt.Printf("OK: base URL %s reachable and authenticated as %s (%s)\n", session.BaseURL, userData.FullName, userData.Email)
}

func runList(args []string) {
	jsonRequested := wantsJSON(args)
	fs := newFlagSet("list")
	common := addCommonFlags(fs)
	testsURL := fs.String("tests-url", "", "Tests directory URL or specific test file URL")
	discover := fs.Bool("discover", false, "Recursively discover assignment/exercise URLs")
	rootURL := fs.String("root-url", "", "Root URL for recursive discovery (used with --discover)")
	discoverDepth := fs.Int("discover-depth", 8, "Maximum depth for recursive discovery")
	start := fs.Int("start", 1, "First test index to probe")
	max := fs.Int("max", 200, "Maximum number of indices to probe")
	maxMisses := fs.Int("max-misses", 5, "Stop after this many consecutive missing indices")
	if err := fs.Parse(args); err != nil {
		fail(err, jsonRequested, "")
	}

	if !*discover && *testsURL == "" {
		fail(fmt.Errorf("missing required --tests-url"), common.jsonOutput, "")
	}
	if *discover && *rootURL == "" {
		fail(fmt.Errorf("missing required --root-url when --discover is set"), common.jsonOutput, "")
	}

	session, err := themis.NewSessionWithAuthConfig(common.baseURL, themis.AuthConfig{
		CookieFile:        common.cookieFile,
		CookieEnv:         common.cookieEnv,
		DefaultCookiePath: common.defaultCookiePath,
	})
	if err != nil {
		fail(err, common.jsonOutput, common.baseURL)
	}

	if _, err := session.ValidateAuthentication(); err != nil {
		fail(err, common.jsonOutput, session.BaseURL)
	}

	discoveryService := discovery.NewService(session.BaseURL)
	if *discover {
		normalizedRootURL, entries, err := discoveryService.DiscoverAssignments(session.Client, *rootURL, *discoverDepth)
		if err != nil {
			fail(err, common.jsonOutput, session.BaseURL)
		}

		if common.jsonOutput {
			writeJSON(commandResult{
				Status:      "ok",
				BaseURL:     session.BaseURL,
				Tests:       []int{},
				Downloaded:  0,
				Files:       []any{},
				Assignments: entries,
				TargetDir:   "",
			})
			return
		}

		fmt.Printf("Discovered %d assignment URLs from %s\n", len(entries), normalizedRootURL)
		for _, entry := range entries {
			indent := strings.Repeat("  ", entry.Depth-1)
			fmt.Printf("%s- %s: %s\n", indent, entry.Name, entry.URL)
		}
		return
	}

	baseTestsURL, testCases, err := discovery.ListTestCasesWithOptions(session.Client, *testsURL, discovery.ListOptions{
		Start:     *start,
		Max:       *max,
		MaxMisses: *maxMisses,
	})
	if err != nil {
		fail(err, common.jsonOutput, session.BaseURL)
	}
	testNumbers := discovery.ListTestNumbers(testCases)

	if common.jsonOutput {
		writeJSON(commandResult{
			Status:       "ok",
			BaseURL:      session.BaseURL,
			TestsBaseURL: baseTestsURL,
			Tests:        testNumbers,
			Downloaded:   0,
			Files:        []any{},
			Assignments:  []any{},
		})
		return
	}

	fmt.Printf("Found %d test cases at %s\n", len(testNumbers), baseTestsURL)
	for _, index := range testNumbers {
		fmt.Printf("%d\n", index)
	}
}

func runFetch(args []string) {
	jsonRequested := wantsJSON(args)
	fs := newFlagSet("fetch")
	common := addCommonFlags(fs)
	testsURL := fs.String("tests-url", "", "Tests directory URL or specific test file URL")
	outDir := fs.String("out", "", "Directory to write downloaded test files (default: ./tests)")
	targetDir := fs.String("target-dir", "", "Deprecated alias for --out")
	if err := fs.Parse(args); err != nil {
		fail(err, jsonRequested, "")
	}

	if *testsURL == "" {
		fail(fmt.Errorf("missing required --tests-url"), common.jsonOutput, "")
	}

	session, err := themis.NewSessionWithAuthConfig(common.baseURL, themis.AuthConfig{
		CookieFile:        common.cookieFile,
		CookieEnv:         common.cookieEnv,
		DefaultCookiePath: common.defaultCookiePath,
	})
	if err != nil {
		fail(err, common.jsonOutput, common.baseURL)
	}

	if _, err := session.ValidateAuthentication(); err != nil {
		fail(err, common.jsonOutput, session.BaseURL)
	}

	resolvedOutDir, err := resolveOutputDir(*outDir, *targetDir)
	if err != nil {
		fail(err, common.jsonOutput, session.BaseURL)
	}

	baseTestsURL, downloaded, err := discovery.FetchTestCases(session.Client, *testsURL, resolvedOutDir)
	if err != nil {
		fail(err, common.jsonOutput, session.BaseURL)
	}

	if common.jsonOutput {
		writeJSON(commandResult{
			Status:       "ok",
			BaseURL:      session.BaseURL,
			TestsBaseURL: baseTestsURL,
			Tests:        []int{},
			Downloaded:   len(downloaded),
			Files:        downloaded,
			TargetDir:    resolvedOutDir,
		})
		return
	}

	fmt.Printf("Downloaded %d test cases into %s\n", len(downloaded), resolvedOutDir)
}

func addCommonFlags(fs *flag.FlagSet) *commonFlags {
	common := &commonFlags{}

	defaultCookiePath := filepath.Join(mustUserHomeDir(), ".config", "themis", "cookie.txt")
	defaultCookieFile := defaultFromEnv("THEMIS_COOKIE_FILE", defaultFromEnv("THEMIS_COOKIE_PATH", ""))
	defaultCookieEnv := defaultFromEnv("THEMIS_COOKIE_ENV", "THEMIS_COOKIE")
	defaultBase := defaultFromEnv("THEMIS_BASE_URL", defaultBaseURL)

	fs.StringVar(&common.baseURL, "base-url", defaultBase, "Themis base URL")
	fs.StringVar(&common.cookieFile, "cookie-file", defaultCookieFile, "Path to cookie file")
	fs.StringVar(&common.cookieEnv, "cookie-env", defaultCookieEnv, "Name of env var containing cookie string")
	common.defaultCookiePath = defaultCookiePath
	fs.BoolVar(&common.jsonOutput, "json", false, "Output JSON")

	return common
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

func printUsage() {
	fmt.Println("Usage:")
	fmt.Println("  themis <subcommand> [flags]")
	fmt.Println()
	fmt.Println("Subcommands:")
	fmt.Println("  check  Validate authentication and base URL access")
	fmt.Println("  list   List available test case indices")
	fmt.Println("  fetch  Download available test cases")
	fmt.Println()
	fmt.Println("Common flags (all subcommands):")
	fmt.Println("  --base-url <url>")
	fmt.Println("  --cookie-file <path>")
	fmt.Println("  --cookie-env <env-var-name>")
	fmt.Println("  --json")
	fmt.Println()
	fmt.Println("Subcommand flags:")
	fmt.Println("  list  --tests-url <url> [--start <n>] [--max <n>] [--max-misses <n>]")
	fmt.Println("  list  --discover --root-url <url> [--discover-depth <n>]")
	fmt.Println("  fetch --tests-url <url> [--out <dir>]")
}

func fail(err error, asJSON bool, baseURL string) {
	if asJSON {
		writeJSON(commandResult{
			Status:      "error",
			BaseURL:     baseURL,
			Tests:       []int{},
			Downloaded:  0,
			Files:       []any{},
			Assignments: []any{},
			Error:       err.Error(),
		})
	} else {
		if err == flag.ErrHelp {
			printUsage()
		} else {
			fmt.Fprintln(os.Stderr, "Error:", err)
		}
	}
	os.Exit(1)
}

func writeJSON(v any) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(v); err != nil {
		fmt.Fprintln(os.Stderr, "Error writing JSON:", err)
		os.Exit(1)
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
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
	return homeDir
}

func wantsJSON(args []string) bool {
	for _, arg := range args {
		if arg == "--json" || arg == "-json" {
			return true
		}
		if strings.HasPrefix(arg, "--json=") || strings.HasPrefix(arg, "-json=") {
			return true
		}
	}
	return false
}

func resolveOutputDir(outFlag string, targetAlias string) (string, error) {
	outFlag = strings.TrimSpace(outFlag)
	targetAlias = strings.TrimSpace(targetAlias)

	if outFlag != "" && targetAlias != "" && outFlag != targetAlias {
		return "", fmt.Errorf("conflicting output flags: --out=%q and --target-dir=%q", outFlag, targetAlias)
	}

	selected := outFlag
	if selected == "" {
		selected = targetAlias
	}
	if selected == "" {
		selected = filepath.Join(".", "tests")
	}

	resolved, err := filepath.Abs(selected)
	if err != nil {
		return "", fmt.Errorf("failed to resolve output path %q: %w", selected, err)
	}
	return resolved, nil
}
