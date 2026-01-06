package rules

import (
	"go/ast"
	"go/token"
)

func init() {
	RegisterRule("cache", &RepeatedRegexpCompileRule{})
	RegisterRule("cache", &RepeatedTemplateParseRule{})
}

// RepeatedRegexpCompileRule detects regexp.Compile inside functions (should be package-level)
type RepeatedRegexpCompileRule struct{}

func (r *RepeatedRegexpCompileRule) Name() string     { return "repeated-regexp-compile" }
func (r *RepeatedRegexpCompileRule) Category() string { return "cache" }

func (r *RepeatedRegexpCompileRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
	var issues []Issue

	// Track if we're inside a function
	var inFunc bool

	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncDecl:
			inFunc = true
			return true
		case *ast.FuncLit:
			inFunc = true
			return true
		case *ast.CallExpr:
			if !inFunc {
				return true
			}

			sel, ok := node.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			ident, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}

			// Check for regexp.Compile, regexp.MustCompile
			if ident.Name == "regexp" && (sel.Sel.Name == "Compile" || sel.Sel.Name == "MustCompile") {
				pos := fset.Position(node.Pos())
				issues = append(issues, Issue{
					Rule:     r.Name(),
					Category: r.Category(),
					Severity: SeverityMedium,
					Line:     pos.Line,
					Column:   pos.Column,
					Message:  "regexp." + sel.Sel.Name + "() called inside function - compile once at package level",
					Why:      "Compiling regexps is expensive. Recompiling the same pattern on every function call wastes CPU.",
					Fix:      "Move to package-level var: var myRegexp = regexp.MustCompile(`pattern`)",
				})
			}
		}
		return true
	})

	return issues
}

// RepeatedTemplateParseRule detects template.Parse inside functions
type RepeatedTemplateParseRule struct{}

func (r *RepeatedTemplateParseRule) Name() string     { return "repeated-template-parse" }
func (r *RepeatedTemplateParseRule) Category() string { return "cache" }

func (r *RepeatedTemplateParseRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
	var issues []Issue

	var inFunc bool

	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncDecl:
			inFunc = true
			return true
		case *ast.FuncLit:
			inFunc = true
			return true
		case *ast.CallExpr:
			if !inFunc {
				return true
			}

			sel, ok := node.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			// Check for template.Parse, template.ParseFiles, etc.
			if sel.Sel.Name == "Parse" || sel.Sel.Name == "ParseFiles" || sel.Sel.Name == "ParseGlob" {
				// Try to see if it's a template operation
				if chainSel, ok := sel.X.(*ast.CallExpr); ok {
					if innerSel, ok := chainSel.Fun.(*ast.SelectorExpr); ok {
						if ident, ok := innerSel.X.(*ast.Ident); ok {
							if ident.Name == "template" {
								pos := fset.Position(node.Pos())
								issues = append(issues, Issue{
									Rule:     r.Name(),
									Category: r.Category(),
									Severity: SeverityMedium,
									Line:     pos.Line,
									Column:   pos.Column,
									Message:  "Template parsing inside function - parse once at startup",
									Why:      "Parsing templates is expensive. Reparsing on every request wastes CPU and memory.",
									Fix:      "Parse templates once at package init or application startup, then reuse with template.Execute()",
								})
							}
						}
					}
				}
			}
		}
		return true
	})

	return issues
}
