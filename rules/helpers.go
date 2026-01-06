package rules

import (
	"go/ast"
	"go/token"
	"strings"
)

// isHTTPHandler checks if a function declaration is an HTTP handler
// by looking for (w http.ResponseWriter, r *http.Request) or similar patterns
func isHTTPHandler(funcDecl *ast.FuncDecl) bool {
	if funcDecl.Type.Params == nil || len(funcDecl.Type.Params.List) < 1 {
		return false
	}

	params := funcDecl.Type.Params.List

	// Check for standard library pattern: (w http.ResponseWriter, r *http.Request)
	for _, param := range params {
		typeName := getTypeName(param.Type)
		if typeName == "ResponseWriter" || typeName == "Request" {
			return true
		}
	}

	// Check for common framework patterns:
	// - Echo: func(c echo.Context)
	// - Gin: func(c *gin.Context)
	// - Chi: func(w http.ResponseWriter, r *http.Request)
	// - Fiber: func(c *fiber.Ctx)
	for _, param := range params {
		typeName := getTypeName(param.Type)
		if strings.HasSuffix(typeName, "Context") || strings.HasSuffix(typeName, "Ctx") {
			return true
		}
	}

	return false
}

// estimateStructSizes estimates the memory size of struct types in a file
func estimateStructSizes(file *ast.File) map[string]int {
	sizes := make(map[string]int)

	// Common known types with their typical sizes (64-bit)
	knownSizes := map[string]int{
		"string":     16, // ptr + len
		"int":        8,
		"int64":      8,
		"int32":      4,
		"int16":      2,
		"int8":       1,
		"uint":       8,
		"uint64":     8,
		"uint32":     4,
		"uint16":     2,
		"uint8":      1,
		"float64":    8,
		"float32":    4,
		"bool":       1,
		"byte":       1,
		"rune":       4,
		"time.Time":  24,
		"sync.Mutex": 8,
	}

	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}

		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}

			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				continue
			}

			size := 0
			if structType.Fields != nil {
				for _, field := range structType.Fields.List {
					fieldSize := 8 // default assumption
					fieldTypeName := getTypeName(field.Type)
					if known, ok := knownSizes[fieldTypeName]; ok {
						fieldSize = known
					}
					// Multiply by number of names (e.g., "a, b int")
					count := len(field.Names)
					if count == 0 {
						count = 1 // embedded field
					}
					size += fieldSize * count
				}
			}

			sizes[typeSpec.Name.Name] = size
		}
	}

	return sizes
}

// getTypeName extracts the type name from an AST expression
func getTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return t.Sel.Name
	case *ast.StarExpr:
		return "*" + getTypeName(t.X)
	case *ast.ArrayType:
		return "[]" + getTypeName(t.Elt)
	}
	return ""
}

// itoa converts an integer to a string (simple implementation without importing strconv)
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	if n < 0 {
		return "-" + itoa(-n)
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

// isInLoop checks if a position is within a loop body
func isInLoop(file *ast.File, fset *token.FileSet, pos token.Pos) bool {
	targetLine := fset.Position(pos).Line
	inLoop := false

	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil {
			return true
		}

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

		startLine := fset.Position(loopBody.Lbrace).Line
		endLine := fset.Position(loopBody.Rbrace).Line

		if targetLine >= startLine && targetLine <= endLine {
			inLoop = true
			return false
		}

		return true
	})

	return inLoop
}

// findFunctionContaining finds the function declaration containing a position
func findFunctionContaining(file *ast.File, fset *token.FileSet, pos token.Pos) *ast.FuncDecl {
	targetLine := fset.Position(pos).Line

	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok || funcDecl.Body == nil {
			continue
		}

		startLine := fset.Position(funcDecl.Body.Lbrace).Line
		endLine := fset.Position(funcDecl.Body.Rbrace).Line

		if targetLine >= startLine && targetLine <= endLine {
			return funcDecl
		}
	}

	return nil
}
