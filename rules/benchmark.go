package rules

import (
	"go/ast"
	"go/token"
)

func init() {
	RegisterRule("benchmark", &BenchmarkSuggestionRule{})
}

// BenchmarkSuggestionRule suggests benchmarks for functions with detected issues
type BenchmarkSuggestionRule struct{}

func (r *BenchmarkSuggestionRule) Name() string     { return "benchmark-suggestion" }
func (r *BenchmarkSuggestionRule) Category() string { return "benchmark" }

func (r *BenchmarkSuggestionRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
	var issues []Issue

	// Find functions that have performance-sensitive patterns
	ast.Inspect(file, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok {
			return true
		}

		if funcDecl.Body == nil {
			return true
		}

		funcName := funcDecl.Name.Name

		// Skip if already a benchmark
		if len(funcName) > 9 && funcName[:9] == "Benchmark" {
			return true
		}

		// Check for performance-sensitive patterns
		patterns := checkPerformancePatterns(funcDecl.Body)

		if len(patterns) > 0 {
			pos := fset.Position(funcDecl.Pos())

			// Generate benchmark suggestion
			benchCode := generateBenchmarkCode(funcName, funcDecl)

			issues = append(issues, Issue{
				Rule:     r.Name(),
				Category: r.Category(),
				Severity: SeverityLow,
				Line:     pos.Line,
				Column:   pos.Column,
				Message:  "Function '" + funcName + "' has " + itoa(len(patterns)) + " performance-sensitive pattern(s) - consider adding benchmark",
				Why:      "This function contains: " + joinPatterns(patterns) + ". Benchmarking helps track performance regressions.",
				Fix:      "Add benchmark:\n" + benchCode,
			})
		}

		return true
	})

	return issues
}

type perfPattern struct {
	name  string
	count int
}

func checkPerformancePatterns(body *ast.BlockStmt) []perfPattern {
	var patterns []perfPattern

	loopCount := 0
	sqlCount := 0
	allocCount := 0
	reflectCount := 0

	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.RangeStmt, *ast.ForStmt:
			loopCount++
		case *ast.CallExpr:
			if sel, ok := node.Fun.(*ast.SelectorExpr); ok {
				if ident, ok := sel.X.(*ast.Ident); ok {
					// Check for SQL
					sqlMethods := map[string]bool{
						"Query": true, "QueryRow": true, "Exec": true,
						"QueryContext": true, "ExecContext": true,
					}
					if sqlMethods[sel.Sel.Name] {
						sqlCount++
					}

					// Check for reflection
					if ident.Name == "reflect" {
						reflectCount++
					}

					// Check for JSON
					if ident.Name == "json" && (sel.Sel.Name == "Marshal" || sel.Sel.Name == "Unmarshal") {
						allocCount++
					}
				}

				// Check for make
				if ident, ok := node.Fun.(*ast.Ident); ok && ident.Name == "make" {
					allocCount++
				}
			}
		}
		return true
	})

	if loopCount > 0 {
		patterns = append(patterns, perfPattern{"loops", loopCount})
	}
	if sqlCount > 0 {
		patterns = append(patterns, perfPattern{"database calls", sqlCount})
	}
	if allocCount > 0 {
		patterns = append(patterns, perfPattern{"allocations", allocCount})
	}
	if reflectCount > 0 {
		patterns = append(patterns, perfPattern{"reflection", reflectCount})
	}

	return patterns
}

func generateBenchmarkCode(funcName string, funcDecl *ast.FuncDecl) string {
	// Generate basic benchmark scaffold
	benchName := "Benchmark" + capitalizeFirst(funcName)

	code := "func " + benchName + "(b *testing.B) {\n"
	code += "\t// Setup: initialize test data\n"

	// Add parameter hints based on function signature
	if funcDecl.Type.Params != nil && len(funcDecl.Type.Params.List) > 0 {
		code += "\t// params: "
		for i, param := range funcDecl.Type.Params.List {
			if i > 0 {
				code += ", "
			}
			for j, name := range param.Names {
				if j > 0 {
					code += ", "
				}
				code += name.Name
			}
		}
		code += "\n"
	}

	code += "\n\tb.ResetTimer()\n"
	code += "\tfor i := 0; i < b.N; i++ {\n"
	code += "\t\t" + funcName + "(...) // Add arguments\n"
	code += "\t}\n"
	code += "}"

	return code
}

func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	first := s[0]
	if first >= 'a' && first <= 'z' {
		return string(first-32) + s[1:]
	}
	return s
}

func joinPatterns(patterns []perfPattern) string {
	if len(patterns) == 0 {
		return ""
	}

	result := ""
	for i, p := range patterns {
		if i > 0 {
			result += ", "
		}
		result += itoa(p.count) + " " + p.name
	}
	return result
}
