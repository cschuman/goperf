package rules

import (
	"go/ast"
	"go/token"
)

func init() {
	RegisterRule("io", &JSONInLoopRule{})
	RegisterRule("io", &HTTPClientCreationRule{})
	RegisterRule("io", &ReadAllRule{})
}

// JSONInLoopRule detects JSON marshal/unmarshal in loops
type JSONInLoopRule struct{}

func (r *JSONInLoopRule) Name() string     { return "json-in-loop" }
func (r *JSONInLoopRule) Category() string { return "io" }

func (r *JSONInLoopRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
	var issues []Issue

	results := FindJSONInLoop(file, fset)
	for _, result := range results {
		issues = append(issues, Issue{
			Rule:     r.Name(),
			Category: r.Category(),
			Severity: SeverityMedium,
			Line:     result.Pos.Line,
			Column:   result.Pos.Column,
			Message:  "json." + result.Method + "() inside loop - reflection overhead",
			Why:      "JSON encoding uses reflection, which is slow. In a loop, this overhead multiplies. Each call also allocates memory.",
			Fix:      "Consider: (1) Processing in batches, (2) Using code-generated encoders (easyjson, ffjson), (3) Streaming with json.Encoder for large datasets",
		})
	}

	return issues
}

// HTTPClientCreationRule detects http.Client{} created inside loops or functions
type HTTPClientCreationRule struct{}

func (r *HTTPClientCreationRule) Name() string     { return "http-client-creation" }
func (r *HTTPClientCreationRule) Category() string { return "io" }

func (r *HTTPClientCreationRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
	var issues []Issue

	ast.Inspect(file, func(n ast.Node) bool {
		// Look for &http.Client{} or http.Client{}
		compLit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}

		sel, ok := compLit.Type.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		ident, ok := sel.X.(*ast.Ident)
		if !ok || ident.Name != "http" || sel.Sel.Name != "Client" {
			return true
		}

		// Check if this is inside a function (not package level)
		// This is a simplified check - ideally we'd track scope
		pos := fset.Position(compLit.Pos())
		issues = append(issues, Issue{
			Rule:     r.Name(),
			Category: r.Category(),
			Severity: SeverityMedium,
			Line:     pos.Line,
			Column:   pos.Column,
			Message:  "http.Client created - ensure reuse across requests",
			Why:      "Creating new http.Client for each request wastes connection pooling benefits. Each client maintains its own connection pool and transport.",
			Fix:      "Create http.Client once at package level or in init, then reuse. Configure Transport for connection pooling.",
		})

		return true
	})

	return issues
}

// ReadAllRule detects ioutil.ReadAll/io.ReadAll that could use streaming
type ReadAllRule struct{}

func (r *ReadAllRule) Name() string     { return "read-all" }
func (r *ReadAllRule) Category() string { return "io" }

func (r *ReadAllRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
	var issues []Issue

	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		// Check for io.ReadAll or ioutil.ReadAll
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		ident, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}

		if (ident.Name == "io" || ident.Name == "ioutil") && sel.Sel.Name == "ReadAll" {
			pos := fset.Position(call.Pos())
			issues = append(issues, Issue{
				Rule:     r.Name(),
				Category: r.Category(),
				Severity: SeverityLow,
				Line:     pos.Line,
				Column:   pos.Column,
				Message:  "ReadAll() loads entire content into memory",
				Why:      "For large files or responses, ReadAll allocates potentially huge buffers. This can cause OOM for large inputs.",
				Fix:      "Consider streaming: io.Copy(), bufio.Scanner, or json.Decoder for JSON. Process data in chunks when possible.",
			})
		}

		return true
	})

	return issues
}
