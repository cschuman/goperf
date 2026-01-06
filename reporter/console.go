package reporter

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/unsaid-dev/goperf/rules"
)

// ConsoleReporter outputs human-readable reports
type ConsoleReporter struct {
	Context int
}

func (r *ConsoleReporter) Report(issues []rules.Issue) string {
	if len(issues) == 0 {
		return greenBox("PERF-AUDIT: No performance issues detected!")
	}

	var sb strings.Builder

	// Summary
	summary := r.summarize(issues)
	sb.WriteString(summaryBox(summary))
	sb.WriteString("\n\n")

	// Group by severity
	bySeverity := make(map[rules.Severity][]rules.Issue)
	for _, issue := range issues {
		bySeverity[issue.Severity] = append(bySeverity[issue.Severity], issue)
	}

	// Report in order: critical, high, medium, low
	for _, sev := range []rules.Severity{rules.SeverityCritical, rules.SeverityHigh, rules.SeverityMedium, rules.SeverityLow} {
		sevIssues := bySeverity[sev]
		if len(sevIssues) == 0 {
			continue
		}

		for _, issue := range sevIssues {
			sb.WriteString(r.formatIssue(issue))
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

func (r *ConsoleReporter) summarize(issues []rules.Issue) map[string]int {
	summary := map[string]int{
		"total":    len(issues),
		"critical": 0,
		"high":     0,
		"medium":   0,
		"low":      0,
	}

	for _, issue := range issues {
		switch issue.Severity {
		case rules.SeverityCritical:
			summary["critical"]++
		case rules.SeverityHigh:
			summary["high"]++
		case rules.SeverityMedium:
			summary["medium"]++
		case rules.SeverityLow:
			summary["low"]++
		}
	}

	return summary
}

func (r *ConsoleReporter) formatIssue(issue rules.Issue) string {
	var sb strings.Builder

	// Severity badge with color
	sevColor := severityColor(issue.Severity)
	sb.WriteString(fmt.Sprintf("%s │ %s\n", sevColor(issue.Severity.String()), issue.Message))

	// Location
	relPath := issue.File
	if abs, err := filepath.Abs(issue.File); err == nil {
		if rel, err := filepath.Rel(".", abs); err == nil {
			relPath = rel
		}
	}
	sb.WriteString(fmt.Sprintf("     │ %s:%d:%d\n", relPath, issue.Line, issue.Column))

	// Code context if available
	if len(issue.Context) > 0 {
		sb.WriteString("     │\n")
		startLine := issue.Line - len(issue.Context)/2
		if startLine < 1 {
			startLine = 1
		}
		for i, line := range issue.Context {
			lineNum := startLine + i
			prefix := "     │  "
			if lineNum == issue.Line {
				prefix = "   → │  "
			}
			sb.WriteString(fmt.Sprintf("%s%4d│ %s\n", prefix, lineNum, line))
		}
		sb.WriteString("     │\n")
	}

	// Why
	sb.WriteString(fmt.Sprintf("     │ WHY: %s\n", wordWrap(issue.Why, 65, "     │      ")))

	// Fix
	sb.WriteString(fmt.Sprintf("     │ FIX: %s\n", wordWrap(issue.Fix, 65, "     │      ")))

	return sb.String()
}

func summaryBox(summary map[string]int) string {
	var parts []string
	if summary["critical"] > 0 {
		parts = append(parts, red(fmt.Sprintf("%d critical", summary["critical"])))
	}
	if summary["high"] > 0 {
		parts = append(parts, yellow(fmt.Sprintf("%d high", summary["high"])))
	}
	if summary["medium"] > 0 {
		parts = append(parts, cyan(fmt.Sprintf("%d medium", summary["medium"])))
	}
	if summary["low"] > 0 {
		parts = append(parts, dim(fmt.Sprintf("%d low", summary["low"])))
	}

	content := fmt.Sprintf("PERF-AUDIT: %d issues found (%s)", summary["total"], strings.Join(parts, ", "))

	width := len(content) + 4
	if width < 60 {
		width = 60
	}

	border := strings.Repeat("─", width-2)
	padding := strings.Repeat(" ", width-len(content)-4)

	return fmt.Sprintf("╭%s╮\n│ %s%s │\n╰%s╯", border, content, padding, border)
}

func greenBox(content string) string {
	width := len(content) + 4
	border := strings.Repeat("─", width-2)
	return fmt.Sprintf("╭%s╮\n│ %s │\n╰%s╯", border, green(content), border)
}

// ANSI color codes
func red(s string) string     { return "\033[31m" + s + "\033[0m" }
func yellow(s string) string  { return "\033[33m" + s + "\033[0m" }
func cyan(s string) string    { return "\033[36m" + s + "\033[0m" }
func green(s string) string   { return "\033[32m" + s + "\033[0m" }
func dim(s string) string     { return "\033[2m" + s + "\033[0m" }
func bold(s string) string    { return "\033[1m" + s + "\033[0m" }

func severityColor(s rules.Severity) func(string) string {
	switch s {
	case rules.SeverityCritical:
		return func(str string) string { return bold(red(str)) }
	case rules.SeverityHigh:
		return red
	case rules.SeverityMedium:
		return yellow
	default:
		return dim
	}
}

func wordWrap(text string, width int, indent string) string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return text
	}

	var lines []string
	var currentLine strings.Builder

	for _, word := range words {
		if currentLine.Len()+len(word)+1 > width {
			lines = append(lines, currentLine.String())
			currentLine.Reset()
		}
		if currentLine.Len() > 0 {
			currentLine.WriteString(" ")
		}
		currentLine.WriteString(word)
	}
	if currentLine.Len() > 0 {
		lines = append(lines, currentLine.String())
	}

	return strings.Join(lines, "\n"+indent)
}
