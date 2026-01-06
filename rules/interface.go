package rules

import (
	"go/ast"
	"go/token"
)

func init() {
	RegisterRule("allocation", &InterfaceBoxingInLoopRule{})
	RegisterRule("allocation", &VariadicInterfaceRule{})
	RegisterRule("allocation", &TypeAssertionInLoopRule{})
}

// InterfaceBoxingInLoopRule detects interface{} assignments in loops
type InterfaceBoxingInLoopRule struct{}

func (r *InterfaceBoxingInLoopRule) Name() string     { return "interface-boxing-loop" }
func (r *InterfaceBoxingInLoopRule) Category() string { return "allocation" }

func (r *InterfaceBoxingInLoopRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
	issues := make([]Issue, 0, 4)

	// Find functions that take interface{} or any parameters
	interfaceFuncs := findInterfaceFunctions(file)

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

			// Check if calling a function that takes interface{}
			funcName := getFuncName(call.Fun)
			if interfaceFuncs[funcName] && len(call.Args) > 0 {
				// Check if passing concrete types (will be boxed)
				for _, arg := range call.Args {
					if !isInterfaceExpr(arg) {
						// Concrete type being boxed
						pos := fset.Position(call.Pos())
						issues = append(issues, Issue{
							Rule:     r.Name(),
							Category: r.Category(),
							Severity: SeverityLow,
							Line:     pos.Line,
							Column:   pos.Column,
							Message:  "Concrete type boxed to interface{} in loop - causes allocation",
							Why:      "Converting concrete types to interface{} allocates memory for the interface header. In hot loops, this adds GC pressure.",
							Fix:      "Consider: (1) Type-specific function overloads, (2) Generics (Go 1.18+), (3) Code generation for hot paths",
						})
						break // Only report once per call
					}
				}
			}

			return true
		})

		return true
	})

	return issues
}

// VariadicInterfaceRule detects slice passed to ...interface{} causing per-element allocation
type VariadicInterfaceRule struct{}

func (r *VariadicInterfaceRule) Name() string     { return "variadic-interface" }
func (r *VariadicInterfaceRule) Category() string { return "allocation" }

func (r *VariadicInterfaceRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
	issues := make([]Issue, 0, 4)

	// Find functions with variadic interface{} parameters
	variadicFuncs := map[string]bool{
		"Printf":  true,
		"Sprintf": true,
		"Fprintf": true,
		"Errorf":  true,
		"Fatalf":  true,
		"Panicf":  true,
		"Logf":    true,
		"Debugf":  true,
		"Infof":   true,
		"Warnf":   true,
	}

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

			// Get function name
			var funcName string
			switch fn := call.Fun.(type) {
			case *ast.Ident:
				funcName = fn.Name
			case *ast.SelectorExpr:
				funcName = fn.Sel.Name
			}

			if variadicFuncs[funcName] {
				// Check for complex arguments that will allocate
				complexArgs := 0
				for _, arg := range call.Args {
					// Skip the format string
					if lit, ok := arg.(*ast.BasicLit); ok && lit.Kind == token.STRING {
						continue
					}
					// Check for complex expressions
					switch arg.(type) {
					case *ast.CallExpr: // Function calls allocate return value
						complexArgs++
					case *ast.BinaryExpr: // Operations may allocate
						complexArgs++
					case *ast.CompositeLit: // Struct/slice literals allocate
						complexArgs++
					}
				}

				if complexArgs > 2 {
					pos := fset.Position(call.Pos())
					issues = append(issues, Issue{
						Rule:     r.Name(),
						Category: r.Category(),
						Severity: SeverityLow,
						Line:     pos.Line,
						Column:   pos.Column,
						Message:  funcName + "() with many arguments in loop - each arg boxes to interface{}",
						Why:      "Each argument to Printf-style functions is boxed to interface{}, causing allocations. Complex expressions allocate twice.",
						Fix:      "For hot paths: (1) Use structured logging, (2) Pre-format strings, (3) Use log level checks to skip entirely",
					})
				}
			}

			return true
		})

		return true
	})

	return issues
}

// TypeAssertionInLoopRule detects type assertions in loops
type TypeAssertionInLoopRule struct{}

func (r *TypeAssertionInLoopRule) Name() string     { return "type-assertion-loop" }
func (r *TypeAssertionInLoopRule) Category() string { return "allocation" }

func (r *TypeAssertionInLoopRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
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

		// Count type assertions in loop
		assertions := 0
		var lastPos token.Position

		ast.Inspect(loopBody, func(inner ast.Node) bool {
			switch node := inner.(type) {
			case *ast.TypeAssertExpr:
				assertions++
				lastPos = fset.Position(node.Pos())
			case *ast.TypeSwitchStmt:
				assertions++
				lastPos = fset.Position(node.Pos())
			}
			return true
		})

		// Only flag if multiple assertions
		if assertions > 2 {
			issues = append(issues, Issue{
				Rule:     r.Name(),
				Category: r.Category(),
				Severity: SeverityLow,
				Line:     lastPos.Line,
				Column:   lastPos.Column,
				Message:  "Multiple type assertions in loop - consider type-specific code paths",
				Why:      "Type assertions have a small overhead. Multiple assertions per iteration suggest the code might benefit from type-specific handling.",
				Fix:      "Consider: (1) Type switch outside loop, (2) Generic functions, (3) Interface with specific methods instead of type assertions",
			})
		}

		return true
	})

	return issues
}

// Helper functions

func findInterfaceFunctions(file *ast.File) map[string]bool {
	funcs := make(map[string]bool)

	ast.Inspect(file, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok {
			return true
		}

		if funcDecl.Type.Params == nil {
			return true
		}

		for _, param := range funcDecl.Type.Params.List {
			if isInterfaceType(param.Type) {
				funcs[funcDecl.Name.Name] = true
				break
			}
		}

		return true
	})

	return funcs
}

func isInterfaceType(expr ast.Expr) bool {
	switch t := expr.(type) {
	case *ast.InterfaceType:
		return true
	case *ast.Ident:
		return t.Name == "any" || t.Name == "error"
	case *ast.Ellipsis:
		return isInterfaceType(t.Elt)
	}
	return false
}

func isInterfaceExpr(expr ast.Expr) bool {
	// This is a heuristic - we can't know types without type checking
	// Assume identifiers ending in "err" or named "any" are interfaces
	if ident, ok := expr.(*ast.Ident); ok {
		name := ident.Name
		if name == "err" || name == "error" || name == "any" {
			return true
		}
	}
	return false
}

func getFuncName(expr ast.Expr) string {
	switch fn := expr.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		return fn.Sel.Name
	}
	return ""
}
