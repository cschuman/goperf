package rules

import (
	"go/ast"
	"go/token"
)

func init() {
	RegisterRule("io", &MissingMaxBytesReaderRule{})
	RegisterRule("io", &MissingBodyCloseRule{})
	RegisterRule("io", &ResponseWriterBufferingRule{})
}

// MissingMaxBytesReaderRule detects reading request body without size limit
type MissingMaxBytesReaderRule struct{}

func (r *MissingMaxBytesReaderRule) Name() string     { return "missing-max-bytes-reader" }
func (r *MissingMaxBytesReaderRule) Category() string { return "io" }

func (r *MissingMaxBytesReaderRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
	issues := make([]Issue, 0, 4)

	ast.Inspect(file, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok {
			return true
		}

		// Check if this looks like an HTTP handler
		if !isHTTPHandler(funcDecl) {
			return true
		}

		if funcDecl.Body == nil {
			return true
		}

		// Track if MaxBytesReader is used
		hasMaxBytesReader := false
		var bodyReadPos *token.Position

		ast.Inspect(funcDecl.Body, func(inner ast.Node) bool {
			call, ok := inner.(*ast.CallExpr)
			if !ok {
				return true
			}

			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			// Check for http.MaxBytesReader
			if ident, ok := sel.X.(*ast.Ident); ok {
				if ident.Name == "http" && sel.Sel.Name == "MaxBytesReader" {
					hasMaxBytesReader = true
				}
			}

			// Check for reading request body
			if sel.Sel.Name == "ReadAll" || sel.Sel.Name == "Copy" || sel.Sel.Name == "Decode" {
				// Check if argument is r.Body
				for _, arg := range call.Args {
					if argSel, ok := arg.(*ast.SelectorExpr); ok {
						if argSel.Sel.Name == "Body" {
							pos := fset.Position(call.Pos())
							bodyReadPos = &pos
						}
					}
				}
			}

			// Check for json.NewDecoder(r.Body)
			if ident, ok := sel.X.(*ast.Ident); ok {
				if ident.Name == "json" && sel.Sel.Name == "NewDecoder" {
					for _, arg := range call.Args {
						if argSel, ok := arg.(*ast.SelectorExpr); ok {
							if argSel.Sel.Name == "Body" {
								pos := fset.Position(call.Pos())
								bodyReadPos = &pos
							}
						}
					}
				}
			}

			return true
		})

		// If body is read without MaxBytesReader, flag it
		if bodyReadPos != nil && !hasMaxBytesReader {
			issues = append(issues, Issue{
				Rule:     r.Name(),
				Category: r.Category(),
				Severity: SeverityMedium,
				Line:     bodyReadPos.Line,
				Column:   bodyReadPos.Column,
				Message:  "Request body read without size limit - potential DoS vulnerability",
				Why:      "Without http.MaxBytesReader, clients can send arbitrarily large bodies causing OOM. This is a denial-of-service vector.",
				Fix:      "Wrap body with limit: r.Body = http.MaxBytesReader(w, r.Body, maxBytes)",
			})
		}

		return true
	})

	return issues
}

// MissingBodyCloseRule detects HTTP response bodies that aren't closed
type MissingBodyCloseRule struct{}

func (r *MissingBodyCloseRule) Name() string     { return "missing-body-close" }
func (r *MissingBodyCloseRule) Category() string { return "io" }

func (r *MissingBodyCloseRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
	issues := make([]Issue, 0, 4)

	ast.Inspect(file, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok {
			return true
		}

		if funcDecl.Body == nil {
			return true
		}

		// Find HTTP client calls that return responses
		type respInfo struct {
			varName string
			pos     token.Position
		}
		responses := make([]respInfo, 0, 4)

		ast.Inspect(funcDecl.Body, func(inner ast.Node) bool {
			assign, ok := inner.(*ast.AssignStmt)
			if !ok {
				return true
			}

			for i, rhs := range assign.Rhs {
				call, ok := rhs.(*ast.CallExpr)
				if !ok {
					continue
				}

				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					continue
				}

				// Check for http.Get, client.Do, etc.
				httpMethods := map[string]bool{
					"Get":      true,
					"Post":     true,
					"PostForm": true,
					"Head":     true,
					"Do":       true,
				}

				if httpMethods[sel.Sel.Name] {
					if i < len(assign.Lhs) {
						if ident, ok := assign.Lhs[i].(*ast.Ident); ok {
							responses = append(responses, respInfo{
								varName: ident.Name,
								pos:     fset.Position(assign.Pos()),
							})
						}
					}
				}
			}

			return true
		})

		// Check if response bodies are closed
		for _, resp := range responses {
			closed := false

			ast.Inspect(funcDecl.Body, func(inner ast.Node) bool {
				// Check for resp.Body.Close() or defer resp.Body.Close()
				call, ok := inner.(*ast.CallExpr)
				if !ok {
					return true
				}

				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}

				if sel.Sel.Name == "Close" {
					if innerSel, ok := sel.X.(*ast.SelectorExpr); ok {
						if innerSel.Sel.Name == "Body" {
							if ident, ok := innerSel.X.(*ast.Ident); ok {
								if ident.Name == resp.varName {
									closed = true
									return false
								}
							}
						}
					}
				}

				return true
			})

			if !closed {
				issues = append(issues, Issue{
					Rule:     r.Name(),
					Category: r.Category(),
					Severity: SeverityHigh,
					Line:     resp.pos.Line,
					Column:   resp.pos.Column,
					Message:  "HTTP response body not closed - connection leak",
					Why:      "Not closing response bodies leaks connections. HTTP keep-alive connections stay open, exhausting the connection pool.",
					Fix:      "Add: defer " + resp.varName + ".Body.Close() (after checking error)",
				})
			}
		}

		return true
	})

	return issues
}

// ResponseWriterBufferingRule detects large writes to ResponseWriter without Flush
type ResponseWriterBufferingRule struct{}

func (r *ResponseWriterBufferingRule) Name() string     { return "response-writer-buffering" }
func (r *ResponseWriterBufferingRule) Category() string { return "io" }

func (r *ResponseWriterBufferingRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
	issues := make([]Issue, 0, 4)

	ast.Inspect(file, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok {
			return true
		}

		// Check if this looks like an HTTP handler
		if !isHTTPHandler(funcDecl) {
			return true
		}

		if funcDecl.Body == nil {
			return true
		}

		// Check for large data streaming without flush
		var loopWritePos *token.Position
		hasFlusher := false

		ast.Inspect(funcDecl.Body, func(inner ast.Node) bool {
			// Check for type assertion to Flusher
			typeAssert, ok := inner.(*ast.TypeAssertExpr)
			if ok {
				if sel, ok := typeAssert.Type.(*ast.SelectorExpr); ok {
					if sel.Sel.Name == "Flusher" {
						hasFlusher = true
					}
				}
			}

			// Look for Write in loops
			var loopBody *ast.BlockStmt
			switch stmt := inner.(type) {
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

			ast.Inspect(loopBody, func(stmt ast.Node) bool {
				call, ok := stmt.(*ast.CallExpr)
				if !ok {
					return true
				}

				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}

				if sel.Sel.Name == "Write" || sel.Sel.Name == "WriteString" {
					// Check if receiver is the response writer (typically w)
					if ident, ok := sel.X.(*ast.Ident); ok {
						if ident.Name == "w" {
							pos := fset.Position(call.Pos())
							loopWritePos = &pos
						}
					}
				}

				return true
			})

			return true
		})

		// If writing in loop without flusher, suggest it
		if loopWritePos != nil && !hasFlusher {
			issues = append(issues, Issue{
				Rule:     r.Name(),
				Category: r.Category(),
				Severity: SeverityLow,
				Line:     loopWritePos.Line,
				Column:   loopWritePos.Column,
				Message:  "Streaming response without Flush - data may buffer until handler returns",
				Why:      "ResponseWriter buffers data. For streaming/SSE, data won't reach the client until the buffer fills or handler returns.",
				Fix:      "For streaming: if f, ok := w.(http.Flusher); ok { f.Flush() } after each chunk",
			})
		}

		return true
	})

	return issues
}
