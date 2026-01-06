package rules

import (
	"go/ast"
	"go/token"
)

func init() {
	RegisterRule("allocation", &UnpreallocatedSliceRule{})
	RegisterRule("allocation", &StringConcatInLoopRule{})
	RegisterRule("allocation", &MapWithoutSizeRule{})
}

// UnpreallocatedSliceRule detects slice append in loops without preallocation
// Now smarter: tracks make() calls with capacity before loops
type UnpreallocatedSliceRule struct{}

func (r *UnpreallocatedSliceRule) Name() string     { return "unpreallocated-slice" }
func (r *UnpreallocatedSliceRule) Category() string { return "allocation" }

func (r *UnpreallocatedSliceRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
	issues := make([]Issue, 0, 8)

	// For each function, track preallocated slices
	ast.Inspect(file, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok {
			return true
		}

		if funcDecl.Body == nil {
			return true
		}

		// Track preallocated slices in this function
		preallocated := findPreallocatedSlices(funcDecl.Body)

		// Now find append in loops
		ast.Inspect(funcDecl.Body, func(inner ast.Node) bool {
			var loopBody *ast.BlockStmt
			var loopNode ast.Node
			switch stmt := inner.(type) {
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

			// Find append calls in the loop body
			ast.Inspect(loopBody, func(stmt ast.Node) bool {
				assign, ok := stmt.(*ast.AssignStmt)
				if !ok {
					return true
				}

				for i, rhs := range assign.Rhs {
					call, ok := rhs.(*ast.CallExpr)
					if !ok {
						continue
					}
					ident, ok := call.Fun.(*ast.Ident)
					if !ok || ident.Name != "append" {
						continue
					}

					// Get the slice variable being appended to
					if i >= len(assign.Lhs) {
						continue
					}
					lhsIdent, ok := assign.Lhs[i].(*ast.Ident)
					if !ok {
						continue
					}

					// Check if this slice was preallocated
					if preallocated[lhsIdent.Name] {
						continue // Skip - preallocated, not an issue
					}

					severity := SeverityLow
					// If loop is large or unbounded, increase severity
					if loopBound < 0 || loopBound > 100 {
						severity = SeverityMedium
					}

					pos := fset.Position(call.Pos())
					issues = append(issues, Issue{
						Rule:     r.Name(),
						Category: r.Category(),
						Severity: severity,
						Line:     pos.Line,
						Column:   pos.Column,
						Message:  "append() in loop without preallocation for '" + lhsIdent.Name + "'",
						Why:      "Slice grows dynamically, causing repeated memory allocations and copies. Each reallocation typically doubles capacity, wasting memory and CPU.",
						Fix:      "Preallocate with make([]T, 0, expectedSize) before the loop if size is known or estimable",
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

// findPreallocatedSlices finds slice variables that were created with make() and a capacity
func findPreallocatedSlices(body *ast.BlockStmt) map[string]bool {
	preallocated := make(map[string]bool)

	ast.Inspect(body, func(n ast.Node) bool {
		// Look for: s := make([]T, 0, cap) or s := make([]T, size)
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

			// Check if it's a slice type
			_, isSlice := call.Args[0].(*ast.ArrayType)
			if !isSlice {
				continue
			}

			// Has capacity if 3 args: make([]T, len, cap)
			// Or if 2 args with non-zero len: make([]T, size)
			hasCapacity := false
			if len(call.Args) >= 3 {
				hasCapacity = true
			} else if len(call.Args) == 2 {
				// Check if size is non-zero or a variable (assumed sized)
				if lit, ok := call.Args[1].(*ast.BasicLit); ok {
					if lit.Kind == token.INT && lit.Value != "0" {
						hasCapacity = true
					}
				} else {
					// It's a variable or expression - assume it's sized
					hasCapacity = true
				}
			}

			if hasCapacity && i < len(assign.Lhs) {
				if lhsIdent, ok := assign.Lhs[i].(*ast.Ident); ok {
					preallocated[lhsIdent.Name] = true
				}
			}
		}

		return true
	})

	return preallocated
}

// StringConcatInLoopRule detects string += concatenation in loops
type StringConcatInLoopRule struct{}

func (r *StringConcatInLoopRule) Name() string     { return "string-concat-loop" }
func (r *StringConcatInLoopRule) Category() string { return "allocation" }

func (r *StringConcatInLoopRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
	issues := make([]Issue, 0, 4)

	// Track strings.Builder usage
	builders := findStringBuilders(file)

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

		// Find += on strings
		ast.Inspect(loopBody, func(inner ast.Node) bool {
			assign, ok := inner.(*ast.AssignStmt)
			if !ok || assign.Tok != token.ADD_ASSIGN {
				return true
			}

			// Get the variable being concatenated
			if len(assign.Lhs) > 0 {
				if ident, ok := assign.Lhs[0].(*ast.Ident); ok {
					// Skip if using strings.Builder
					if builders[ident.Name] {
						return true
					}
				}
			}

			// Check if RHS involves strings
			for _, rhs := range assign.Rhs {
				if isStringExpr(rhs) {
					pos := fset.Position(assign.Pos())
					issues = append(issues, Issue{
						Rule:     r.Name(),
						Category: r.Category(),
						Severity: SeverityMedium,
						Line:     pos.Line,
						Column:   pos.Column,
						Message:  "String concatenation in loop creates O(n²) allocations",
						Why:      "Strings are immutable in Go. Each += creates a new string, copying all previous content. Building a 1000-char string this way allocates ~500KB total.",
						Fix:      "Use strings.Builder: var b strings.Builder; for ... { b.WriteString(s) }; result := b.String()",
					})
				}
			}
			return true
		})

		return true
	})

	return issues
}

// findStringBuilders finds variables declared as strings.Builder
func findStringBuilders(file *ast.File) map[string]bool {
	builders := make(map[string]bool)

	ast.Inspect(file, func(n ast.Node) bool {
		// var b strings.Builder
		genDecl, ok := n.(*ast.GenDecl)
		if ok && genDecl.Tok == token.VAR {
			for _, spec := range genDecl.Specs {
				valueSpec, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				if sel, ok := valueSpec.Type.(*ast.SelectorExpr); ok {
					if ident, ok := sel.X.(*ast.Ident); ok {
						if ident.Name == "strings" && sel.Sel.Name == "Builder" {
							for _, name := range valueSpec.Names {
								builders[name.Name] = true
							}
						}
					}
				}
			}
		}

		// Also check for short declarations: b := strings.Builder{}
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}

		for i, rhs := range assign.Rhs {
			compLit, ok := rhs.(*ast.CompositeLit)
			if !ok {
				continue
			}
			if sel, ok := compLit.Type.(*ast.SelectorExpr); ok {
				if ident, ok := sel.X.(*ast.Ident); ok {
					if ident.Name == "strings" && sel.Sel.Name == "Builder" {
						if i < len(assign.Lhs) {
							if lhsIdent, ok := assign.Lhs[i].(*ast.Ident); ok {
								builders[lhsIdent.Name] = true
							}
						}
					}
				}
			}
		}

		return true
	})

	return builders
}

func isStringExpr(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.BasicLit:
		return e.Kind == token.STRING
	case *ast.BinaryExpr:
		return isStringExpr(e.X) || isStringExpr(e.Y)
	}
	return false
}

// MapWithoutSizeRule detects map creation without size hint when populated in a loop
type MapWithoutSizeRule struct{}

func (r *MapWithoutSizeRule) Name() string     { return "map-without-size" }
func (r *MapWithoutSizeRule) Category() string { return "allocation" }

func (r *MapWithoutSizeRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
	issues := make([]Issue, 0, 4)

	// Only flag maps that are actually populated in loops
	ast.Inspect(file, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok {
			return true
		}

		if funcDecl.Body == nil {
			return true
		}

		// Find maps without size hints
		unsizedMaps := findUnsizedMaps(funcDecl.Body, fset)

		// Find maps that are populated in loops
		populatedInLoop := findMapsPopulatedInLoop(funcDecl.Body)

		// Only report maps that are both unsized AND populated in a loop
		for name, pos := range unsizedMaps {
			if populatedInLoop[name] {
				issues = append(issues, Issue{
					Rule:     r.Name(),
					Category: r.Category(),
					Severity: SeverityLow,
					Line:     pos.Line,
					Column:   pos.Column,
					Message:  "Map '" + name + "' created without size hint and populated in loop",
					Why:      "Maps without size hints start small and rehash as they grow. If you know the approximate size, providing it avoids rehashing overhead.",
					Fix:      "Use make(map[K]V, expectedSize) if the size is known or estimable from the loop source",
				})
			}
		}

		return true
	})

	return issues
}

func findUnsizedMaps(body *ast.BlockStmt, fset *token.FileSet) map[string]token.Position {
	maps := make(map[string]token.Position)

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

			// Check if it's a map type without size
			_, isMap := call.Args[0].(*ast.MapType)
			if !isMap || len(call.Args) > 1 {
				continue // Has size hint or not a map
			}

			if i < len(assign.Lhs) {
				if lhsIdent, ok := assign.Lhs[i].(*ast.Ident); ok {
					maps[lhsIdent.Name] = fset.Position(call.Pos())
				}
			}
		}

		return true
	})

	return maps
}

func findMapsPopulatedInLoop(body *ast.BlockStmt) map[string]bool {
	populated := make(map[string]bool)

	ast.Inspect(body, func(n ast.Node) bool {
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

		// Find map assignments in the loop: m[k] = v
		ast.Inspect(loopBody, func(inner ast.Node) bool {
			assign, ok := inner.(*ast.AssignStmt)
			if !ok {
				return true
			}

			for _, lhs := range assign.Lhs {
				if indexExpr, ok := lhs.(*ast.IndexExpr); ok {
					if ident, ok := indexExpr.X.(*ast.Ident); ok {
						populated[ident.Name] = true
					}
				}
			}

			return true
		})

		return true
	})

	return populated
}
