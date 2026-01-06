package rules

import (
	"testing"
)

func TestIgnoreSet_LineIgnore(t *testing.T) {
	src := []byte(`package main

func foo() {
	// perf:ignore
	for _, item := range items {
		db.Exec(query, item) // This should be ignored
	}
}
`)
	is := NewIgnoreSet(src)

	// Line 4 is the comment, line 5 is the for loop - both should be ignored
	if !is.ShouldIgnore(4, "sql-in-loop") {
		t.Error("Line 4 should be ignored")
	}
	if !is.ShouldIgnore(5, "sql-in-loop") {
		t.Error("Line 5 should be ignored")
	}
	// Line 6 should NOT be ignored (only next line after comment)
	if is.ShouldIgnore(7, "sql-in-loop") {
		t.Error("Line 7 should NOT be ignored")
	}
}

func TestIgnoreSet_SameLineIgnore(t *testing.T) {
	src := []byte(`package main

func foo() {
	db.Exec(query, item) // perf:ignore
}
`)
	is := NewIgnoreSet(src)

	// Line 4 has the ignore comment on the same line
	if !is.ShouldIgnore(4, "sql-in-loop") {
		t.Error("Line 4 should be ignored (same-line comment)")
	}
}

func TestIgnoreSet_BlockIgnore(t *testing.T) {
	src := []byte(`package main

func foo() {
	// perf:ignore-start
	for _, item := range items {
		db.Exec(query, item)
	}
	for _, other := range others {
		db.Query(q, other)
	}
	// perf:ignore-end

	// This should NOT be ignored
	db.Exec(query, x)
}
`)
	is := NewIgnoreSet(src)

	// Lines 4-11 should be ignored (inside block)
	for line := 4; line <= 11; line++ {
		if !is.ShouldIgnore(line, "sql-in-loop") {
			t.Errorf("Line %d should be ignored (inside block)", line)
		}
	}

	// Line 14 should NOT be ignored (after block)
	if is.ShouldIgnore(14, "sql-in-loop") {
		t.Error("Line 14 should NOT be ignored (after block)")
	}
}

func TestIgnoreSet_SpecificRule(t *testing.T) {
	src := []byte(`package main

func foo() {
	// perf:ignore sql-in-loop
	for _, item := range items {
		db.Exec(query, item) // ignored
		result = append(result, item) // NOT ignored - different rule
	}
}
`)
	is := NewIgnoreSet(src)

	// sql-in-loop should be ignored
	if !is.ShouldIgnore(5, "sql-in-loop") {
		t.Error("sql-in-loop should be ignored on line 5")
	}

	// append-in-loop should NOT be ignored (different rule)
	if is.ShouldIgnore(5, "append-in-loop") {
		t.Error("append-in-loop should NOT be ignored on line 5")
	}
}

func TestIgnoreSet_NoIgnore(t *testing.T) {
	src := []byte(`package main

func foo() {
	// This is a normal comment
	db.Exec(query, item)
}
`)
	is := NewIgnoreSet(src)

	if is.ShouldIgnore(5, "sql-in-loop") {
		t.Error("Line 5 should NOT be ignored")
	}
}

func TestParseIgnoreComment(t *testing.T) {
	tests := []struct {
		line      string
		wantRule  string
		wantFound bool
	}{
		{"// perf:ignore", "", true},
		{"// perf:ignore sql-in-loop", "sql-in-loop", true},
		{"  // perf:ignore", "", true},
		{"code() // perf:ignore", "", true},
		{"// perf:ignore-start", "", false}, // Should not match block markers
		{"// perf:ignore-end", "", false},
		{"// normal comment", "", false},
		{"no comment", "", false},
	}

	for _, tt := range tests {
		rule, found := parseIgnoreComment(tt.line)
		if found != tt.wantFound {
			t.Errorf("parseIgnoreComment(%q): found = %v, want %v", tt.line, found, tt.wantFound)
		}
		if rule != tt.wantRule {
			t.Errorf("parseIgnoreComment(%q): rule = %q, want %q", tt.line, rule, tt.wantRule)
		}
	}
}
