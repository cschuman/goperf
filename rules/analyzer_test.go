package rules

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
)

func testdataPath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("..", "testdata", name)
}

func TestAnalyzerBasics(t *testing.T) {
	analyzer := NewAnalyzer(AnalyzerConfig{
		Rules: []string{"allocation"},
	})

	file := testdataPath(t, "analyzer_basic.go")
	issues, errs := analyzer.Analyze([]string{file})
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(issues) != 2 {
		t.Fatalf("got %d issues, want 2", len(issues))
	}

	seen := make(map[string]bool)
	for _, issue := range issues {
		if issue.File != file {
			t.Fatalf("issue file = %q, want %q", issue.File, file)
		}
		seen[issue.Rule] = true
	}

	if !seen["unpreallocated-slice"] {
		t.Errorf("missing rule: unpreallocated-slice")
	}
	if !seen["string-concat-loop"] {
		t.Errorf("missing rule: string-concat-loop")
	}
}

func TestAnalyzerIgnorePaths(t *testing.T) {
	analyzer := NewAnalyzer(AnalyzerConfig{
		Rules:       []string{"allocation"},
		IgnorePaths: []string{"analyzer_ignored.go"},
	})

	ignored := testdataPath(t, "analyzer_ignored.go")
	scanned := testdataPath(t, "analyzer_scanned.go")
	issues, errs := analyzer.Analyze([]string{ignored, scanned})
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(issues) != 1 {
		t.Fatalf("got %d issues, want 1", len(issues))
	}
	if issues[0].File != scanned {
		t.Fatalf("issue file = %q, want %q", issues[0].File, scanned)
	}
}

func TestAnalyzerEmptyFile(t *testing.T) {
	analyzer := NewAnalyzer(AnalyzerConfig{
		Rules: []string{"allocation"},
	})

	file := testdataPath(t, "analyzer_empty.go")
	issues, errs := analyzer.Analyze([]string{file})
	if len(errs) != 1 {
		t.Fatalf("got %d errors, want 1", len(errs))
	}
	if len(issues) != 0 {
		t.Fatalf("got %d issues, want 0", len(issues))
	}
}

func TestAnalyzerSyntaxError(t *testing.T) {
	analyzer := NewAnalyzer(AnalyzerConfig{
		Rules: []string{"allocation"},
	})

	bad := testdataPath(t, "analyzer_syntax_error.go")
	good := testdataPath(t, "analyzer_basic.go")
	issues, errs := analyzer.Analyze([]string{bad, good})
	if len(errs) != 1 {
		t.Fatalf("got %d errors, want 1", len(errs))
	}
	if len(issues) != 2 {
		t.Fatalf("got %d issues, want 2", len(issues))
	}
	for _, issue := range issues {
		if issue.File == bad {
			t.Fatalf("unexpected issue from syntax error file: %q", issue.Rule)
		}
	}
}

func TestAnalyzerContextExtraction(t *testing.T) {
	analyzer := NewAnalyzer(AnalyzerConfig{
		Rules:   []string{"allocation"},
		Context: 1,
	})

	file := testdataPath(t, "analyzer_context.go")
	issues, errs := analyzer.Analyze([]string{file})
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(issues) != 1 {
		t.Fatalf("got %d issues, want 1", len(issues))
	}

	want := []string{
		"\tfor _, item := range items {",
		"\t\tout = append(out, item)",
		"\t}",
	}

	if len(issues[0].Context) != len(want) {
		t.Fatalf("context lines = %d, want %d", len(issues[0].Context), len(want))
	}
	for i := range want {
		if issues[0].Context[i] != want[i] {
			t.Fatalf("context[%d] = %q, want %q", i, issues[0].Context[i], want[i])
		}
	}
}

func TestAnalyzerMultipleFiles(t *testing.T) {
	analyzer := NewAnalyzer(AnalyzerConfig{
		Rules: []string{"allocation"},
	})

	fileA := testdataPath(t, "analyzer_multi_a.go")
	fileB := testdataPath(t, "analyzer_multi_b.go")
	issues, errs := analyzer.Analyze([]string{fileA, fileB})
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(issues) != 2 {
		t.Fatalf("got %d issues, want 2", len(issues))
	}

	seen := map[string]bool{
		fileA: false,
		fileB: false,
	}
	for _, issue := range issues {
		if _, ok := seen[issue.File]; ok {
			seen[issue.File] = true
		}
	}
	for file, ok := range seen {
		if !ok {
			t.Fatalf("missing issue from file: %q", file)
		}
	}
}

func TestAnalyzerHelpers(t *testing.T) {
	src := `package main

func nested(items [][]int) {
	for _, outer := range items {
		for _, inner := range outer {
			_ = inner
		}
	}
}

func appendInLoop(items []int) []int {
	var out []int
	for _, item := range items {
		out = append(out, item)
	}
	return out
}

func concatLoop(items []string) string {
	var s string
	for _, item := range items {
		s += item + "!"
	}
	return s
}
`

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "helpers.go", src, 0)
	if err != nil {
		t.Fatalf("failed to parse helpers src: %v", err)
	}

	nested := FindNestedRangeLoops(file, fset)
	if len(nested) != 1 {
		t.Fatalf("nested loops = %d, want 1", len(nested))
	}

	appends := FindAppendInLoop(file, fset)
	if len(appends) != 1 {
		t.Fatalf("append-in-loop = %d, want 1", len(appends))
	}

	concat := FindStringConcatInLoop(file, fset)
	if len(concat) != 1 {
		t.Fatalf("string concat in loop = %d, want 1", len(concat))
	}
}
