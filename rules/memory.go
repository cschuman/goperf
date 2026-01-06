package rules

import (
	"go/ast"
	"go/token"
)

func init() {
	RegisterRule("memory", &PprofInHotPathRule{})
	RegisterRule("memory", &LargeStructCopyRule{})
	RegisterRule("memory", &EscapeToHeapRule{})
}

// PprofInHotPathRule detects pprof calls in hot paths
type PprofInHotPathRule struct{}

func (r *PprofInHotPathRule) Name() string     { return "pprof-in-hot-path" }
func (r *PprofInHotPathRule) Category() string { return "memory" }

func (r *PprofInHotPathRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
	issues := make([]Issue, 0, 4)

	pprofFuncs := map[string]bool{
		"WriteHeapProfile":    true,
		"StartCPUProfile":     true,
		"StopCPUProfile":      true,
		"Lookup":              true,
		"WriteTo":             true,
	}

	ast.Inspect(file, func(n ast.Node) bool {
		// Check for pprof calls in loops
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

		ast.Inspect(loopBody, func(inner ast.Node) bool {
			call, ok := inner.(*ast.CallExpr)
			if !ok {
				return true
			}

			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			// Check for pprof.X or runtime/pprof calls
			if ident, ok := sel.X.(*ast.Ident); ok {
				if ident.Name == "pprof" && pprofFuncs[sel.Sel.Name] {
					pos := fset.Position(call.Pos())
					issues = append(issues, Issue{
						Rule:     r.Name(),
						Category: r.Category(),
						Severity: SeverityHigh,
						Line:     pos.Line,
						Column:   pos.Column,
						Message:  "pprof." + sel.Sel.Name + "() called in loop - significant overhead",
						Why:      "Profiling operations are expensive and should not be called repeatedly. They're meant for sampling, not continuous collection.",
						Fix:      "Move profiling outside the loop, or use sampling: if rand.Intn(1000) == 0 { profile() }",
					})
				}
			}

			return true
		})

		return true
	})

	// Also check for pprof in HTTP handlers (common mistake)
	ast.Inspect(file, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok {
			return true
		}

		// Check if this looks like an HTTP handler
		if !isHTTPHandler(funcDecl) {
			return true
		}

		ast.Inspect(funcDecl.Body, func(inner ast.Node) bool {
			call, ok := inner.(*ast.CallExpr)
			if !ok {
				return true
			}

			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			if ident, ok := sel.X.(*ast.Ident); ok {
				if ident.Name == "pprof" && pprofFuncs[sel.Sel.Name] {
					pos := fset.Position(call.Pos())
					issues = append(issues, Issue{
						Rule:     r.Name(),
						Category: r.Category(),
						Severity: SeverityMedium,
						Line:     pos.Line,
						Column:   pos.Column,
						Message:  "pprof." + sel.Sel.Name + "() in HTTP handler - consider using /debug/pprof endpoints",
						Why:      "Manual pprof calls in handlers add latency. Use net/http/pprof endpoints for on-demand profiling.",
						Fix:      "Import _ \"net/http/pprof\" and use /debug/pprof/* endpoints instead",
					})
				}
			}

			return true
		})

		return true
	})

	return issues
}

// LargeStructCopyRule detects passing large structs by value
type LargeStructCopyRule struct{}

func (r *LargeStructCopyRule) Name() string     { return "large-struct-copy" }
func (r *LargeStructCopyRule) Category() string { return "memory" }

func (r *LargeStructCopyRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
	issues := make([]Issue, 0, 4)

	// Find struct definitions and estimate their size
	structSizes := estimateStructSizes(file)

	ast.Inspect(file, func(n ast.Node) bool {
		// Check function parameters
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok {
			return true
		}

		if funcDecl.Type.Params == nil {
			return true
		}

		for _, param := range funcDecl.Type.Params.List {
			// Skip pointer types
			if _, isPtr := param.Type.(*ast.StarExpr); isPtr {
				continue
			}

			typeName := getTypeName(param.Type)
			if size, ok := structSizes[typeName]; ok && size > 64 {
				// Large struct passed by value
				pos := fset.Position(param.Pos())
				issues = append(issues, Issue{
					Rule:     r.Name(),
					Category: r.Category(),
					Severity: SeverityMedium,
					Line:     pos.Line,
					Column:   pos.Column,
					Message:  "Large struct '" + typeName + "' (~" + itoa(size) + " bytes) passed by value",
					Why:      "Passing large structs by value copies all fields on each call. This wastes CPU and memory bandwidth.",
					Fix:      "Pass by pointer: func f(s *" + typeName + ") instead of func f(s " + typeName + ")",
				})
			}
		}

		// Check for large struct copies in loops
		if funcDecl.Body == nil {
			return true
		}

		ast.Inspect(funcDecl.Body, func(inner ast.Node) bool {
			var loopBody *ast.BlockStmt
			switch stmt := inner.(type) {
			case *ast.RangeStmt:
				loopBody = stmt.Body
				// Check range value - copying struct on each iteration
				if stmt.Value != nil {
					if ident, ok := stmt.Value.(*ast.Ident); ok && ident.Name != "_" {
						// This copies the value on each iteration
						// We'd need type info to know if it's a large struct
					}
				}
			case *ast.ForStmt:
				loopBody = stmt.Body
			default:
				return true
			}

			if loopBody == nil {
				return true
			}

			// Check for assignments that copy large structs
			ast.Inspect(loopBody, func(stmt ast.Node) bool {
				assign, ok := stmt.(*ast.AssignStmt)
				if !ok {
					return true
				}

				for _, rhs := range assign.Rhs {
					typeName := getTypeName(rhs)
					if size, ok := structSizes[typeName]; ok && size > 64 {
						pos := fset.Position(assign.Pos())
						issues = append(issues, Issue{
							Rule:     r.Name(),
							Category: r.Category(),
							Severity: SeverityMedium,
							Line:     pos.Line,
							Column:   pos.Column,
							Message:  "Large struct copy in loop - consider using pointer",
							Why:      "Copying a ~" + itoa(size) + " byte struct on each iteration adds significant overhead.",
							Fix:      "Use pointer to avoid copy, or access fields directly without intermediate variable",
						})
					}
				}

				return true
			})

			return true
		})

		return true
	})

	return issues
}

// EscapeToHeapRule detects patterns that likely cause heap escapes
type EscapeToHeapRule struct{}

func (r *EscapeToHeapRule) Name() string     { return "escape-to-heap" }
func (r *EscapeToHeapRule) Category() string { return "memory" }

func (r *EscapeToHeapRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
	issues := make([]Issue, 0, 4)

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

		// Find &x patterns in loops that likely escape
		ast.Inspect(loopBody, func(inner ast.Node) bool {
			// Check for &localVar being stored or passed
			unary, ok := inner.(*ast.UnaryExpr)
			if !ok || unary.Op != token.AND {
				return true
			}

			// Check if this is part of an append or map assignment
			// These typically cause the pointed value to escape

			// For now, flag pointer creation in loops as informational
			// since we can't do full escape analysis without type info

			return true
		})

		return true
	})

	return issues
}

// Helper functions are now in helpers.go
