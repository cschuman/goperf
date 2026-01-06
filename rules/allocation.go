package rules

import (
	"go/ast"
	"go/token"
)

func init() {
	RegisterRule("allocation", &UnpreallocatedSliceRule{})
	RegisterRule("allocation", &StringConcatInLoopRule{})
	RegisterRule("allocation", &MapWithoutSizeRule{})
}

// UnpreallocatedSliceRule detects slice append in loops without preallocation
type UnpreallocatedSliceRule struct{}

func (r *UnpreallocatedSliceRule) Name() string     { return "unpreallocated-slice" }
func (r *UnpreallocatedSliceRule) Category() string { return "allocation" }

func (r *UnpreallocatedSliceRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
	var issues []Issue

	results := FindAppendInLoop(file, fset)
	for _, result := range results {
		issues = append(issues, Issue{
			Rule:     r.Name(),
			Category: r.Category(),
			Severity: SeverityLow,
			Line:     result.Pos.Line,
			Column:   result.Pos.Column,
			Message:  "append() in loop without preallocation",
			Why:      "Slice grows dynamically, causing repeated memory allocations and copies. Each reallocation typically doubles capacity, wasting memory and CPU.",
			Fix:      "Preallocate with make([]T, 0, expectedSize) before the loop if size is known or estimable",
		})
	}

	return issues
}

// StringConcatInLoopRule detects string += concatenation in loops
type StringConcatInLoopRule struct{}

func (r *StringConcatInLoopRule) Name() string     { return "string-concat-loop" }
func (r *StringConcatInLoopRule) Category() string { return "allocation" }

func (r *StringConcatInLoopRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
	var issues []Issue

	positions := FindStringConcatInLoop(file, fset)
	for _, pos := range positions {
		issues = append(issues, Issue{
			Rule:     r.Name(),
			Category: r.Category(),
			Severity: SeverityMedium,
			Line:     pos.Line,
			Column:   pos.Column,
			Message:  "String concatenation in loop creates O(n²) allocations",
			Why:      "Strings are immutable in Go. Each += creates a new string, copying all previous content. Building a 1000-char string this way allocates ~500KB total.",
			Fix:      "Use strings.Builder: var b strings.Builder; for ... { b.WriteString(s) }; result := b.String()",
		})
	}

	return issues
}

// MapWithoutSizeRule detects map creation without size hint when size is known
type MapWithoutSizeRule struct{}

func (r *MapWithoutSizeRule) Name() string     { return "map-without-size" }
func (r *MapWithoutSizeRule) Category() string { return "allocation" }

func (r *MapWithoutSizeRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
	var issues []Issue

	ast.Inspect(file, func(n ast.Node) bool {
		// Look for patterns like:
		// m := make(map[K]V)
		// for _, item := range items { m[k] = v }

		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Rhs) != 1 {
			return true
		}

		call, ok := assign.Rhs[0].(*ast.CallExpr)
		if !ok {
			return true
		}

		ident, ok := call.Fun.(*ast.Ident)
		if !ok || ident.Name != "make" {
			return true
		}

		if len(call.Args) < 1 {
			return true
		}

		// Check if it's a map type without size
		_, isMap := call.Args[0].(*ast.MapType)
		if !isMap || len(call.Args) > 1 {
			return true // Has size hint or not a map
		}

		// Check if there's a loop that populates it nearby
		// This is a simplified heuristic
		pos := fset.Position(call.Pos())
		issues = append(issues, Issue{
			Rule:     r.Name(),
			Category: r.Category(),
			Severity: SeverityLow,
			Line:     pos.Line,
			Column:   pos.Column,
			Message:  "Map created without size hint",
			Why:      "Maps without size hints start small and rehash as they grow. If you know the approximate size, providing it avoids rehashing overhead.",
			Fix:      "Use make(map[K]V, expectedSize) if the size is known or estimable",
		})

		return true
	})

	return issues
}
