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
// Now smarter: recognizes json.Encoder which reuses reflection cache
type JSONInLoopRule struct{}

func (r *JSONInLoopRule) Name() string     { return "json-in-loop" }
func (r *JSONInLoopRule) Category() string { return "io" }

func (r *JSONInLoopRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
	var issues []Issue

	ast.Inspect(file, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok {
			return true
		}

		if funcDecl.Body == nil {
			return true
		}

		// Find json.Encoder variables (more efficient than Marshal in loops)
		encoders := findJSONEncoders(funcDecl.Body)
		decoders := findJSONDecoders(funcDecl.Body)

		// Find loops and check for JSON operations
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

			ast.Inspect(loopBody, func(stmt ast.Node) bool {
				call, ok := stmt.(*ast.CallExpr)
				if !ok {
					return true
				}

				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}

				// Check for json.Marshal/Unmarshal
				if ident, ok := sel.X.(*ast.Ident); ok {
					if ident.Name == "json" && (sel.Sel.Name == "Marshal" || sel.Sel.Name == "Unmarshal") {
						severity := SeverityMedium
						if loopBound > 0 && loopBound <= 10 {
							severity = SeverityLow
						}

						pos := fset.Position(call.Pos())
						issues = append(issues, Issue{
							Rule:     r.Name(),
							Category: r.Category(),
							Severity: severity,
							Line:     pos.Line,
							Column:   pos.Column,
							Message:  "json." + sel.Sel.Name + "() inside loop - reflection overhead",
							Why:      "JSON encoding uses reflection, which is slow. In a loop, this overhead multiplies. Each call also allocates memory.",
							Fix:      "Consider: (1) Use json.Encoder/json.Decoder which cache reflection, (2) Process in batches, (3) Use code-generated encoders (easyjson, ffjson)",
						})
					}
				}

				// Check for Encode/Decode on json.Encoder/Decoder - this is actually fine
				// We just want to make sure they're not creating new encoders in the loop
				if sel.Sel.Name == "Encode" {
					receiver := getReceiverName(sel.X)
					if !encoders[receiver] {
						// Could be creating encoder inside loop
						// Check if the receiver is json.NewEncoder call
						if isNewEncoderCall(sel.X) {
							pos := fset.Position(call.Pos())
							issues = append(issues, Issue{
								Rule:     r.Name(),
								Category: r.Category(),
								Severity: SeverityMedium,
								Line:     pos.Line,
								Column:   pos.Column,
								Message:  "json.NewEncoder().Encode() inside loop - create encoder once outside",
								Why:      "Creating a new encoder for each iteration wastes the reflection caching benefit. Create the encoder once before the loop.",
								Fix:      "Move encoder creation outside the loop: enc := json.NewEncoder(w); for ... { enc.Encode(item) }",
							})
						}
					}
				}

				if sel.Sel.Name == "Decode" {
					receiver := getReceiverName(sel.X)
					if !decoders[receiver] {
						if isNewDecoderCall(sel.X) {
							pos := fset.Position(call.Pos())
							issues = append(issues, Issue{
								Rule:     r.Name(),
								Category: r.Category(),
								Severity: SeverityMedium,
								Line:     pos.Line,
								Column:   pos.Column,
								Message:  "json.NewDecoder().Decode() inside loop - create decoder once outside",
								Why:      "Creating a new decoder for each iteration wastes the reflection caching benefit.",
								Fix:      "Move decoder creation outside the loop if reading from a single source",
							})
						}
					}
				}

				return true
			})

			return true
		})

		return true
	})

	return issues
}

// findJSONEncoders finds variables that hold json.Encoder
func findJSONEncoders(body *ast.BlockStmt) map[string]bool {
	encoders := make(map[string]bool)

	ast.Inspect(body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}

		for i, rhs := range assign.Rhs {
			if isNewEncoderCall(rhs) {
				if i < len(assign.Lhs) {
					if ident, ok := assign.Lhs[i].(*ast.Ident); ok {
						encoders[ident.Name] = true
					}
				}
			}
		}

		return true
	})

	return encoders
}

// findJSONDecoders finds variables that hold json.Decoder
func findJSONDecoders(body *ast.BlockStmt) map[string]bool {
	decoders := make(map[string]bool)

	ast.Inspect(body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}

		for i, rhs := range assign.Rhs {
			if isNewDecoderCall(rhs) {
				if i < len(assign.Lhs) {
					if ident, ok := assign.Lhs[i].(*ast.Ident); ok {
						decoders[ident.Name] = true
					}
				}
			}
		}

		return true
	})

	return decoders
}

func isNewEncoderCall(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	return ident.Name == "json" && sel.Sel.Name == "NewEncoder"
}

func isNewDecoderCall(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	return ident.Name == "json" && sel.Sel.Name == "NewDecoder"
}

// HTTPClientCreationRule detects http.Client{} created inside functions (not reused)
type HTTPClientCreationRule struct{}

func (r *HTTPClientCreationRule) Name() string     { return "http-client-creation" }
func (r *HTTPClientCreationRule) Category() string { return "io" }

func (r *HTTPClientCreationRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
	var issues []Issue

	// Track package-level http.Client declarations (those are fine)
	packageLevelClients := findPackageLevelHTTPClients(file)

	ast.Inspect(file, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok {
			return true
		}

		if funcDecl.Body == nil {
			return true
		}

		// Look for http.Client creation inside functions
		ast.Inspect(funcDecl.Body, func(inner ast.Node) bool {
			// Look for &http.Client{} or http.Client{}
			compLit, ok := inner.(*ast.CompositeLit)
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

			// Check if this is assigned to a package-level variable (acceptable)
			// This is a simplified check - we mainly care about function-local creation

			pos := fset.Position(compLit.Pos())

			// Determine severity based on context
			severity := SeverityMedium
			message := "http.Client created inside function - ensure reuse across requests"
			fix := "Create http.Client once at package level or in init, then reuse. Configure Transport for connection pooling."

			// Check if this is inside a loop - that's worse
			if isInsideLoop(funcDecl.Body, compLit) {
				severity = SeverityHigh
				message = "http.Client created inside loop - significant overhead"
				fix = "Move http.Client creation outside the loop. Create once and reuse for all requests."
			}

			// Skip if there are package-level clients (user probably knows what they're doing)
			if len(packageLevelClients) > 0 {
				severity = SeverityLow
			}

			issues = append(issues, Issue{
				Rule:     r.Name(),
				Category: r.Category(),
				Severity: severity,
				Line:     pos.Line,
				Column:   pos.Column,
				Message:  message,
				Why:      "Creating new http.Client for each request wastes connection pooling benefits. Each client maintains its own connection pool and transport.",
				Fix:      fix,
			})

			return true
		})

		return true
	})

	return issues
}

func findPackageLevelHTTPClients(file *ast.File) []string {
	var clients []string

	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.VAR {
			continue
		}

		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}

			// Check type
			if sel, ok := valueSpec.Type.(*ast.SelectorExpr); ok {
				if ident, ok := sel.X.(*ast.Ident); ok {
					if ident.Name == "http" && sel.Sel.Name == "Client" {
						for _, name := range valueSpec.Names {
							clients = append(clients, name.Name)
						}
					}
				}
			}

			// Check values (for var client = &http.Client{})
			for i, val := range valueSpec.Values {
				if isHTTPClientLiteral(val) && i < len(valueSpec.Names) {
					clients = append(clients, valueSpec.Names[i].Name)
				}
			}
		}
	}

	return clients
}

func isHTTPClientLiteral(expr ast.Expr) bool {
	// Handle &http.Client{}
	if unary, ok := expr.(*ast.UnaryExpr); ok && unary.Op == token.AND {
		expr = unary.X
	}

	compLit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return false
	}

	sel, ok := compLit.Type.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}

	return ident.Name == "http" && sel.Sel.Name == "Client"
}

func isInsideLoop(body *ast.BlockStmt, target ast.Node) bool {
	inside := false

	ast.Inspect(body, func(n ast.Node) bool {
		switch n.(type) {
		case *ast.RangeStmt, *ast.ForStmt:
			// Check if target is inside this loop
			ast.Inspect(n, func(inner ast.Node) bool {
				if inner == target {
					inside = true
					return false
				}
				return true
			})
		}
		return !inside
	})

	return inside
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
			// Check context - is this reading from http response body?
			severity := SeverityLow
			why := "For large files or responses, ReadAll allocates potentially huge buffers. This can cause OOM for large inputs."
			fix := "Consider streaming: io.Copy(), bufio.Scanner, or json.Decoder for JSON. Process data in chunks when possible."

			// Check if reading HTTP response (common pattern)
			if len(call.Args) > 0 {
				if sel, ok := call.Args[0].(*ast.SelectorExpr); ok {
					if sel.Sel.Name == "Body" {
						severity = SeverityMedium
						why = "Reading entire HTTP response body into memory. For large responses, this can cause memory issues."
						fix = "Consider: (1) Setting Content-Length limits, (2) Using io.LimitReader, (3) Streaming with json.Decoder for JSON responses"
					}
				}
			}

			pos := fset.Position(call.Pos())
			issues = append(issues, Issue{
				Rule:     r.Name(),
				Category: r.Category(),
				Severity: severity,
				Line:     pos.Line,
				Column:   pos.Column,
				Message:  "ReadAll() loads entire content into memory",
				Why:      why,
				Fix:      fix,
			})
		}

		return true
	})

	return issues
}
