package reporter

import (
	"encoding/json"

	"github.com/cschuman/goperf/rules"
)

// JSONReporter outputs machine-readable JSON
type JSONReporter struct{}

type JSONOutput struct {
	Summary Summary       `json:"summary"`
	Issues  []JSONIssue   `json:"issues"`
}

type Summary struct {
	Total    int `json:"total"`
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
}

type JSONIssue struct {
	Rule     string   `json:"rule"`
	Category string   `json:"category"`
	Severity string   `json:"severity"`
	File     string   `json:"file"`
	Line     int      `json:"line"`
	Column   int      `json:"column"`
	Message  string   `json:"message"`
	Why      string   `json:"why"`
	Fix      string   `json:"fix"`
	Context  []string `json:"context,omitempty"`
}

func (r *JSONReporter) Report(issues []rules.Issue) string {
	summary := Summary{Total: len(issues)}

	jsonIssues := make([]JSONIssue, 0, len(issues))
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

		jsonIssues = append(jsonIssues, JSONIssue{
			Rule:     issue.Rule,
			Category: issue.Category,
			Severity: issue.Severity.String(),
			File:     issue.File,
			Line:     issue.Line,
			Column:   issue.Column,
			Message:  issue.Message,
			Why:      issue.Why,
			Fix:      issue.Fix,
			Context:  issue.Context,
		})
	}

	output := JSONOutput{
		Summary: summary,
		Issues:  jsonIssues,
	}

	b, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return `{"error": "failed to marshal output"}`
	}

	return string(b)
}
