package rules

import (
	"go/ast"
	"go/token"
)

func init() {
	RegisterRule("allocation", &ErrorWrapInLoopRule{})
	RegisterRule("allocation", &FmtErrorfInLoopRule{})
}

// ErrorWrapInLoopRule detects error wrapping in hot paths
type ErrorWrapInLoopRule struct{}

func (r *ErrorWrapInLoopRule) Name() string     { return "error-wrap-in-loop" }
func (r *ErrorWrapInLoopRule) Category() string { return "allocation" }

func (r *ErrorWrapInLoopRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
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

		ast.Inspect(loopBody, func(inner ast.Node) bool {
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

			// Check for errors.Wrap, errors.Wrapf, fmt.Errorf with %w
			if ident.Name == "errors" && (sel.Sel.Name == "Wrap" || sel.Sel.Name == "Wrapf") {
				pos := fset.Position(call.Pos())
				issues = append(issues, Issue{
					Rule:     r.Name(),
					Category: r.Category(),
					Severity: SeverityLow,
					Line:     pos.Line,
					Column:   pos.Column,
					Message:  "errors." + sel.Sel.Name + "() in loop - creates new error each iteration",
					Why:      "Error wrapping allocates a new error struct each time. In hot loops, this adds GC pressure.",
					Fix:      "Consider: (1) Pre-allocate sentinel errors, (2) Only wrap once outside loop, (3) Use error code patterns",
				})
			}

			// Check for fmt.Errorf
			if ident.Name == "fmt" && sel.Sel.Name == "Errorf" {
				pos := fset.Position(call.Pos())
				issues = append(issues, Issue{
					Rule:     r.Name(),
					Category: r.Category(),
					Severity: SeverityLow,
					Line:     pos.Line,
					Column:   pos.Column,
					Message:  "fmt.Errorf() in loop - allocates on each iteration",
					Why:      "fmt.Errorf allocates a new error and formats strings each time. In hot loops, use pre-defined errors.",
					Fix:      "Define errors at package level: var ErrInvalidItem = errors.New(\"invalid item\"), then use them in the loop",
				})
			}

			return true
		})

		return true
	})

	return issues
}

// FmtErrorfInLoopRule specifically detects fmt.Errorf with %w verb (error wrapping)
type FmtErrorfInLoopRule struct{}

func (r *FmtErrorfInLoopRule) Name() string     { return "fmt-errorf-wrap-loop" }
func (r *FmtErrorfInLoopRule) Category() string { return "allocation" }

func (r *FmtErrorfInLoopRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
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

		ast.Inspect(loopBody, func(inner ast.Node) bool {
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

			if ident.Name == "fmt" && sel.Sel.Name == "Errorf" && len(call.Args) > 0 {
				// Check if format string contains %w
				if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
					if containsWrapVerb(lit.Value) {
						pos := fset.Position(call.Pos())
						issues = append(issues, Issue{
							Rule:     r.Name(),
							Category: r.Category(),
							Severity: SeverityMedium,
							Line:     pos.Line,
							Column:   pos.Column,
							Message:  "fmt.Errorf() with %w in loop - error chain allocates each iteration",
							Why:      "Error wrapping with %w creates an error chain, allocating memory each time. In hot paths, this adds up.",
							Fix:      "For hot paths: (1) Return the original error, (2) Use sentinel errors, (3) Wrap once after the loop with context",
						})
					}
				}
			}

			return true
		})

		return true
	})

	return issues
}

func containsWrapVerb(s string) bool {
	for i := 0; i < len(s)-1; i++ {
		if s[i] == '%' && s[i+1] == 'w' {
			return true
		}
	}
	return false
}
