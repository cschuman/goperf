package rules

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"time"
)

const (
	ParseTimeout = 5 * time.Second
	MaxASTNodes  = 100000
	MaxASTDepth  = 1000
)

// Analyzer runs rules against Go source files
type Analyzer struct {
	config AnalyzerConfig
	rules  []Rule
	Errors []string
}

type parseResult struct {
	file     *ast.File
	err      error
	panicErr error
}

// ASTComplexityValidator enforces node count and depth limits during AST walks.
type ASTComplexityValidator struct {
	maxNodes int
	maxDepth int
	nodes    int
	depth    int
}

func (v *ASTComplexityValidator) Visit(n ast.Node) ast.Visitor {
	if n == nil {
		if v.depth > 0 {
			v.depth--
		}
		return v
	}

	v.nodes++
	v.depth++

	if v.nodes > v.maxNodes {
		panic("ast node limit exceeded")
	}
	if v.depth > v.maxDepth {
		panic("ast depth limit exceeded")
	}

	return v
}

func ValidateASTComplexity(file *ast.File) {
	validator := &ASTComplexityValidator{
		maxNodes: MaxASTNodes,
		maxDepth: MaxASTDepth,
	}
	ast.Walk(validator, file)
}

// NewAnalyzer creates a new analyzer with the given config
func NewAnalyzer(config AnalyzerConfig) *Analyzer {
	// Estimate total rules: ~3 rules per category on average
	estimatedRules := len(config.Rules) * 3
	a := &Analyzer{
		config: config,
		rules:  make([]Rule, 0, estimatedRules),
	}

	// Collect rules based on config
	for _, category := range config.Rules {
		if rules, ok := RuleRegistry[category]; ok {
			a.rules = append(a.rules, rules...)
		}
	}

	return a
}

// Analyze runs all rules against the given files
func (a *Analyzer) Analyze(files []string) ([]Issue, []string) {
	// Preallocate with estimate: ~2 issues per file on average
	allIssues := make([]Issue, 0, len(files)*2)
	a.Errors = make([]string, 0, len(files))

	fset := token.NewFileSet()

	for _, filename := range files {
		// Skip ignored paths
		skip := false
		for _, ignore := range a.config.IgnorePaths {
			if strings.Contains(filename, ignore) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}

		// Read source
		src, err := os.ReadFile(filename)
		if err != nil {
			a.recordError("read", filename, err)
			continue
		}

		// Parse file with a timeout and AST complexity enforcement
		parseCtx, cancel := context.WithTimeout(context.Background(), ParseTimeout)
		resultCh := make(chan parseResult, 1)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					resultCh <- parseResult{panicErr: fmt.Errorf("parser panic: %v", r)}
				}
			}()

			file, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
			if err == nil {
				ValidateASTComplexity(file)
			}
			resultCh <- parseResult{file: file, err: err}
		}()

		var file *ast.File
		select {
		case <-parseCtx.Done():
			cancel()
			a.recordError("parse", filename, parseCtx.Err())
			continue
		case result := <-resultCh:
			cancel()
			if result.panicErr != nil || result.err != nil {
				if result.panicErr != nil {
					a.recordError("parse", filename, result.panicErr)
				} else {
					a.recordError("parse", filename, result.err)
				}
				continue
			}
			file = result.file
		}

		// Parse ignore comments
		ignoreSet := NewIgnoreSet(src)

		// Run all rules
		for _, rule := range a.rules {
			issues := rule.Check(file, fset, src)
			for i := range issues {
				issues[i].File = filename

				// Skip ignored issues
				if ignoreSet.ShouldIgnore(issues[i].Line, issues[i].Rule) {
					continue
				}

				if a.config.Context > 0 {
					pos := fset.Position(token.Pos(issues[i].Line))
					pos.Line = issues[i].Line
					issues[i].Context = ExtractContext(src, pos, a.config.Context)
				}
				allIssues = append(allIssues, issues[i])
			}
		}
	}

	if len(a.Errors) > 0 {
		fmt.Fprintf(os.Stderr, "Warning: %d files could not be analyzed\n", len(a.Errors))
	}

	return allIssues, a.Errors
}

func (a *Analyzer) recordError(action, filename string, err error) {
	a.Errors = append(a.Errors, fmt.Sprintf("%s: %v", filename, err))
	if a.config.Verbose {
		fmt.Fprintf(os.Stderr, "Warning: failed to %s %s: %v\n", action, filename, err)
	}
}

// Helper functions for rule implementations

// FindNestedRangeLoops finds nested for-range loops
func FindNestedRangeLoops(file *ast.File, fset *token.FileSet) []ast.Node {
	nested := make([]ast.Node, 0, 4) // Most files have few nested loops

	ast.Inspect(file, func(n ast.Node) bool {
		outerRange, ok := n.(*ast.RangeStmt)
		if !ok {
			return true
		}

		// Look for nested range inside this one
		ast.Inspect(outerRange.Body, func(inner ast.Node) bool {
			if innerRange, ok := inner.(*ast.RangeStmt); ok {
				// Found nested range
				nested = append(nested, innerRange)
				return false
			}
			return true
		})

		return true
	})

	return nested
}

// FindAppendInLoop finds append calls inside loops without preallocation
func FindAppendInLoop(file *ast.File, fset *token.FileSet) []AppendInLoopInfo {
	results := make([]AppendInLoopInfo, 0, 8) // Typical file has few append-in-loop issues

	ast.Inspect(file, func(n ast.Node) bool {
		// Look for for statements (both range and regular)
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

		// Find append calls in the loop body
		ast.Inspect(loopBody, func(inner ast.Node) bool {
			assign, ok := inner.(*ast.AssignStmt)
			if !ok {
				return true
			}

			for _, rhs := range assign.Rhs {
				call, ok := rhs.(*ast.CallExpr)
				if !ok {
					continue
				}
				ident, ok := call.Fun.(*ast.Ident)
				if !ok || ident.Name != "append" {
					continue
				}

				// Check if this slice was preallocated
				// This is a simplified check - would need data flow analysis for full accuracy
				results = append(results, AppendInLoopInfo{
					Node: call,
					Pos:  fset.Position(call.Pos()),
				})
			}
			return true
		})

		return true
	})

	return results
}

type AppendInLoopInfo struct {
	Node *ast.CallExpr
	Pos  token.Position
}

// FindStringConcatInLoop finds string concatenation in loops
func FindStringConcatInLoop(file *ast.File, fset *token.FileSet) []token.Position {
	results := make([]token.Position, 0, 4)

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

			// Check if RHS involves strings (simplified check)
			for _, rhs := range assign.Rhs {
				if isStringExpr(rhs) {
					results = append(results, fset.Position(assign.Pos()))
				}
			}
			return true
		})

		return true
	})

	return results
}

// FindSQLInLoop finds database query patterns inside loops
func FindSQLInLoop(file *ast.File, fset *token.FileSet) []SQLInLoopInfo {
	results := make([]SQLInLoopInfo, 0, 4)

	sqlMethods := map[string]bool{
		"Query":           true,
		"QueryRow":        true,
		"Exec":            true,
		"QueryRowContext": true,
		"QueryContext":    true,
		"ExecContext":     true,
		"Get":             true,
		"Select":          true,
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

		// Find SQL method calls in the loop body
		ast.Inspect(loopBody, func(inner ast.Node) bool {
			call, ok := inner.(*ast.CallExpr)
			if !ok {
				return true
			}

			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			if sqlMethods[sel.Sel.Name] {
				results = append(results, SQLInLoopInfo{
					Method: sel.Sel.Name,
					Pos:    fset.Position(call.Pos()),
				})
			}
			return true
		})

		return true
	})

	return results
}

type SQLInLoopInfo struct {
	Method string
	Pos    token.Position
}

// FindUnbufferedChannels finds unbuffered channel creation
func FindUnbufferedChannels(file *ast.File, fset *token.FileSet) []token.Position {
	results := make([]token.Position, 0, 4)

	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		ident, ok := call.Fun.(*ast.Ident)
		if !ok || ident.Name != "make" {
			return true
		}

		if len(call.Args) < 1 {
			return true
		}

		// Check if it's a channel type
		_, ok = call.Args[0].(*ast.ChanType)
		if !ok {
			return true
		}

		// Unbuffered if no second argument or second arg is 0
		if len(call.Args) == 1 {
			results = append(results, fset.Position(call.Pos()))
		} else if len(call.Args) >= 2 {
			if lit, ok := call.Args[1].(*ast.BasicLit); ok {
				if lit.Kind == token.INT && lit.Value == "0" {
					results = append(results, fset.Position(call.Pos()))
				}
			}
		}

		return true
	})

	return results
}

// FindMutexHotspots counts mutex lock calls per function
func FindMutexHotspots(file *ast.File, fset *token.FileSet) map[string]int {
	lockCounts := make(map[string]int)
	currentFunc := ""

	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncDecl:
			if node.Name != nil {
				currentFunc = node.Name.Name
			}
		case *ast.CallExpr:
			sel, ok := node.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if sel.Sel.Name == "Lock" || sel.Sel.Name == "RLock" {
				if currentFunc != "" {
					lockCounts[currentFunc]++
				}
			}
		}
		return true
	})

	return lockCounts
}

// FindJSONInLoop finds JSON marshal/unmarshal calls in loops
func FindJSONInLoop(file *ast.File, fset *token.FileSet) []JSONInLoopInfo {
	results := make([]JSONInLoopInfo, 0, 4)

	jsonFuncs := map[string]bool{
		"Marshal":   true,
		"Unmarshal": true,
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

			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			// Check for json.Marshal/Unmarshal
			if ident, ok := sel.X.(*ast.Ident); ok {
				if ident.Name == "json" && jsonFuncs[sel.Sel.Name] {
					results = append(results, JSONInLoopInfo{
						Method: sel.Sel.Name,
						Pos:    fset.Position(call.Pos()),
					})
				}
			}
			return true
		})

		return true
	})

	return results
}

type JSONInLoopInfo struct {
	Method string
	Pos    token.Position
}
