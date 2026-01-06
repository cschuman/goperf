package fixer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsValidIdentifier(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"cancel", true},
		{"Cancel", true},
		{"_cancel", true},
		{"cancel1", true},
		{"my_cancel_func", true},
		{"", false},
		{"1cancel", false},
		{"-cancel", false},
		{"cancel-func", false},
		{"cancel func", false},
		{"cancel()", false},
		{string(make([]byte, 101)), false}, // too long
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isValidIdentifier(tt.input)
			if got != tt.want {
				t.Errorf("isValidIdentifier(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestGetLine(t *testing.T) {
	lines := []string{"line1", "line2", "line3"}

	tests := []struct {
		lineNum int
		want    string
	}{
		{1, "line1"},
		{2, "line2"},
		{3, "line3"},
		{0, ""},  // out of bounds
		{4, ""},  // out of bounds
		{-1, ""}, // negative
	}

	for _, tt := range tests {
		got := getLine(lines, tt.lineNum)
		if got != tt.want {
			t.Errorf("getLine(lines, %d) = %q, want %q", tt.lineNum, got, tt.want)
		}
	}
}

func TestValidatePathForWrite(t *testing.T) {
	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}

	// Create a temp file in current directory for testing
	tmpFile, err := os.CreateTemp(cwd, "test-*.go")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "valid file in cwd",
			path:    tmpFile.Name(),
			wantErr: false,
		},
		{
			name:    "path outside cwd",
			path:    "/etc/passwd",
			wantErr: true,
		},
		{
			name:    "path traversal attempt",
			path:    filepath.Join(cwd, "..", "..", "etc", "passwd"),
			wantErr: true,
		},
		{
			name:    "relative path outside",
			path:    "../../../etc/passwd",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePathForWrite(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePathForWrite(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

func TestNewFixer(t *testing.T) {
	f := NewFixer(true, true)
	if f == nil {
		t.Fatal("NewFixer returned nil")
	}
	if !f.DryRun {
		t.Error("DryRun should be true")
	}
	if !f.Verbose {
		t.Error("Verbose should be true")
	}

	f2 := NewFixer(false, false)
	if f2.DryRun {
		t.Error("DryRun should be false")
	}
	if f2.Verbose {
		t.Error("Verbose should be false")
	}
}

func TestGenerateDiff(t *testing.T) {
	fixes := []Fix{
		{
			File:     "test.go",
			Line:     10,
			Original: "old code",
			Fixed:    "new code",
			Rule:     "test-rule",
		},
	}

	diff := GenerateDiff(fixes)
	if diff == "" {
		t.Error("GenerateDiff returned empty string")
	}

	// Check diff format
	expectedContains := []string{
		"--- a/test.go",
		"+++ b/test.go",
		"-old code",
		"+new code",
	}

	for _, expected := range expectedContains {
		if !contains(diff, expected) {
			t.Errorf("diff missing %q", expected)
		}
	}
}

func TestGenerateDiff_EmptyFix(t *testing.T) {
	fixes := []Fix{
		{
			File:     "test.go",
			Line:     10,
			Original: "old code",
			Fixed:    "", // No fix available
			Rule:     "test-rule",
		},
	}

	diff := GenerateDiff(fixes)
	if diff != "" {
		t.Error("GenerateDiff should return empty string for fixes without Fixed content")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
