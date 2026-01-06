package rules

import (
	"go/ast"
	"go/token"
)

func init() {
	RegisterRule("database", &SQLInLoopRule{})
	RegisterRule("database", &UnbatchedInsertRule{})
}

// SQLInLoopRule detects N+1 query patterns with smart prepared statement detection
type SQLInLoopRule struct{}

func (r *SQLInLoopRule) Name() string     { return "sql-in-loop" }
func (r *SQLInLoopRule) Category() string { return "database" }

func (r *SQLInLoopRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
	var issues []Issue

	// Track prepared statements declared in the current scope
	preparedStmts := findPreparedStatements(file)

	// Track transaction variables
	txVars := findTransactionVariables(file)

	ast.Inspect(file, func(n ast.Node) bool {
		var loopBody *ast.BlockStmt
		var loopNode ast.Node
		switch stmt := n.(type) {
		case *ast.RangeStmt:
			loopBody = stmt.Body
			loopNode = stmt
		case *ast.ForStmt:
			loopBody = stmt.Body
			loopNode = stmt
		default:
			return true
		}

		if loopBody == nil {
			return true
		}

		// Check for bounded loops (small, known iteration count)
		loopBound := getLoopBound(loopNode)

		// Find SQL method calls in the loop body
		ast.Inspect(loopBody, func(inner ast.Node) bool {
			call, ok := inner.(*ast.CallExpr)
			if !ok {
				return true
			}

			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			sqlMethods := map[string]bool{
				"Query": true, "QueryRow": true, "Exec": true,
				"QueryRowContext": true, "QueryContext": true, "ExecContext": true,
				"Get": true, "Select": true,
			}

			if !sqlMethods[sel.Sel.Name] {
				return true
			}

			// Get receiver variable name
			receiver := getReceiverName(sel.X)

			// Check if using a prepared statement
			if preparedStmts[receiver] {
				// Using prepared statement - much lower severity
				// This is the idiomatic Go batch pattern
				return true // Skip - not a real issue
			}

			// Determine severity based on context
			severity := SeverityHigh
			why := "Each iteration makes a separate database round-trip. With 100 items, that's 100 queries instead of 1. Network latency dominates."
			fix := "Use batch operations: SELECT ... WHERE id IN (...), bulk INSERT, or collect IDs and query once outside the loop"

			// Writes are worse than reads
			if sel.Sel.Name == "Exec" || sel.Sel.Name == "ExecContext" {
				severity = SeverityCritical
			}

			// Transaction context reduces severity slightly (batched round-trips)
			if txVars[receiver] {
				if severity == SeverityCritical {
					severity = SeverityHigh
				} else {
					severity = SeverityMedium
				}
				why = "Each iteration executes separately within the transaction. While batched at commit, individual executions still have overhead."
				fix = "Consider using prepared statements with tx.Prepare() for better performance, or batch the operations"
			}

			// Small bounded loops are less severe
			if loopBound > 0 && loopBound <= 10 {
				if severity == SeverityCritical {
					severity = SeverityHigh
				} else if severity == SeverityHigh {
					severity = SeverityMedium
				} else {
					severity = SeverityLow
				}
				why = "Loop appears bounded to a small number of iterations. Still incurs round-trip overhead but impact is limited."
			}

			pos := fset.Position(call.Pos())
			issues = append(issues, Issue{
				Rule:     r.Name(),
				Category: r.Category(),
				Severity: severity,
				Line:     pos.Line,
				Column:   pos.Column,
				Message:  "Database " + sel.Sel.Name + "() called inside loop - N+1 query pattern",
				Why:      why,
				Fix:      fix,
			})

			return true
		})

		return true
	})

	return issues
}

// findPreparedStatements finds variables that hold prepared statements
func findPreparedStatements(file *ast.File) map[string]bool {
	stmts := make(map[string]bool)

	ast.Inspect(file, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}

		// Look for stmt, err := db.Prepare(...) or tx.Prepare(...)
		for i, rhs := range assign.Rhs {
			call, ok := rhs.(*ast.CallExpr)
			if !ok {
				continue
			}

			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				continue
			}

			if sel.Sel.Name == "Prepare" || sel.Sel.Name == "PrepareContext" {
				if i < len(assign.Lhs) {
					if ident, ok := assign.Lhs[i].(*ast.Ident); ok {
						stmts[ident.Name] = true
					}
				}
			}
		}

		return true
	})

	return stmts
}

// findTransactionVariables finds variables that hold database transactions
func findTransactionVariables(file *ast.File) map[string]bool {
	txVars := make(map[string]bool)

	ast.Inspect(file, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}

		// Look for tx, err := db.Begin() or db.BeginTx(...)
		for i, rhs := range assign.Rhs {
			call, ok := rhs.(*ast.CallExpr)
			if !ok {
				continue
			}

			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				continue
			}

			if sel.Sel.Name == "Begin" || sel.Sel.Name == "BeginTx" {
				if i < len(assign.Lhs) {
					if ident, ok := assign.Lhs[i].(*ast.Ident); ok {
						txVars[ident.Name] = true
					}
				}
			}
		}

		return true
	})

	return txVars
}

// getReceiverName extracts the receiver variable name from a selector expression
func getReceiverName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		// s.db -> return the final selector
		return e.Sel.Name
	}
	return ""
}

// getLoopBound tries to determine if a loop has a small, known bound
func getLoopBound(loopNode ast.Node) int {
	switch stmt := loopNode.(type) {
	case *ast.ForStmt:
		// for i := 0; i < N; i++ pattern
		if stmt.Cond != nil {
			if binExpr, ok := stmt.Cond.(*ast.BinaryExpr); ok {
				if binExpr.Op == token.LSS || binExpr.Op == token.LEQ {
					if lit, ok := binExpr.Y.(*ast.BasicLit); ok && lit.Kind == token.INT {
						// Parse the integer
						var bound int
						for _, c := range lit.Value {
							if c >= '0' && c <= '9' {
								bound = bound*10 + int(c-'0')
							}
						}
						return bound
					}
				}
			}
		}
	case *ast.RangeStmt:
		// Check if ranging over a small literal
		if compLit, ok := stmt.X.(*ast.CompositeLit); ok {
			return len(compLit.Elts)
		}
	}
	return -1 // Unknown bound
}

// UnbatchedInsertRule detects single-row inserts that could be batched
type UnbatchedInsertRule struct{}

func (r *UnbatchedInsertRule) Name() string     { return "unbatched-insert" }
func (r *UnbatchedInsertRule) Category() string { return "database" }

func (r *UnbatchedInsertRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
	var issues []Issue

	ast.Inspect(file, func(n ast.Node) bool {
		var loopBody *ast.BlockStmt
		var loopNode ast.Node
		switch stmt := n.(type) {
		case *ast.RangeStmt:
			loopBody = stmt.Body
			loopNode = stmt
		case *ast.ForStmt:
			loopBody = stmt.Body
			loopNode = stmt
		default:
			return true
		}

		if loopBody == nil {
			return true
		}

		loopBound := getLoopBound(loopNode)

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
				severity := SeverityHigh
				if loopBound > 0 && loopBound <= 10 {
					severity = SeverityMedium
				}

				pos := fset.Position(call.Pos())
				issues = append(issues, Issue{
					Rule:     r.Name(),
					Category: r.Category(),
					Severity: severity,
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
