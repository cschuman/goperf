package rules

import (
	"go/ast"
	"go/token"
)

func init() {
	RegisterRule("database", &SQLInLoopRule{})
	RegisterRule("database", &UnbatchedInsertRule{})
}

// SQLInLoopRule detects N+1 query patterns
type SQLInLoopRule struct{}

func (r *SQLInLoopRule) Name() string     { return "sql-in-loop" }
func (r *SQLInLoopRule) Category() string { return "database" }

func (r *SQLInLoopRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
	var issues []Issue

	results := FindSQLInLoop(file, fset)
	for _, result := range results {
		severity := SeverityHigh
		if result.Method == "Exec" || result.Method == "ExecContext" {
			severity = SeverityCritical // Writes in loops are worse
		}

		issues = append(issues, Issue{
			Rule:     r.Name(),
			Category: r.Category(),
			Severity: severity,
			Line:     result.Pos.Line,
			Column:   result.Pos.Column,
			Message:  "Database " + result.Method + "() called inside loop - N+1 query pattern",
			Why:      "Each iteration makes a separate database round-trip. With 100 items, that's 100 queries instead of 1. Network latency dominates, making this extremely slow.",
			Fix:      "Use batch operations: SELECT ... WHERE id IN (...), bulk INSERT, or collect IDs and query once outside the loop",
		})
	}

	return issues
}

// UnbatchedInsertRule detects single-row inserts that could be batched
type UnbatchedInsertRule struct{}

func (r *UnbatchedInsertRule) Name() string     { return "unbatched-insert" }
func (r *UnbatchedInsertRule) Category() string { return "database" }

func (r *UnbatchedInsertRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
	var issues []Issue

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

		// Look for Create/Insert/Save patterns (common ORM methods)
		ormMethods := map[string]bool{
			"Create": true,
			"Insert": true,
			"Save":   true,
		}

		ast.Inspect(loopBody, func(inner ast.Node) bool {
			call, ok := inner.(*ast.CallExpr)
			if !ok {
				return true
			}

			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			if ormMethods[sel.Sel.Name] {
				pos := fset.Position(call.Pos())
				issues = append(issues, Issue{
					Rule:     r.Name(),
					Category: r.Category(),
					Severity: SeverityHigh,
					Line:     pos.Line,
					Column:   pos.Column,
					Message:  "Single-row " + sel.Sel.Name + "() inside loop - consider batch insert",
					Why:      "Each insert is a separate transaction and network round-trip. Batch inserts are 10-100x faster for bulk data.",
					Fix:      "Collect items and use batch insert: db.CreateInBatches(), COPY, or multi-value INSERT",
				})
			}
			return true
		})

		return true
	})

	return issues
}
