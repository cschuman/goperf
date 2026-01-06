package rules

import (
	"go/ast"
	"go/token"
)

func init() {
	RegisterRule("cache", &RepeatedRegexpCompileRule{})
	RegisterRule("cache", &RepeatedTemplateParseRule{})
	RegisterRule("cache", &RegexpMatchStringRule{})
	RegisterRule("cache", &JSONSchemaValidationRule{})
}

// RepeatedRegexpCompileRule detects regexp.Compile inside functions (should be package-level)
type RepeatedRegexpCompileRule struct{}

func (r *RepeatedRegexpCompileRule) Name() string     { return "repeated-regexp-compile" }
func (r *RepeatedRegexpCompileRule) Category() string { return "cache" }

func (r *RepeatedRegexpCompileRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
	issues := make([]Issue, 0, 4)

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
	issues := make([]Issue, 0, 4)

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

// RegexpMatchStringRule detects regexp.MatchString in loops (compiles each time)
type RegexpMatchStringRule struct{}

func (r *RegexpMatchStringRule) Name() string     { return "regexp-match-string-loop" }
func (r *RegexpMatchStringRule) Category() string { return "cache" }

func (r *RegexpMatchStringRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
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

			// regexp.MatchString, regexp.Match compile the pattern each time
			if ident.Name == "regexp" {
				switch sel.Sel.Name {
				case "MatchString", "Match", "ReplaceAllString", "ReplaceAll",
					"FindString", "FindAllString", "FindStringSubmatch":
					pos := fset.Position(call.Pos())
					issues = append(issues, Issue{
						Rule:     r.Name(),
						Category: r.Category(),
						Severity: SeverityHigh,
						Line:     pos.Line,
						Column:   pos.Column,
						Message:  "regexp." + sel.Sel.Name + "() in loop - compiles regex on EVERY call",
						Why:      "regexp.MatchString and similar functions compile the pattern each time. This is O(n*m) where m is pattern complexity.",
						Fix:      "Compile once: var re = regexp.MustCompile(`pattern`); then use re.MatchString(s) in the loop",
					})
				}
			}

			return true
		})

		return true
	})

	return issues
}

// JSONSchemaValidationRule detects JSON schema validation in loops
type JSONSchemaValidationRule struct{}

func (r *JSONSchemaValidationRule) Name() string     { return "json-schema-in-loop" }
func (r *JSONSchemaValidationRule) Category() string { return "cache" }

func (r *JSONSchemaValidationRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
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

		ast.Inspect(loopBody, func(inner ast.Node) bool {
			call, ok := inner.(*ast.CallExpr)
			if !ok {
				return true
			}

			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			// Check for schema compilation/validation patterns
			if sel.Sel.Name == "Compile" || sel.Sel.Name == "NewSchema" || sel.Sel.Name == "Validate" {
				// Look for jsonschema, gojsonschema, etc.
				ident, ok := sel.X.(*ast.Ident)
				if ok && (ident.Name == "jsonschema" || ident.Name == "gojsonschema" || ident.Name == "schema") {
					pos := fset.Position(call.Pos())
					issues = append(issues, Issue{
						Rule:     r.Name(),
						Category: r.Category(),
						Severity: SeverityMedium,
						Line:     pos.Line,
						Column:   pos.Column,
						Message:  "JSON schema compilation/validation in loop - compile schema once",
						Why:      "Compiling JSON schemas is expensive. Recompiling for each item wastes CPU.",
						Fix:      "Compile the schema once outside the loop, then call Validate() in the loop",
					})
				}
			}

			return true
		})

		return true
	})

	return issues
}
