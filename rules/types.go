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
	var lines []string
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
