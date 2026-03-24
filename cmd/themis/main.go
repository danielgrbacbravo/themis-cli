package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"themis-cli/internal/discovery"
	"themis-cli/internal/projectlink"
	"themis-cli/internal/state"
	"themis-cli/internal/themis"
	tuiapp "themis-cli/internal/tui/app"
	"time"
)

const defaultBaseURL = "https://themis.housing.rug.nl"

type commonFlags struct {
	baseURL     string
	sessionFile string
	jsonOutput  bool
}

type commandResult struct {
	Status        string `json:"status"`
	BaseURL       string `json:"base_url,omitempty"`
	Mode          string `json:"mode,omitempty"`
	RootURL       string `json:"root_url,omitempty"`
	Refreshed     bool   `json:"refreshed,omitempty"`
	RefreshScope  string `json:"refresh_scope,omitempty"`
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
	case "project":
		runProject(os.Args[2:])
	case "tui":
		runTUI(os.Args[2:])
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
		SessionFile: common.sessionFile,
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
	rootURL := fs.String("root-url", "", "Root URL for recursive discovery (used with --discover). Optional when project is linked.")
	discoverDepth := fs.Int("discover-depth", 8, "Maximum depth for recursive discovery")
	refreshURL := fs.String("refresh-url", "", "Refresh URL before reading from state (used with --discover)")
	refreshDepth := fs.Int("refresh-depth", 1, "Depth used with --refresh-url (used with --discover)")
	fullRefresh := fs.Bool("full-refresh", false, "Refresh catalog root before reading from state (used with --discover)")
	fromStateOnly := fs.Bool("from-state-only", false, "Read discovery results only from local state; skip network refresh")
	start := fs.Int("start", 1, "First test index to probe")
	max := fs.Int("max", 200, "Maximum number of indices to probe")
	maxMisses := fs.Int("max-misses", 5, "Stop after this many consecutive missing indices")
	if err := fs.Parse(args); err != nil {
		fail(err, jsonRequested, "")
	}

	if !*discover && *testsURL == "" {
		fail(fmt.Errorf("missing required --tests-url"), common.jsonOutput, "")
	}
	if *discover && *fullRefresh && *fromStateOnly {
		fail(fmt.Errorf("--full-refresh and --from-state-only cannot be combined"), common.jsonOutput, "")
	}
	if *discover && strings.TrimSpace(*refreshURL) != "" && *fromStateOnly {
		fail(fmt.Errorf("--refresh-url and --from-state-only cannot be combined"), common.jsonOutput, "")
	}
	if *discover && *refreshDepth < 0 {
		fail(fmt.Errorf("--refresh-depth must be >= 0"), common.jsonOutput, "")
	}

	if *discover {
		result, entries, err := runDiscoverStateFirst(discoverOptions{
			common:        *common,
			rootURL:       *rootURL,
			discoverDepth: *discoverDepth,
			refreshURL:    *refreshURL,
			refreshDepth:  *refreshDepth,
			fullRefresh:   *fullRefresh,
			fromStateOnly: *fromStateOnly,
		})
		if err != nil {
			fail(err, common.jsonOutput, common.baseURL)
		}
		result.Assignments = entries

		if common.jsonOutput {
			writeJSON(result)
			return
		}

		fmt.Printf("Discovered %d assignment URLs from %s (%s mode)\n", len(entries), result.RootURL, result.Mode)
		for _, entry := range entries {
			indent := strings.Repeat("  ", entry.Depth-1)
			fmt.Printf("%s- %s: %s\n", indent, entry.Name, entry.URL)
		}
		if !result.Refreshed {
			fmt.Println("Returned from cached state (no refresh).")
		}
		return
	}

	session, err := themis.NewSessionWithAuthConfig(common.baseURL, themis.AuthConfig{
		SessionFile: common.sessionFile,
	})
	if err != nil {
		fail(err, common.jsonOutput, common.baseURL)
	}

	if _, err := session.ValidateAuthentication(); err != nil {
		fail(err, common.jsonOutput, session.BaseURL)
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
		SessionFile: common.sessionFile,
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

type discoverOptions struct {
	common        commonFlags
	rootURL       string
	discoverDepth int
	refreshURL    string
	refreshDepth  int
	fullRefresh   bool
	fromStateOnly bool
}

func runDiscoverStateFirst(opts discoverOptions) (commandResult, []discovery.AssignmentEntry, error) {
	statePath, err := state.DefaultStatePath()
	if err != nil {
		return commandResult{}, nil, err
	}
	st, err := state.Load(statePath)
	if err != nil {
		return commandResult{}, nil, err
	}

	baseURL, err := themis.NormalizeBaseURL(opts.common.baseURL)
	if err != nil {
		return commandResult{}, nil, err
	}
	if st.BaseURL == "" {
		st.BaseURL = baseURL
	}
	if st.CatalogRootURL == "" {
		st.CatalogRootURL = strings.TrimRight(baseURL, "/") + "/course"
	}

	effectiveRootURL, err := resolveDiscoverRootURL(strings.TrimSpace(opts.rootURL), st)
	if err != nil {
		return commandResult{}, nil, err
	}

	refreshed := false
	refreshScope := "none"
	if !opts.fromStateOnly {
		needBootstrap := false
		rootID, canonicalRoot, rootErr := state.NodeIDFromURL(effectiveRootURL)
		if rootErr == nil {
			if _, ok := st.Nodes[rootID]; !ok {
				needBootstrap = true
			}
			effectiveRootURL = canonicalRoot
		}

		if opts.fullRefresh || strings.TrimSpace(opts.refreshURL) != "" || needBootstrap {
			session, err := themis.NewSessionWithAuthConfig(baseURL, themis.AuthConfig{
				SessionFile: opts.common.sessionFile,
			})
			if err != nil {
				return commandResult{}, nil, err
			}
			if _, err := session.ValidateAuthentication(); err != nil {
				return commandResult{}, nil, err
			}

			service := discovery.NewService(session.BaseURL)
			switch {
			case opts.fullRefresh:
				if _, err := service.RefreshCatalog(session.Client, &st, opts.discoverDepth); err != nil {
					return commandResult{}, nil, err
				}
				refreshed = true
				refreshScope = "catalog"
			case strings.TrimSpace(opts.refreshURL) != "":
				if _, err := service.RefreshNode(session.Client, &st, opts.refreshURL, opts.refreshDepth); err != nil {
					return commandResult{}, nil, err
				}
				refreshed = true
				refreshScope = "subtree"
			case needBootstrap:
				if _, err := service.RefreshNode(session.Client, &st, effectiveRootURL, opts.discoverDepth); err != nil {
					return commandResult{}, nil, err
				}
				refreshed = true
				refreshScope = "subtree"
			}
		}
	}

	rootID, canonicalRootURL, err := state.NodeIDFromURL(effectiveRootURL)
	if err != nil {
		return commandResult{}, nil, err
	}
	effectiveRootURL = canonicalRootURL

	if _, ok := st.Nodes[rootID]; !ok {
		return commandResult{}, nil, fmt.Errorf("root %s is not in local state; run discover with --full-refresh or --refresh-url", effectiveRootURL)
	}

	if upsertRootRef(&st, rootID, effectiveRootURL) {
		refreshed = true
		if refreshScope == "none" {
			refreshScope = "metadata"
		}
	}

	if refreshed {
		if err := state.SaveAtomic(statePath, st, true); err != nil {
			return commandResult{}, nil, err
		}
	}

	entries := collectAssignmentsFromState(st, rootID, opts.discoverDepth)
	return commandResult{
		Status:       "ok",
		BaseURL:      baseURL,
		Mode:         "state-first",
		RootURL:      effectiveRootURL,
		Refreshed:    refreshed,
		RefreshScope: refreshScope,
		Tests:        []int{},
		Downloaded:   0,
		Files:        []any{},
		TargetDir:    "",
	}, entries, nil
}

func resolveDiscoverRootURL(rootURLFlag string, st state.State) (string, error) {
	if rootURLFlag != "" {
		return state.CanonicalizeURL(rootURLFlag)
	}

	if cfg, _, err := projectlink.ResolveByCWD("."); err == nil {
		return state.CanonicalizeURL(cfg.LinkedRootURL)
	} else if !errors.Is(err, projectlink.ErrNotLinked) {
		return "", err
	}

	if len(st.Roots) == 1 {
		return state.CanonicalizeURL(st.Roots[0].CanonicalURL)
	}

	return "", fmt.Errorf("missing --root-url and no linked project found; run `themis project link --root-url <url>`")
}

func upsertRootRef(st *state.State, rootID string, canonicalRootURL string) bool {
	now := time.Now().UTC()
	for i := range st.Roots {
		if st.Roots[i].NodeID == rootID {
			changed := st.Roots[i].CanonicalURL != canonicalRootURL
			st.Roots[i].CanonicalURL = canonicalRootURL
			st.Roots[i].UpdatedAt = now
			return changed
		}
	}

	title := ""
	kind := ""
	if rootNode, ok := st.Nodes[rootID]; ok {
		title = rootNode.Title
		kind = rootNode.Kind
	}
	st.Roots = append(st.Roots, state.RootRef{
		NodeID:       rootID,
		CanonicalURL: canonicalRootURL,
		Title:        title,
		Kind:         kind,
		UpdatedAt:    now,
	})
	return true
}

func collectAssignmentsFromState(st state.State, rootID string, maxDepth int) []discovery.AssignmentEntry {
	if maxDepth < 0 {
		maxDepth = 0
	}

	entries := make([]discovery.AssignmentEntry, 0)
	visited := map[string]bool{}

	var walk func(nodeID string, depth int)
	walk = func(nodeID string, depth int) {
		if depth >= maxDepth {
			return
		}
		node, ok := st.Nodes[nodeID]
		if !ok {
			return
		}

		children := append([]string{}, node.ChildIDs...)
		for _, childID := range children {
			child, ok := st.Nodes[childID]
			if !ok {
				continue
			}
			name := strings.TrimSpace(child.Title)
			if name == "" {
				name = child.CanonicalURL
			}
			entries = append(entries, discovery.AssignmentEntry{
				Name:      name,
				URL:       child.CanonicalURL,
				Depth:     depth + 1,
				ParentURL: node.CanonicalURL,
			})

			if !visited[childID] {
				visited[childID] = true
				walk(childID, depth+1)
			}
		}
	}

	visited[rootID] = true
	walk(rootID, 0)

	return entries
}

func runProject(args []string) {
	if len(args) == 0 {
		fail(fmt.Errorf("missing project subcommand"), wantsJSON(args), "")
	}

	switch args[0] {
	case "link":
		runProjectLink(args[1:])
	default:
		fail(fmt.Errorf("unknown project subcommand: %s", args[0]), wantsJSON(args[1:]), "")
	}
}

func runProjectLink(args []string) {
	jsonRequested := wantsJSON(args)
	fs := newFlagSet("project link")
	common := addCommonFlags(fs)
	rootURL := fs.String("root-url", "", "Course root URL to link to current repository")
	lastOpenNodeID := fs.String("last-open-node-id", "", "Optional last opened node id")
	defaultRefreshDepth := fs.Int("default-refresh-depth", 1, "Default refresh depth for linked project")
	autoRefreshOnOpen := fs.Bool("auto-refresh-on-open", false, "Auto refresh when opening TUI in this project")
	showStaleWarning := fs.Int("show-stale-warning-after-minutes", 120, "Minutes before stale warning appears")
	if err := fs.Parse(args); err != nil {
		fail(err, jsonRequested, "")
	}

	if strings.TrimSpace(*rootURL) == "" {
		fail(fmt.Errorf("missing required --root-url"), common.jsonOutput, "")
	}
	if *defaultRefreshDepth < 1 {
		fail(fmt.Errorf("--default-refresh-depth must be >= 1"), common.jsonOutput, "")
	}
	if *showStaleWarning < 1 {
		fail(fmt.Errorf("--show-stale-warning-after-minutes must be >= 1"), common.jsonOutput, "")
	}

	repoRoot, err := findRepoRoot(".")
	if err != nil {
		fail(err, common.jsonOutput, "")
	}

	canonicalRootURL, err := state.CanonicalizeURL(*rootURL)
	if err != nil {
		fail(err, common.jsonOutput, common.baseURL)
	}
	rootNodeID := state.NodeIDFromCanonicalURL(canonicalRootURL)

	cfg := projectlink.Config{
		BaseURL:          common.baseURL,
		LinkedRootURL:    canonicalRootURL,
		LinkedRootNodeID: rootNodeID,
		LastOpenNodeID:   strings.TrimSpace(*lastOpenNodeID),
		Preferences: projectlink.Preferences{
			DefaultRefreshDepth:          *defaultRefreshDepth,
			AutoRefreshOnOpen:            *autoRefreshOnOpen,
			ShowStaleWarningAfterMinutes: *showStaleWarning,
		},
	}

	cfgPath := projectlink.ConfigPathFromRepoRoot(repoRoot)
	if err := projectlink.Save(cfgPath, cfg); err != nil {
		fail(err, common.jsonOutput, common.baseURL)
	}

	if common.jsonOutput {
		writeJSON(map[string]any{
			"status":                "ok",
			"base_url":              common.baseURL,
			"repo_root":             repoRoot,
			"project_config_path":   cfgPath,
			"linked_root_url":       canonicalRootURL,
			"linked_root_node_id":   rootNodeID,
			"default_refresh_depth": *defaultRefreshDepth,
		})
		return
	}

	fmt.Printf("Linked %s to %s\n", repoRoot, canonicalRootURL)
	fmt.Printf("Saved project config at %s\n", cfgPath)
}

func runTUI(args []string) {
	jsonRequested := wantsJSON(args)
	fs := newFlagSet("tui")
	common := addCommonFlags(fs)
	rootURL := fs.String("root-url", "", "Optional root URL to focus in TUI")
	if err := fs.Parse(args); err != nil {
		fail(err, jsonRequested, "")
	}
	if jsonRequested {
		fail(fmt.Errorf("--json is not supported for interactive tui"), false, "")
	}

	statePath, err := state.DefaultStatePath()
	if err != nil {
		fail(err, false, "")
	}
	st, err := state.Load(statePath)
	if err != nil {
		fail(err, false, "")
	}

	rootNodeID := ""
	if strings.TrimSpace(*rootURL) != "" {
		id, _, err := state.NodeIDFromURL(strings.TrimSpace(*rootURL))
		if err != nil {
			fail(err, false, "")
		}
		rootNodeID = id
	}

	linkedRootNodeID := ""
	subtreeRefreshDepth := 1
	downloadDir := "."
	recentChoices := map[string][]string{}
	var persistChoices tuiapp.PersistChoicesFunc
	if cfg, cfgPath, err := projectlink.ResolveByCWD("."); err == nil {
		projectRootID := strings.TrimSpace(cfg.LinkedRootNodeID)
		if projectRootID == "" {
			projectRootID = state.NodeIDFromCanonicalURL(cfg.LinkedRootURL)
		}
		if rootNodeID == "" {
			rootNodeID = projectRootID
		}
		linkedRootNodeID = projectRootID
		if cfg.Preferences.DefaultRefreshDepth > 0 {
			subtreeRefreshDepth = cfg.Preferences.DefaultRefreshDepth
		}
		if strings.TrimSpace(cfg.LastDownloadDir) != "" {
			downloadDir = cfg.LastDownloadDir
		}
		for nodeID, urls := range cfg.RecentAssetChoices {
			recentChoices[nodeID] = append([]string(nil), urls...)
		}
		persistChoices = func(nodeID string, assetURLs []string, targetDir string) error {
			latest, err := projectlink.Load(cfgPath)
			if err != nil {
				return err
			}
			if latest.RecentAssetChoices == nil {
				latest.RecentAssetChoices = map[string][]string{}
			}
			latest.RecentAssetChoices[nodeID] = append([]string(nil), assetURLs...)
			latest.LastDownloadDir = targetDir
			return projectlink.Save(cfgPath, latest)
		}
	}

	refreshExec := func(current state.State, req tuiapp.RefreshRequest) tuiapp.RefreshOutcome {
		start := time.Now()
		out := tuiapp.RefreshOutcome{
			State:        current,
			Scope:        req.Scope,
			TargetNodeID: req.TargetNodeID,
		}

		baseURL, err := themis.NormalizeBaseURL(common.baseURL)
		if err != nil {
			out.Err = err
			return out
		}
		session, err := themis.NewSessionWithAuthConfig(baseURL, themis.AuthConfig{
			SessionFile: common.sessionFile,
		})
		if err != nil {
			out.Err = err
			return out
		}
		if _, err := session.ValidateAuthentication(); err != nil {
			out.Err = err
			return out
		}

		service := discovery.NewService(session.BaseURL)
		var result discovery.RefreshResult
		switch req.Scope {
		case tuiapp.RefreshScopeNode:
			result, err = service.RefreshNode(session.Client, &current, req.TargetURL, 0)
		case tuiapp.RefreshScopeSubtree:
			result, err = service.RefreshNode(session.Client, &current, req.TargetURL, req.Depth)
		case tuiapp.RefreshScopeFull:
			result, err = service.RefreshCatalog(session.Client, &current, req.Depth)
		default:
			err = fmt.Errorf("unsupported refresh scope: %s", req.Scope)
		}
		if err != nil {
			out.Err = err
			out.DurationMs = time.Since(start).Milliseconds()
			return out
		}
		if len(result.Errors) > 0 {
			out.Warnings = append(out.Warnings, result.Errors...)
		}

		if err := state.SaveAtomic(statePath, current, true); err != nil {
			out.Err = err
			out.DurationMs = time.Since(start).Milliseconds()
			return out
		}
		out.State = current
		out.UpdatedNodes = result.UpdatedNodes
		out.DurationMs = time.Since(start).Milliseconds()
		return out
	}

	downloadExec := func(current state.State, req tuiapp.DownloadRequest) tuiapp.DownloadOutcome {
		start := time.Now()
		out := tuiapp.DownloadOutcome{
			NodeID:    req.NodeID,
			TargetDir: req.TargetDir,
		}

		baseURL, err := themis.NormalizeBaseURL(common.baseURL)
		if err != nil {
			out.Err = err
			return out
		}
		session, err := themis.NewSessionWithAuthConfig(baseURL, themis.AuthConfig{
			SessionFile: common.sessionFile,
		})
		if err != nil {
			out.Err = err
			return out
		}
		if _, err := session.ValidateAuthentication(); err != nil {
			out.Err = err
			return out
		}

		items, err := discovery.DownloadAssetRefs(session.Client, req.Assets, req.TargetDir)
		out.DurationMs = time.Since(start).Milliseconds()
		if err != nil {
			out.Err = err
			return out
		}
		files := make([]tuiapp.DownloadedFile, 0, len(items))
		for _, item := range items {
			files = append(files, tuiapp.DownloadedFile{
				Name: item.Name,
				URL:  item.URL,
				Path: item.Path,
			})
		}
		out.Downloaded = len(files)
		out.Files = files
		return out
	}

	if err := tuiapp.Run(tuiapp.Config{
		State:               st,
		RootNodeID:          rootNodeID,
		LinkedRootNodeID:    linkedRootNodeID,
		SubtreeRefreshDepth: subtreeRefreshDepth,
		RefreshExecutor:     refreshExec,
		DownloadExecutor:    downloadExec,
		DefaultDownloadDir:  downloadDir,
		RecentAssetChoices:  recentChoices,
		PersistChoices:      persistChoices,
	}); err != nil {
		fail(err, false, "")
	}
}

func findRepoRoot(start string) (string, error) {
	cur, err := filepath.Abs(start)
	if err != nil {
		return "", fmt.Errorf("resolve cwd: %w", err)
	}

	for {
		gitPath := filepath.Join(cur, ".git")
		if _, err := os.Stat(gitPath); err == nil {
			return cur, nil
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("check %s: %w", gitPath, err)
		}

		parent := filepath.Dir(cur)
		if parent == cur {
			return cur, nil
		}
		cur = parent
	}
}

func addCommonFlags(fs *flag.FlagSet) *commonFlags {
	common := &commonFlags{}

	defaultSessionFile := defaultFromEnv("THEMIS_SESSION_FILE", filepath.Join(mustUserHomeDir(), ".config", "themis", "session.json"))
	defaultBase := defaultFromEnv("THEMIS_BASE_URL", defaultBaseURL)

	fs.StringVar(&common.baseURL, "base-url", defaultBase, "Themis base URL")
	fs.StringVar(&common.sessionFile, "session-file", defaultSessionFile, "Path to persisted session file")
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
	fmt.Println("  project Manage repository link metadata")
	fmt.Println("  tui    Browse cached hierarchy and trigger targeted refresh actions")
	fmt.Println()
	fmt.Println("Common flags (all subcommands):")
	fmt.Println("  --base-url <url>")
	fmt.Println("  --session-file <path>")
	fmt.Println("  --json")
	fmt.Println()
	fmt.Println("Subcommand flags:")
	fmt.Println("  list  --tests-url <url> [--start <n>] [--max <n>] [--max-misses <n>]")
	fmt.Println("  list  --discover [--root-url <url>] [--discover-depth <n>] [--refresh-url <url>] [--refresh-depth <n>] [--full-refresh] [--from-state-only]")
	fmt.Println("  fetch --tests-url <url> [--out <dir>]")
	fmt.Println("  project link --root-url <url> [--default-refresh-depth <n>]")
	fmt.Println("  tui [--root-url <url>]")
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
