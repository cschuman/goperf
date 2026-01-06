package rules

import (
	"go/ast"
	"go/token"
)

// Severity levels for issues
type Severity int

const (
	SeverityLow Severity = iota
	SeverityMedium
	SeverityHigh
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

// Issue represents a detected performance issue
type Issue struct {
	Rule        string   `json:"rule"`
	Category    string   `json:"category"`
	Severity    Severity `json:"severity"`
	File        string   `json:"file"`
	Line        int      `json:"line"`
	Column      int      `json:"column"`
	Message     string   `json:"message"`
	Why         string   `json:"why"`
	Fix         string   `json:"fix"`
	CodeSnippet string   `json:"code_snippet,omitempty"`
	Context     []string `json:"context,omitempty"`
}

// Rule is the interface all detection rules must implement
type Rule interface {
	Name() string
	Category() string
	Check(file *ast.File, fset *token.FileSet, src []byte) []Issue
}

// AnalyzerConfig configures the analyzer
type AnalyzerConfig struct {
	Rules       []string
	IgnorePaths []string
	Context     int
	Verbose     bool
}

// RuleRegistry holds all available rules
var RuleRegistry = make(map[string][]Rule)

// RegisterRule adds a rule to the registry
func RegisterRule(category string, rule Rule) {
	RuleRegistry[category] = append(RuleRegistry[category], rule)
}

// Helper to extract code context around a position
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

// IgnoreSet tracks which lines should be ignored based on perf:ignore comments
type IgnoreSet struct {
	lines  map[int]bool   // Line-level ignores
	ranges [][2]int       // Start/end ranges for block ignores
	rules  map[int]string // Optional: specific rules to ignore per line
}

// NewIgnoreSet parses source code for perf:ignore comments
// Supports:
//   - // perf:ignore - ignore the next line or same line
//   - // perf:ignore rule-name - ignore specific rule
//   - // perf:ignore-start / // perf:ignore-end - block ignore
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
