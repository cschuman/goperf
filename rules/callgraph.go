package rules

import (
	"go/ast"
	"go/token"
)

func init() {
	RegisterRule("database", &IndirectSQLInLoopRule{})
}

// IndirectSQLInLoopRule detects when functions containing SQL are called in loops
// This catches the N+1 pattern even when SQL is wrapped in helper functions
type IndirectSQLInLoopRule struct{}

func (r *IndirectSQLInLoopRule) Name() string     { return "indirect-sql-in-loop" }
func (r *IndirectSQLInLoopRule) Category() string { return "database" }

func (r *IndirectSQLInLoopRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
	var issues []Issue

	// First pass: find functions that contain direct SQL calls
	sqlFuncs := findFunctionsWithSQL(file)

	// Second pass: find loops that call these functions
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

		// Find function calls in the loop body
		ast.Inspect(loopBody, func(inner ast.Node) bool {
			call, ok := inner.(*ast.CallExpr)
			if !ok {
				return true
			}

			// Get the function name being called
			var funcName string
			switch fn := call.Fun.(type) {
			case *ast.Ident:
				funcName = fn.Name
			case *ast.SelectorExpr:
				// Method call like s.Save() - check method name
				funcName = fn.Sel.Name
			}

			if funcName == "" {
				return true
			}

			// Check if this function contains SQL
			if sqlInfo, ok := sqlFuncs[funcName]; ok {
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
					Message:  "Function '" + funcName + "' called in loop contains SQL (" + sqlInfo.method + ")",
					Why:      "The function " + funcName + "() contains database operations. Calling it in a loop creates N+1 query patterns even though the SQL isn't directly visible.",
					Fix:      "Refactor to batch: (1) Collect items first, (2) Pass slice to function, (3) Use single batch query inside function",
				})
			}

			return true
		})

		return true
	})

	return issues
}

type sqlFuncInfo struct {
	method string
	line   int
}

// findFunctionsWithSQL finds all functions in the file that contain SQL operations
func findFunctionsWithSQL(file *ast.File) map[string]sqlFuncInfo {
	sqlFuncs := make(map[string]sqlFuncInfo)

	sqlMethods := map[string]bool{
		"Query": true, "QueryRow": true, "Exec": true,
		"QueryRowContext": true, "QueryContext": true, "ExecContext": true,
		"Get": true, "Select": true,
	}

	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok || funcDecl.Body == nil {
			continue
		}

		funcName := funcDecl.Name.Name

		// Skip methods that are themselves SQL operations
		if sqlMethods[funcName] {
			continue
		}

		// Check if function body contains SQL calls
		ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			if sqlMethods[sel.Sel.Name] {
				sqlFuncs[funcName] = sqlFuncInfo{
					method: sel.Sel.Name,
				}
				return false // Found one, stop searching this function
			}

			return true
		})
	}

	return sqlFuncs
}

// ReflectionInLoopRule detects reflection usage in loops (advanced)
type ReflectionInLoopRule struct{}

func init() {
	RegisterRule("io", &ReflectionInLoopRule{})
}

func (r *ReflectionInLoopRule) Name() string     { return "reflection-in-loop" }
func (r *ReflectionInLoopRule) Category() string { return "io" }

func (r *ReflectionInLoopRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
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

		// Find reflect.ValueOf, reflect.TypeOf calls in loops
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

			if ident.Name == "reflect" {
				reflectMethods := map[string]bool{
					"ValueOf": true, "TypeOf": true, "New": true,
					"MakeSlice": true, "MakeMap": true, "MakeFunc": true,
				}

				if reflectMethods[sel.Sel.Name] {
					pos := fset.Position(call.Pos())
					issues = append(issues, Issue{
						Rule:     r.Name(),
						Category: r.Category(),
						Severity: SeverityMedium,
						Line:     pos.Line,
						Column:   pos.Column,
						Message:  "reflect." + sel.Sel.Name + "() inside loop - significant overhead",
						Why:      "Reflection is slow compared to direct type access. In loops, this overhead multiplies significantly.",
						Fix:      "Consider: (1) Caching reflection results outside loop, (2) Using type assertions, (3) Code generation for type-specific operations",
					})
				}
			}

			return true
		})

		return true
	})

	return issues
}

// SyncPoolOpportunityRule detects repeated allocations that could use sync.Pool
type SyncPoolOpportunityRule struct{}

func init() {
	RegisterRule("allocation", &SyncPoolOpportunityRule{})
}

func (r *SyncPoolOpportunityRule) Name() string     { return "sync-pool-opportunity" }
func (r *SyncPoolOpportunityRule) Category() string { return "allocation" }

func (r *SyncPoolOpportunityRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
	var issues []Issue

	// Find functions that are called frequently (heuristic: called in loops)
	// and allocate buffers/slices that could be pooled
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

		// Find allocations that could be pooled
		ast.Inspect(loopBody, func(inner ast.Node) bool {
			call, ok := inner.(*ast.CallExpr)
			if !ok {
				return true
			}

			ident, ok := call.Fun.(*ast.Ident)
			if !ok {
				return true
			}

			// Check for make([]byte, ...) - common buffer allocation
			if ident.Name == "make" && len(call.Args) >= 1 {
				if arrayType, ok := call.Args[0].(*ast.ArrayType); ok {
					if elemIdent, ok := arrayType.Elt.(*ast.Ident); ok {
						if elemIdent.Name == "byte" {
							pos := fset.Position(call.Pos())
							issues = append(issues, Issue{
								Rule:     r.Name(),
								Category: r.Category(),
								Severity: SeverityLow,
								Line:     pos.Line,
								Column:   pos.Column,
								Message:  "Byte slice allocation in loop - consider sync.Pool",
								Why:      "Allocating buffers in a loop creates GC pressure. For high-frequency operations, sync.Pool can reuse allocations.",
								Fix:      "Use sync.Pool: var bufPool = sync.Pool{New: func() any { return make([]byte, size) }}; buf := bufPool.Get().([]byte); defer bufPool.Put(buf)",
							})
						}
					}
				}
			}

			// Check for bytes.Buffer creation
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				if x, ok := sel.X.(*ast.Ident); ok {
					if x.Name == "bytes" && sel.Sel.Name == "NewBuffer" {
						pos := fset.Position(call.Pos())
						issues = append(issues, Issue{
							Rule:     r.Name(),
							Category: r.Category(),
							Severity: SeverityLow,
							Line:     pos.Line,
							Column:   pos.Column,
							Message:  "bytes.Buffer creation in loop - consider sync.Pool",
							Why:      "Creating new buffers in a loop creates GC pressure. sync.Pool can reuse buffer instances.",
							Fix:      "Pool bytes.Buffer instances and Reset() before reuse",
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
