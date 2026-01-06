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
// Now smarter: recognizes map-based optimizations
type NestedRangeRule struct{}

func (r *NestedRangeRule) Name() string     { return "nested-range" }
func (r *NestedRangeRule) Category() string { return "algorithm" }

func (r *NestedRangeRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
	issues := make([]Issue, 0, 4)

	ast.Inspect(file, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok {
			return true
		}

		if funcDecl.Body == nil {
			return true
		}

		// Find maps that are populated before loops (lookup optimization)
		lookupMaps := findLookupMaps(funcDecl.Body)

		ast.Inspect(funcDecl.Body, func(inner ast.Node) bool {
			outerRange, ok := inner.(*ast.RangeStmt)
			if !ok {
				return true
			}

			outerVar := getRangeVar(outerRange)

			// Look for nested range inside this one
			ast.Inspect(outerRange.Body, func(innerNode ast.Node) bool {
				innerRange, ok := innerNode.(*ast.RangeStmt)
				if !ok {
					return true
				}

				innerVar := getRangeVar(innerRange)

				// Check if the inner loop uses a map lookup instead of linear search
				// This is O(n*m) where m is O(1) = O(n), not O(n²)
				if usesMapLookup(innerRange.Body, lookupMaps) {
					// This is actually optimized - don't flag
					return false
				}

				// Check if inner loop iterates over same or related collection
				severity := SeverityMedium

				// Same variable = definitely O(n²)
				if outerVar != "" && outerVar == innerVar {
					severity = SeverityHigh
				}

				// Check if the loop body is trivial (few operations)
				if isTrivalLoopBody(innerRange.Body) {
					if severity == SeverityHigh {
						severity = SeverityMedium
					} else {
						severity = SeverityLow
					}
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

		return true
	})

	return issues
}

// findLookupMaps finds maps that are populated before being used as lookups
func findLookupMaps(body *ast.BlockStmt) map[string]bool {
	maps := make(map[string]bool)

	// Find map declarations
	ast.Inspect(body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}

		for i, rhs := range assign.Rhs {
			call, ok := rhs.(*ast.CallExpr)
			if !ok {
				continue
			}

			ident, ok := call.Fun.(*ast.Ident)
			if !ok || ident.Name != "make" {
				continue
			}

			if len(call.Args) < 1 {
				continue
			}

			// Check if it's a map type
			if _, isMap := call.Args[0].(*ast.MapType); isMap {
				if i < len(assign.Lhs) {
					if lhsIdent, ok := assign.Lhs[i].(*ast.Ident); ok {
						maps[lhsIdent.Name] = true
					}
				}
			}
		}

		return true
	})

	return maps
}

// usesMapLookup checks if a loop body uses map lookup instead of iteration
func usesMapLookup(body *ast.BlockStmt, maps map[string]bool) bool {
	usesMap := false

	ast.Inspect(body, func(n ast.Node) bool {
		// Look for map[key] access pattern
		indexExpr, ok := n.(*ast.IndexExpr)
		if !ok {
			return true
		}

		if ident, ok := indexExpr.X.(*ast.Ident); ok {
			if maps[ident.Name] {
				usesMap = true
				return false
			}
		}

		return true
	})

	return usesMap
}

// isTrivalLoopBody checks if loop body is very simple (few statements)
func isTrivalLoopBody(body *ast.BlockStmt) bool {
	if body == nil {
		return true
	}
	// Consider trivial if <= 3 statements
	return len(body.List) <= 3
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
	issues := make([]Issue, 0, 4)

	ast.Inspect(file, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok {
			return true
		}

		if funcDecl.Body == nil {
			return true
		}

		// Find existing lookup maps
		lookupMaps := findLookupMaps(funcDecl.Body)

		// Find slice/array contains checks in loops
		ast.Inspect(funcDecl.Body, func(inner ast.Node) bool {
			var loopBody *ast.BlockStmt
			switch stmt := inner.(type) {
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
			ast.Inspect(loopBody, func(stmt ast.Node) bool {
				innerRange, ok := stmt.(*ast.RangeStmt)
				if !ok {
					return true
				}

				// Check if already using map lookup
				if usesMapLookup(innerRange.Body, lookupMaps) {
					return true // Already optimized
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

		return true
	})

	return issues
}

// containsSearchPattern checks if a loop body looks like a linear search
func containsSearchPattern(body *ast.BlockStmt) bool {
	hasComparison := false
	hasBreakOrReturn := false

	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.BinaryExpr:
			// Look for equality comparison (== or !=)
			if node.Op == token.EQL || node.Op == token.NEQ {
				hasComparison = true
			}
		case *ast.BranchStmt:
			if node.Tok == token.BREAK {
				hasBreakOrReturn = true
			}
		case *ast.ReturnStmt:
			hasBreakOrReturn = true
		}
		return true
	})

	return hasComparison && hasBreakOrReturn
}
