package fixer

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
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

// validatePathForWrite ensures the file path is safe for writing
func validatePathForWrite(filename string) error {
	// Get current working directory
	cwd, err := filepath.Abs(".")
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// Get absolute path of target
	absPath, err := filepath.Abs(filename)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	// Clean the path to resolve any .. components
	absPath = filepath.Clean(absPath)

	// Check if path is within current working directory
	if !strings.HasPrefix(absPath, cwd+string(filepath.Separator)) && absPath != cwd {
		return fmt.Errorf("refusing to write to %q: path is outside working directory (security restriction)", filename)
	}

	// Check for symlinks to prevent TOCTOU attacks
	realPath, err := filepath.EvalSymlinks(absPath)
	if err == nil && realPath != absPath {
		// Path contains symlinks - verify the real path is also within CWD
		if !strings.HasPrefix(realPath, cwd+string(filepath.Separator)) && realPath != cwd {
			return fmt.Errorf("refusing to write to %q: symlink points outside working directory (security restriction)", filename)
		}
	}

	// Verify it's a regular file (not a device, socket, etc.)
	info, err := os.Lstat(absPath)
	if err == nil {
		// File exists - check it's a regular file
		if info.Mode()&os.ModeType != 0 {
			return fmt.Errorf("refusing to write to %q: not a regular file", filename)
		}
	}

	return nil
}

// FixIssues attempts to fix the given issues
func (f *Fixer) FixIssues(issues []rules.Issue) []Fix {
	// Preallocate: assume ~1 fix per issue
	fixes := make([]Fix, 0, len(issues))

	// Group issues by file with size hint
	byFile := make(map[string][]rules.Issue, len(issues)/2+1)
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
	fixes := make([]Fix, 0, len(issues))

	// Validate path before any operations
	if err := validatePathForWrite(filename); err != nil {
		if f.Verbose {
			fmt.Fprintf(os.Stderr, "Skipping %s: %v\n", filename, err)
		}
		return fixes
	}

	src, err := os.ReadFile(filename)
	if err != nil {
		return fixes
	}

	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return fixes
	}

	lines := strings.Split(string(src), "\n")

	for _, issue := range issues {
		fix := f.attemptFix(issue, astFile, fset, lines)
		if fix != nil {
			fixes = append(fixes, *fix)
		}
	}

	// NOTE: Auto-fix currently only generates suggestions.
	// Actual AST modification and file writing is not implemented
	// because safe AST modification is complex and error-prone.
	// The fixes are displayed as suggestions for manual application.

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

	// Extract slice name from message safely
	msg := issue.Message
	start := strings.Index(msg, "'")
	end := strings.LastIndex(msg, "'")
	if start >= 0 && end > start && start+1 < len(msg) {
		sliceName := msg[start+1 : end]
		// Validate the extracted name looks like an identifier
		if isValidIdentifier(sliceName) {
			fix.Fixed = fmt.Sprintf("%s = make([]T, 0, expectedSize) // Preallocate %s", sliceName, sliceName)
		}
	}

	return fix
}

func (f *Fixer) fixMissingBodyClose(issue rules.Issue, file *ast.File, fset *token.FileSet, lines []string) *Fix {
	line := issue.Line
	original := getLine(lines, line)

	// Find the variable name from the message safely
	msg := issue.Message
	start := strings.Index(msg, "'")
	end := strings.LastIndex(msg, "'")

	varName := "resp"
	if start >= 0 && end > start && start+1 < len(msg) {
		extracted := msg[start+1 : end]
		if isValidIdentifier(extracted) {
			varName = extracted
		}
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

	// Extract cancel function name from message safely
	msg := issue.Message
	start := strings.Index(msg, "'")
	end := strings.LastIndex(msg, "'")

	cancelName := "cancel"
	if start >= 0 && end > start && start+1 < len(msg) {
		extracted := msg[start+1 : end]
		if isValidIdentifier(extracted) {
			cancelName = extracted
		}
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

// isValidIdentifier checks if a string looks like a valid Go identifier
func isValidIdentifier(s string) bool {
	if len(s) == 0 || len(s) > 100 {
		return false
	}
	for i, c := range s {
		if i == 0 {
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_') {
				return false
			}
		} else {
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
				return false
			}
		}
	}
	return true
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
		fmt.Println("=== DRY RUN: Suggested fixes (no files modified) ===")
		fmt.Println()
	} else {
		fmt.Println("=== Suggested fixes ===")
		fmt.Println()
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
