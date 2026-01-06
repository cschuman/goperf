package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/unsaid-dev/goperf/fixer"
	"github.com/unsaid-dev/goperf/reporter"
	"github.com/unsaid-dev/goperf/rules"
)

var (
	rulesFlag   = flag.String("rules", "all", "Comma-separated rules to run: algorithm,allocation,database,concurrency,io,cache,context,memory,benchmark,all")
	formatFlag  = flag.String("format", "console", "Output format: console, json, diff")
	failOnFlag  = flag.String("fail-on", "", "Exit with code 1 if issues at this level or higher: low, medium, high, critical")
	contextFlag = flag.Int("context", 3, "Lines of context to show around issues")
	ignoreFlag  = flag.String("ignore", "", "Comma-separated paths to ignore")
	verboseFlag = flag.Bool("verbose", false, "Show verbose output")
	versionFlag = flag.Bool("version", false, "Show version")
	fixFlag     = flag.Bool("fix", false, "Automatically fix issues where possible")
	dryRunFlag  = flag.Bool("dry-run", false, "Show fixes without applying them (use with --fix)")
)

var version = "0.1.0"

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `goperf - Performance Pattern Detector for Go

Scans Go source code for common performance anti-patterns.

Usage:
  goperf [flags] [packages...]

Examples:
  goperf ./...                          # Audit entire project
  goperf --rules=algorithm ./internal/  # Only algorithm rules
  goperf --format=json ./...            # JSON output for CI
  goperf --fail-on=high ./...           # Exit 1 if high+ issues
  goperf --fix --dry-run ./...          # Preview auto-fixes
  goperf --fix ./...                    # Apply auto-fixes

Flags:
`)
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Rule Categories:
  algorithm   - O(n²) loops, repeated linear searches
  allocation  - Unpreallocated slices, string concatenation, interface boxing
  database    - N+1 queries, SQL in loops, connection pool issues
  concurrency - Lock contention, unbuffered channels
  io          - Unbuffered I/O, HTTP body handling, response buffering
  cache       - Repeated regex/template compilation, JSON schema in loops
  context     - Missing timeouts, context leaks, context.Background in handlers
  memory      - Large struct copying, pprof in hot paths, heap escapes
  benchmark   - Functions with performance patterns that need benchmarks

Severity Levels:
  critical - Will cause production issues
  high     - Significant performance impact
  medium   - Moderate impact, should fix
  low      - Minor optimization opportunity

Auto-Fix Support:
  The following rules support auto-fix:
  - string-concat-loop    → strings.Builder suggestion
  - unpreallocated-slice  → make() with capacity
  - missing-body-close    → defer Body.Close()
  - context-leak          → defer cancel()
`)
	}
	flag.Parse()

	if *versionFlag {
		fmt.Printf("goperf version %s\n", version)
		os.Exit(0)
	}

	paths := flag.Args()
	if len(paths) == 0 {
		paths = []string{"./..."}
	}

	// Parse ignore paths
	var ignorePaths []string
	if *ignoreFlag != "" {
		ignorePaths = strings.Split(*ignoreFlag, ",")
	}

	// Parse rules to run
	ruleSet := parseRules(*rulesFlag)

	// Create analyzer
	analyzer := rules.NewAnalyzer(rules.AnalyzerConfig{
		Rules:       ruleSet,
		IgnorePaths: ignorePaths,
		Context:     *contextFlag,
		Verbose:     *verboseFlag,
	})

	// Collect Go files
	var files []string
	for _, pattern := range paths {
		matches, err := collectGoFiles(pattern, ignorePaths)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error collecting files: %v\n", err)
			os.Exit(1)
		}
		files = append(files, matches...)
	}

	if *verboseFlag {
		fmt.Fprintf(os.Stderr, "Analyzing %d files...\n", len(files))
	}

	// Run analysis
	issues := analyzer.Analyze(files)

	// Handle fix mode
	if *fixFlag {
		f := fixer.NewFixer(*dryRunFlag, *verboseFlag)
		fixes := f.FixIssues(issues)

		if *formatFlag == "diff" {
			fmt.Println(fixer.GenerateDiff(fixes))
		} else {
			fixer.PrintFixes(fixes, *dryRunFlag)
		}

		// Still show summary
		if len(issues) > 0 {
			fmt.Printf("\nTotal issues found: %d\n", len(issues))
			fixable := 0
			for _, fix := range fixes {
				if fix.Fixed != "" {
					fixable++
				}
			}
			fmt.Printf("Auto-fixable: %d\n", fixable)
		}
		return
	}

	// Report results
	var rep reporter.Reporter
	switch *formatFlag {
	case "json":
		rep = &reporter.JSONReporter{}
	case "diff":
		// Generate diff output even without --fix
		f := fixer.NewFixer(true, *verboseFlag)
		fixes := f.FixIssues(issues)
		fmt.Println(fixer.GenerateDiff(fixes))
		return
	default:
		rep = &reporter.ConsoleReporter{Context: *contextFlag}
	}

	output := rep.Report(issues)
	fmt.Println(output)

	// Exit with appropriate code
	if *failOnFlag != "" {
		threshold := parseSeverity(*failOnFlag)
		for _, issue := range issues {
			if issue.Severity >= threshold {
				os.Exit(1)
			}
		}
	}
}

func parseRules(rulesStr string) []string {
	if rulesStr == "all" {
		return []string{"algorithm", "allocation", "database", "concurrency", "io", "cache", "context", "memory", "benchmark"}
	}
	return strings.Split(rulesStr, ",")
}

func parseSeverity(s string) rules.Severity {
	switch strings.ToLower(s) {
	case "critical":
		return rules.SeverityCritical
	case "high":
		return rules.SeverityHigh
	case "medium":
		return rules.SeverityMedium
	default:
		return rules.SeverityLow
	}
}

func collectGoFiles(pattern string, ignorePaths []string) ([]string, error) {
	var files []string

	// Handle ./... pattern
	if strings.HasSuffix(pattern, "/...") {
		root := strings.TrimSuffix(pattern, "/...")
		if root == "." {
			root = "."
		}
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				// Skip vendor, testdata, etc.
				base := filepath.Base(path)
				if base == "vendor" || base == "testdata" || base == ".git" {
					return filepath.SkipDir
				}
				for _, ignore := range ignorePaths {
					if strings.Contains(path, ignore) {
						return filepath.SkipDir
					}
				}
				return nil
			}
			if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
				files = append(files, path)
			}
			return nil
		})
		return files, err
	}

	// Handle direct file or directory
	info, err := os.Stat(pattern)
	if err != nil {
		return nil, err
	}

	if info.IsDir() {
		entries, err := os.ReadDir(pattern)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") && !strings.HasSuffix(entry.Name(), "_test.go") {
				files = append(files, filepath.Join(pattern, entry.Name()))
			}
		}
	} else if strings.HasSuffix(pattern, ".go") {
		files = append(files, pattern)
	}

	return files, nil
}

// For JSON output in CI
type JSONOutput struct {
	Summary Summary       `json:"summary"`
	Issues  []rules.Issue `json:"issues"`
}

type Summary struct {
	Total    int `json:"total"`
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
}

func toJSON(issues []rules.Issue) string {
	summary := Summary{Total: len(issues)}
	for _, issue := range issues {
		switch issue.Severity {
		case rules.SeverityCritical:
			summary.Critical++
		case rules.SeverityHigh:
			summary.High++
		case rules.SeverityMedium:
			summary.Medium++
		case rules.SeverityLow:
			summary.Low++
		}
	}

	output := JSONOutput{Summary: summary, Issues: issues}
	b, _ := json.MarshalIndent(output, "", "  ")
	return string(b)
}
