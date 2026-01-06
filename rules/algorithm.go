package rules

import (
	"go/ast"
	"go/token"
)

func init() {
	RegisterRule("algorithm", &NestedRangeRule{})
	RegisterRule("algorithm", &LinearSearchInLoopRule{})
}

// NestedRangeRule detects O(n²) nested range loops
type NestedRangeRule struct{}

func (r *NestedRangeRule) Name() string     { return "nested-range" }
func (r *NestedRangeRule) Category() string { return "algorithm" }

func (r *NestedRangeRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
	var issues []Issue

	ast.Inspect(file, func(n ast.Node) bool {
		outerRange, ok := n.(*ast.RangeStmt)
		if !ok {
			return true
		}

		outerVar := getRangeVar(outerRange)

		// Look for nested range inside this one
		ast.Inspect(outerRange.Body, func(inner ast.Node) bool {
			innerRange, ok := inner.(*ast.RangeStmt)
			if !ok {
				return true
			}

			// Check if inner loop iterates over same or related collection
			severity := SeverityMedium
			innerVar := getRangeVar(innerRange)

			// Same variable = definitely O(n²)
			if outerVar != "" && outerVar == innerVar {
				severity = SeverityHigh
			}

			pos := fset.Position(innerRange.Pos())
			issues = append(issues, Issue{
				Rule:     r.Name(),
				Category: r.Category(),
				Severity: severity,
				Line:     pos.Line,
				Column:   pos.Column,
				Message:  "Nested range loop detected - potential O(n²) complexity",
				Why:      "Nested iteration over collections scales quadratically. With 1000 items, this runs 1,000,000 times. With 10,000 items, 100,000,000 times.",
				Fix:      "Consider: (1) Building a map for O(1) lookups, (2) Using incremental/delta computation, (3) Sorting + binary search, (4) Breaking early when possible",
			})

			return false // Don't recurse into inner range
		})

		return true
	})

	return issues
}

func getRangeVar(r *ast.RangeStmt) string {
	if ident, ok := r.X.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}

// LinearSearchInLoopRule detects repeated linear searches that should use maps
type LinearSearchInLoopRule struct{}

func (r *LinearSearchInLoopRule) Name() string     { return "linear-search-in-loop" }
func (r *LinearSearchInLoopRule) Category() string { return "algorithm" }

func (r *LinearSearchInLoopRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
	var issues []Issue

	// Find slice/array contains checks in loops
	ast.Inspect(file, func(n ast.Node) bool {
		var loopBody *ast.BlockStmt
		switch stmt := n.(type) {
		case *ast.RangeStmt:
			loopBody = stmt.Body
		case *ast.ForStmt:
			loopBody = stmt.Body
		default:
			return true
		}

		if loopBody == nil {
			return true
		}

		// Look for inner range loops that look like linear searches
		ast.Inspect(loopBody, func(inner ast.Node) bool {
			innerRange, ok := inner.(*ast.RangeStmt)
			if !ok {
				return true
			}

			// Check if the inner loop body contains a comparison and break/return
			if containsSearchPattern(innerRange.Body) {
				pos := fset.Position(innerRange.Pos())
				issues = append(issues, Issue{
					Rule:     r.Name(),
					Category: r.Category(),
					Severity: SeverityMedium,
					Line:     pos.Line,
					Column:   pos.Column,
					Message:  "Linear search inside loop - consider using a map",
					Why:      "Searching a slice/array is O(n). Inside a loop, this becomes O(n*m). Building a map once is O(n), then lookups are O(1).",
					Fix:      "Build a map[key]value or map[key]struct{} before the loop for O(1) lookups",
				})
			}

			return true
		})

		return true
	})

	return issues
}

// containsSearchPattern checks if a loop body looks like a linear search
func containsSearchPattern(body *ast.BlockStmt) bool {
	hasComparison := false
	hasBreakOrReturn := false

	ast.Inspect(body, func(n ast.Node) bool {
		switch n.(type) {
		case *ast.BinaryExpr:
			// Has a comparison
			hasComparison = true
		case *ast.BranchStmt:
			hasBreakOrReturn = true
		case *ast.ReturnStmt:
			hasBreakOrReturn = true
		}
		return true
	})

	return hasComparison && hasBreakOrReturn
}
