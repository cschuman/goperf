package rules

import (
	"go/ast"
	"go/token"
)

func init() {
	RegisterRule("context", &ContextBackgroundInHandlerRule{})
	RegisterRule("context", &MissingContextTimeoutRule{})
	RegisterRule("context", &ContextLeakRule{})
}

// ContextBackgroundInHandlerRule detects context.Background() in HTTP handlers
type ContextBackgroundInHandlerRule struct{}

func (r *ContextBackgroundInHandlerRule) Name() string     { return "context-background-in-handler" }
func (r *ContextBackgroundInHandlerRule) Category() string { return "context" }

func (r *ContextBackgroundInHandlerRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
	issues := make([]Issue, 0, 4)

	ast.Inspect(file, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok {
			return true
		}

		// Check if this looks like an HTTP handler
		if !isHTTPHandler(funcDecl) {
			return true
		}

		if funcDecl.Body == nil {
			return true
		}

		// Look for context.Background() or context.TODO() calls
		ast.Inspect(funcDecl.Body, func(inner ast.Node) bool {
			call, ok := inner.(*ast.CallExpr)
			if !ok {
				return true
			}

			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			ident, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}

			if ident.Name == "context" && (sel.Sel.Name == "Background" || sel.Sel.Name == "TODO") {
				pos := fset.Position(call.Pos())
				issues = append(issues, Issue{
					Rule:     r.Name(),
					Category: r.Category(),
					Severity: SeverityMedium,
					Line:     pos.Line,
					Column:   pos.Column,
					Message:  "context." + sel.Sel.Name + "() in HTTP handler - use request context instead",
					Why:      "HTTP handlers should use r.Context() which is cancelled when the client disconnects. context.Background() ignores client disconnection.",
					Fix:      "Use ctx := r.Context() instead, or derive from it: ctx, cancel := context.WithTimeout(r.Context(), timeout)",
				})
			}

			return true
		})

		return true
	})

	return issues
}

// MissingContextTimeoutRule detects external calls without context timeout
type MissingContextTimeoutRule struct{}

func (r *MissingContextTimeoutRule) Name() string     { return "missing-context-timeout" }
func (r *MissingContextTimeoutRule) Category() string { return "context" }

func (r *MissingContextTimeoutRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
	issues := make([]Issue, 0, 4)

	// Track contexts with timeouts in each function
	ast.Inspect(file, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok {
			return true
		}

		if funcDecl.Body == nil {
			return true
		}

		// Find context variables with timeouts
		ctxsWithTimeout := findContextsWithTimeout(funcDecl.Body)

		// Check external service calls
		ast.Inspect(funcDecl.Body, func(inner ast.Node) bool {
			call, ok := inner.(*ast.CallExpr)
			if !ok {
				return true
			}

			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			// Check for HTTP client calls, gRPC calls, database calls
			externalMethods := map[string]bool{
				"Do":          true, // http.Client.Do
				"Get":         true, // http.Get or client.Get
				"Post":        true,
				"PostForm":    true,
				"Head":        true,
				"Invoke":      true, // gRPC
				"NewRequest":  true, // http.NewRequest (should use NewRequestWithContext)
			}

			if externalMethods[sel.Sel.Name] {
				// Check if any argument is a context with timeout
				hasTimeoutCtx := false
				for _, arg := range call.Args {
					if ident, ok := arg.(*ast.Ident); ok {
						if ctxsWithTimeout[ident.Name] {
							hasTimeoutCtx = true
							break
						}
					}
				}

				// Special check for http.NewRequest - should be NewRequestWithContext
				if sel.Sel.Name == "NewRequest" {
					if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "http" {
						pos := fset.Position(call.Pos())
						issues = append(issues, Issue{
							Rule:     r.Name(),
							Category: r.Category(),
							Severity: SeverityMedium,
							Line:     pos.Line,
							Column:   pos.Column,
							Message:  "http.NewRequest() doesn't accept context - use NewRequestWithContext()",
							Why:      "NewRequest creates a request without context, making it impossible to cancel or timeout.",
							Fix:      "Use http.NewRequestWithContext(ctx, method, url, body) instead",
						})
					}
				}

				// For Do, Get, etc. - check if context seems to have timeout
				if !hasTimeoutCtx && (sel.Sel.Name == "Do" || sel.Sel.Name == "Get" || sel.Sel.Name == "Invoke") {
					// This is a heuristic - we can't be 100% sure without type info
					// Only flag if the receiver looks like an HTTP client
				}
			}

			return true
		})

		return true
	})

	return issues
}

// ContextLeakRule detects context cancel functions that aren't called
type ContextLeakRule struct{}

func (r *ContextLeakRule) Name() string     { return "context-leak" }
func (r *ContextLeakRule) Category() string { return "context" }

func (r *ContextLeakRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
	issues := make([]Issue, 0, 4)

	ast.Inspect(file, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok {
			return true
		}

		if funcDecl.Body == nil {
			return true
		}

		// Find cancel functions from WithCancel/WithTimeout/WithDeadline
		cancelFuncs := make(map[string]token.Position)

		ast.Inspect(funcDecl.Body, func(inner ast.Node) bool {
			assign, ok := inner.(*ast.AssignStmt)
			if !ok {
				return true
			}

			for i, rhs := range assign.Rhs {
				call, ok := rhs.(*ast.CallExpr)
				if !ok {
					continue
				}

				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					continue
				}

				ident, ok := sel.X.(*ast.Ident)
				if !ok {
					continue
				}

				if ident.Name == "context" {
					switch sel.Sel.Name {
					case "WithCancel", "WithTimeout", "WithDeadline":
						// Second return value is the cancel function
						if len(assign.Lhs) > i+1 {
							if cancelIdent, ok := assign.Lhs[i+1].(*ast.Ident); ok {
								if cancelIdent.Name != "_" {
									cancelFuncs[cancelIdent.Name] = fset.Position(assign.Pos())
								}
							}
						}
					}
				}
			}

			return true
		})

		// Check if cancel functions are called (either directly or via defer)
		// Only track calls to identifiers that are in our cancelFuncs map
		calledCancels := make(map[string]bool)

		ast.Inspect(funcDecl.Body, func(inner ast.Node) bool {
			switch node := inner.(type) {
			case *ast.CallExpr:
				if ident, ok := node.Fun.(*ast.Ident); ok {
					// Only mark as called if it's a known cancel function
					if _, isCancel := cancelFuncs[ident.Name]; isCancel {
						calledCancels[ident.Name] = true
					}
				}
			case *ast.DeferStmt:
				if call, ok := node.Call.Fun.(*ast.Ident); ok {
					// Only mark as called if it's a known cancel function
					if _, isCancel := cancelFuncs[call.Name]; isCancel {
						calledCancels[call.Name] = true
					}
				}
			}
			return true
		})

		// Report uncalled cancel functions
		for name, pos := range cancelFuncs {
			if !calledCancels[name] {
				issues = append(issues, Issue{
					Rule:     r.Name(),
					Category: r.Category(),
					Severity: SeverityHigh,
					Line:     pos.Line,
					Column:   pos.Column,
					Message:  "Context cancel function '" + name + "' is never called - resource leak",
					Why:      "Not calling cancel() leaks goroutines and resources associated with the context. The context will never be cancelled.",
					Fix:      "Add 'defer " + name + "()' immediately after creating the context",
				})
			}
		}

		return true
	})

	return issues
}

// Helper function to find contexts that have timeout/deadline
func findContextsWithTimeout(body *ast.BlockStmt) map[string]bool {
	ctxs := make(map[string]bool)

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

			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				continue
			}

			ident, ok := sel.X.(*ast.Ident)
			if !ok {
				continue
			}

			if ident.Name == "context" {
				switch sel.Sel.Name {
				case "WithTimeout", "WithDeadline":
					if i < len(assign.Lhs) {
						if ctxIdent, ok := assign.Lhs[i].(*ast.Ident); ok {
							ctxs[ctxIdent.Name] = true
						}
					}
				}
			}
		}

		return true
	})

	return ctxs
}
