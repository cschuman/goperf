package fixer

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"strings"

	"github.com/unsaid-dev/goperf/rules"
)

// Fix represents an auto-fixable change
type Fix struct {
	File     string
	Line     int
	Original string
	Fixed    string
	Rule     string
	Applied  bool
}

// Fixer handles automatic code fixes
type Fixer struct {
	DryRun  bool
	Verbose bool
}

// NewFixer creates a new fixer
func NewFixer(dryRun, verbose bool) *Fixer {
	return &Fixer{
		DryRun:  dryRun,
		Verbose: verbose,
	}
}

// FixIssues attempts to fix the given issues
func (f *Fixer) FixIssues(issues []rules.Issue) []Fix {
	var fixes []Fix

	// Group issues by file
	byFile := make(map[string][]rules.Issue)
	for _, issue := range issues {
		byFile[issue.File] = append(byFile[issue.File], issue)
	}

	for file, fileIssues := range byFile {
		fileFixes := f.fixFile(file, fileIssues)
		fixes = append(fixes, fileFixes...)
	}

	return fixes
}

func (f *Fixer) fixFile(filename string, issues []rules.Issue) []Fix {
	var fixes []Fix

	src, err := os.ReadFile(filename)
	if err != nil {
		return fixes
	}

	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return fixes
	}

	modified := false
	lines := strings.Split(string(src), "\n")

	for _, issue := range issues {
		fix := f.attemptFix(issue, astFile, fset, lines)
		if fix != nil {
			fixes = append(fixes, *fix)
			if !f.DryRun && fix.Fixed != "" {
				modified = true
			}
		}
	}

	if modified && !f.DryRun {
		// Format and write back
		var buf bytes.Buffer
		if err := format.Node(&buf, fset, astFile); err == nil {
			os.WriteFile(filename, buf.Bytes(), 0644)
		}
	}

	return fixes
}

func (f *Fixer) attemptFix(issue rules.Issue, file *ast.File, fset *token.FileSet, lines []string) *Fix {
	switch issue.Rule {
	case "string-concat-loop":
		return f.fixStringConcat(issue, file, fset, lines)
	case "unpreallocated-slice":
		return f.fixUnpreallocatedSlice(issue, file, fset, lines)
	case "missing-body-close":
		return f.fixMissingBodyClose(issue, file, fset, lines)
	case "context-leak":
		return f.fixContextLeak(issue, file, fset, lines)
	default:
		// Return suggestion-only fix
		return &Fix{
			File:     issue.File,
			Line:     issue.Line,
			Original: getLine(lines, issue.Line),
			Fixed:    "", // No auto-fix available
			Rule:     issue.Rule,
			Applied:  false,
		}
	}
}

func (f *Fixer) fixStringConcat(issue rules.Issue, file *ast.File, fset *token.FileSet, lines []string) *Fix {
	// Find the function containing this line
	line := issue.Line
	original := getLine(lines, line)

	// Suggest using strings.Builder
	fix := &Fix{
		File:     issue.File,
		Line:     line,
		Original: original,
		Rule:     issue.Rule,
		Applied:  false,
	}

	// Generate suggestion (actual AST modification is complex)
	fix.Fixed = "// TODO: Replace with strings.Builder\n" +
		"// var b strings.Builder\n" +
		"// for ... { b.WriteString(s) }\n" +
		"// result := b.String()"

	return fix
}

func (f *Fixer) fixUnpreallocatedSlice(issue rules.Issue, file *ast.File, fset *token.FileSet, lines []string) *Fix {
	line := issue.Line
	original := getLine(lines, line)

	fix := &Fix{
		File:     issue.File,
		Line:     line,
		Original: original,
		Rule:     issue.Rule,
		Applied:  false,
	}

	// Extract slice name from message
	msg := issue.Message
	start := strings.Index(msg, "'")
	end := strings.LastIndex(msg, "'")
	if start >= 0 && end > start {
		sliceName := msg[start+1 : end]
		fix.Fixed = fmt.Sprintf("%s = make([]T, 0, expectedSize) // Preallocate %s", sliceName, sliceName)
	}

	return fix
}

func (f *Fixer) fixMissingBodyClose(issue rules.Issue, file *ast.File, fset *token.FileSet, lines []string) *Fix {
	line := issue.Line
	original := getLine(lines, line)

	// Find the variable name from the message
	msg := issue.Message
	start := strings.Index(msg, "'")
	end := strings.LastIndex(msg, "'")

	varName := "resp"
	if start >= 0 && end > start {
		varName = msg[start+1 : end]
	}

	fix := &Fix{
		File:     issue.File,
		Line:     line,
		Original: original,
		Rule:     issue.Rule,
		Applied:  false,
		Fixed:    fmt.Sprintf("defer %s.Body.Close()", varName),
	}

	return fix
}

func (f *Fixer) fixContextLeak(issue rules.Issue, file *ast.File, fset *token.FileSet, lines []string) *Fix {
	line := issue.Line
	original := getLine(lines, line)

	// Extract cancel function name from message
	msg := issue.Message
	start := strings.Index(msg, "'")
	end := strings.LastIndex(msg, "'")

	cancelName := "cancel"
	if start >= 0 && end > start {
		cancelName = msg[start+1 : end]
	}

	fix := &Fix{
		File:     issue.File,
		Line:     line,
		Original: original,
		Rule:     issue.Rule,
		Applied:  false,
		Fixed:    fmt.Sprintf("defer %s()", cancelName),
	}

	return fix
}

func getLine(lines []string, lineNum int) string {
	if lineNum > 0 && lineNum <= len(lines) {
		return lines[lineNum-1]
	}
	return ""
}

// PrintFixes displays the fixes in a readable format
func PrintFixes(fixes []Fix, dryRun bool) {
	if len(fixes) == 0 {
		fmt.Println("No auto-fixes available for the detected issues.")
		return
	}

	if dryRun {
		fmt.Println("=== DRY RUN: Suggested fixes (no files modified) ===\n")
	} else {
		fmt.Println("=== Applied fixes ===\n")
	}

	for _, fix := range fixes {
		fmt.Printf("File: %s:%d\n", fix.File, fix.Line)
		fmt.Printf("Rule: %s\n", fix.Rule)
		if fix.Original != "" {
			fmt.Printf("Original: %s\n", strings.TrimSpace(fix.Original))
		}
		if fix.Fixed != "" {
			fmt.Printf("Fix: %s\n", fix.Fixed)
		} else {
			fmt.Println("Fix: Manual intervention required - see issue suggestion")
		}
		fmt.Println()
	}
}

// GenerateDiff creates a unified diff for review
func GenerateDiff(fixes []Fix) string {
	var buf bytes.Buffer

	for _, fix := range fixes {
		if fix.Fixed == "" {
			continue
		}

		buf.WriteString(fmt.Sprintf("--- a/%s\n", fix.File))
		buf.WriteString(fmt.Sprintf("+++ b/%s\n", fix.File))
		buf.WriteString(fmt.Sprintf("@@ -%d,1 +%d,1 @@\n", fix.Line, fix.Line))
		buf.WriteString(fmt.Sprintf("-%s\n", fix.Original))
		buf.WriteString(fmt.Sprintf("+%s\n", fix.Fixed))
		buf.WriteString("\n")
	}

	return buf.String()
}
