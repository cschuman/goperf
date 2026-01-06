// Package rules provides the core types and interfaces for goperf's
// static analysis rules. Each rule detects specific performance anti-patterns
// in Go source code.
//
// Rules are organized by category (algorithm, allocation, database, etc.)
// and registered via the RegisterRule function during package initialization.
//
// Example usage:
//
//	analyzer := rules.NewAnalyzer(rules.AnalyzerConfig{
//	    Rules:   []string{"algorithm", "database"},
//	    Context: 3,
//	})
//	issues, _ := analyzer.Analyze("./...")
package rules

import (
	"go/ast"
	"go/token"
)

// Severity represents the impact level of a detected performance issue.
// Higher severity issues have greater performance impact and should
// be addressed with higher priority.
type Severity int

const (
	// SeverityLow indicates a minor optimization opportunity.
	// These are "nice to have" improvements that may not have
	// measurable impact in most applications.
	SeverityLow Severity = iota

	// SeverityMedium indicates a moderate performance concern.
	// These issues should be addressed but may not be critical.
	SeverityMedium

	// SeverityHigh indicates a significant performance problem.
	// These issues will likely cause noticeable slowdowns and
	// should be fixed before release.
	SeverityHigh

	// SeverityCritical indicates a severe performance issue
	// that will cause production problems. Fix immediately.
	SeverityCritical
)

func (s Severity) String() string {
	switch s {
	case SeverityCritical:
		return "CRITICAL"
	case SeverityHigh:
		return "HIGH"
	case SeverityMedium:
		return "MEDIUM"
	default:
		return "LOW"
	}
}

// Issue represents a detected performance anti-pattern in source code.
// Each issue includes location information, a description, an explanation
// of why it's a problem, and a suggested fix.
type Issue struct {
	// Rule is the identifier of the rule that detected this issue
	// (e.g., "nested-loop", "append-in-loop", "sql-in-loop").
	Rule string `json:"rule"`

	// Category groups related rules together
	// (e.g., "algorithm", "allocation", "database").
	Category string `json:"category"`

	// Severity indicates the impact level of this issue.
	Severity Severity `json:"severity"`

	// File is the path to the source file containing the issue.
	File string `json:"file"`

	// Line is the 1-indexed line number where the issue occurs.
	Line int `json:"line"`

	// Column is the 1-indexed column number where the issue occurs.
	Column int `json:"column"`

	// Message is a brief description of the detected issue.
	Message string `json:"message"`

	// Why explains the performance impact of this pattern.
	Why string `json:"why"`

	// Fix suggests how to resolve the issue.
	Fix string `json:"fix"`

	// CodeSnippet contains the problematic line of code.
	CodeSnippet string `json:"code_snippet,omitempty"`

	// Context contains surrounding lines of code for display.
	Context []string `json:"context,omitempty"`
}

// Rule is the interface that all detection rules must implement.
// Rules are the core building blocks of goperf's analysis engine.
//
// To create a custom rule:
//
//  1. Implement the Rule interface
//  2. Register it using RegisterRule in an init() function
//  3. The analyzer will automatically pick it up
//
// Example:
//
//	type MyRule struct{}
//
//	func (r *MyRule) Name() string     { return "my-pattern" }
//	func (r *MyRule) Category() string { return "custom" }
//	func (r *MyRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
//	    // Analyze the AST and return issues
//	    return nil
//	}
//
//	func init() {
//	    RegisterRule("custom", &MyRule{})
//	}
type Rule interface {
	// Name returns a unique identifier for this rule (e.g., "append-in-loop").
	Name() string

	// Category returns the rule's category (e.g., "allocation", "database").
	Category() string

	// Check analyzes a Go source file and returns any detected issues.
	// The file parameter is the parsed AST, fset provides position info,
	// and src is the original source code for context extraction.
	Check(file *ast.File, fset *token.FileSet, src []byte) []Issue
}

// AnalyzerConfig configures the behavior of the analyzer.
type AnalyzerConfig struct {
	// Rules is a list of rule categories to run.
	// An empty list means all rules. Example: []string{"algorithm", "database"}
	Rules []string

	// IgnorePaths is a list of path patterns to skip during analysis.
	// Supports glob patterns. Example: []string{"vendor/**", "**/*_test.go"}
	IgnorePaths []string

	// Context is the number of lines of code to include around each issue.
	Context int

	// Verbose enables detailed output during analysis.
	Verbose bool
}

// RuleRegistry maps category names to the rules in that category.
// Rules are added to this registry via RegisterRule during init().
var RuleRegistry = make(map[string][]Rule)

// RegisterRule adds a rule to the registry under the specified category.
// This should be called from init() functions in rule implementation files.
func RegisterRule(category string, rule Rule) {
	RuleRegistry[category] = append(RuleRegistry[category], rule)
}

// ExtractContext returns lines of code surrounding the given position.
// It returns up to contextLines lines before and after the specified position.
func ExtractContext(src []byte, pos token.Position, contextLines int) []string {
	lines := splitLines(src)
	if pos.Line <= 0 || pos.Line > len(lines) {
		return nil
	}

	start := pos.Line - contextLines - 1
	if start < 0 {
		start = 0
	}
	end := pos.Line + contextLines
	if end > len(lines) {
		end = len(lines)
	}

	context := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		context = append(context, lines[i])
	}
	return context
}

func splitLines(src []byte) []string {
	// Count newlines to preallocate
	lineCount := 1
	for _, b := range src {
		if b == '\n' {
			lineCount++
		}
	}
	lines := make([]string, 0, lineCount)
	start := 0
	for i, b := range src {
		if b == '\n' {
			lines = append(lines, string(src[start:i]))
			start = i + 1
		}
	}
	if start < len(src) {
		lines = append(lines, string(src[start:]))
	}
	return lines
}

// IgnoreSet tracks which lines should be ignored based on perf:ignore comments.
// This allows developers to suppress specific warnings when they've verified
// the code is intentional or when there's a false positive.
type IgnoreSet struct {
	lines  map[int]bool   // Line-level ignores
	ranges [][2]int       // Start/end ranges for block ignores
	rules  map[int]string // Optional: specific rules to ignore per line
}

// NewIgnoreSet parses source code to find perf:ignore comments.
// It supports several ignore patterns:
//
// Line-level ignore (ignores the current and next line):
//
//	// perf:ignore
//	for _, item := range items {
//	    db.Exec(query, item)  // This line is ignored
//	}
//
// Same-line ignore:
//
//	db.Exec(query, item) // perf:ignore
//
// Rule-specific ignore:
//
//	// perf:ignore sql-in-loop
//	for _, item := range items {
//	    db.Exec(query, item)  // Only sql-in-loop is ignored
//	}
//
// Block ignore (ignores all lines between start and end):
//
//	// perf:ignore-start
//	for _, item := range items {
//	    db.Exec(query, item)
//	}
//	// perf:ignore-end
func NewIgnoreSet(src []byte) *IgnoreSet {
	is := &IgnoreSet{
		lines: make(map[int]bool),
		rules: make(map[int]string),
	}

	lines := splitLines(src)
	var blockStart int
	inBlock := false

	for i, line := range lines {
		lineNum := i + 1 // 1-indexed

		// Check for block markers
		if containsIgnoreStart(line) {
			inBlock = true
			blockStart = lineNum
			continue
		}
		if containsIgnoreEnd(line) && inBlock {
			is.ranges = append(is.ranges, [2]int{blockStart, lineNum})
			inBlock = false
			continue
		}

		// Check for line-level ignore
		if rule, hasIgnore := parseIgnoreComment(line); hasIgnore {
			// Ignore the current line and the next line
			is.lines[lineNum] = true
			is.lines[lineNum+1] = true
			if rule != "" {
				is.rules[lineNum] = rule
				is.rules[lineNum+1] = rule
			}
		}
	}

	return is
}

// ShouldIgnore returns true if the given line should be ignored
func (is *IgnoreSet) ShouldIgnore(line int, rule string) bool {
	// Check block ranges
	for _, r := range is.ranges {
		if line >= r[0] && line <= r[1] {
			return true
		}
	}

	// Check line-level ignores
	if is.lines[line] {
		// If a specific rule is set, only ignore that rule
		if specificRule, ok := is.rules[line]; ok && specificRule != "" {
			return rule == specificRule
		}
		return true
	}

	return false
}

func containsIgnoreStart(line string) bool {
	return containsComment(line, "perf:ignore-start")
}

func containsIgnoreEnd(line string) bool {
	return containsComment(line, "perf:ignore-end")
}

func containsComment(line, marker string) bool {
	idx := -1
	for i := 0; i < len(line)-1; i++ {
		if line[i] == '/' && line[i+1] == '/' {
			idx = i
			break
		}
	}
	if idx == -1 {
		return false
	}
	comment := line[idx:]
	for i := 0; i < len(comment)-len(marker)+1; i++ {
		if comment[i:i+len(marker)] == marker {
			return true
		}
	}
	return false
}

// parseIgnoreComment checks if line has a perf:ignore comment and returns
// the optional rule name to ignore
func parseIgnoreComment(line string) (rule string, hasIgnore bool) {
	marker := "perf:ignore"

	idx := -1
	for i := 0; i < len(line)-1; i++ {
		if line[i] == '/' && line[i+1] == '/' {
			idx = i
			break
		}
	}
	if idx == -1 {
		return "", false
	}

	comment := line[idx+2:] // Skip //

	// Find perf:ignore in comment
	markerIdx := -1
	for i := 0; i < len(comment)-len(marker)+1; i++ {
		if comment[i:i+len(marker)] == marker {
			markerIdx = i
			break
		}
	}
	if markerIdx == -1 {
		return "", false
	}

	// Don't match perf:ignore-start or perf:ignore-end
	afterMarker := comment[markerIdx+len(marker):]
	if len(afterMarker) > 0 && afterMarker[0] == '-' {
		return "", false
	}

	// Extract optional rule name
	afterMarker = trimLeftSpace(afterMarker)
	if len(afterMarker) > 0 {
		// Get the rule name (first word after perf:ignore)
		end := 0
		for end < len(afterMarker) && afterMarker[end] != ' ' && afterMarker[end] != '\t' && afterMarker[end] != '\n' {
			end++
		}
		rule = afterMarker[:end]
	}

	return rule, true
}

func trimLeftSpace(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' && s[i] != '\t' {
			return s[i:]
		}
	}
	return ""
}
