package rules

import (
	"go/ast"
	"go/token"
)

func init() {
	RegisterRule("concurrency", &UnbufferedChannelRule{})
	RegisterRule("concurrency", &MutexInLoopRule{})
	RegisterRule("concurrency", &GoroutineLeakRule{})
}

// UnbufferedChannelRule detects unbuffered channel creation
// Now smarter: checks for intentional synchronization patterns
type UnbufferedChannelRule struct{}

func (r *UnbufferedChannelRule) Name() string     { return "unbuffered-channel" }
func (r *UnbufferedChannelRule) Category() string { return "concurrency" }

func (r *UnbufferedChannelRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
	var issues []Issue

	ast.Inspect(file, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok {
			return true
		}

		if funcDecl.Body == nil {
			return true
		}

		// Find unbuffered channels and their variable names
		unbufferedChans := findUnbufferedChannelVars(funcDecl.Body, fset)

		// Find channels used in select statements (intentional synchronization)
		selectChans := findChannelsInSelect(funcDecl.Body)

		// Find channels used with proper goroutine coordination (done/signal patterns)
		signalChans := findSignalChannels(funcDecl.Body)

		// Find channels that are struct{} type (typically signals)
		emptyStructChans := findEmptyStructChannels(funcDecl.Body)

		// Report only channels that are NOT in select and NOT used as signals
		for name, pos := range unbufferedChans {
			// Skip if used in select (intentional blocking)
			if selectChans[name] {
				continue
			}

			// Skip signal/done pattern channels
			if signalChans[name] {
				continue
			}

			// Skip chan struct{} - almost always intentional signals
			if emptyStructChans[name] {
				continue
			}

			// Lower severity - could be intentional
			severity := SeverityLow
			message := "Unbuffered channel '" + name + "' - verify synchronization is intentional"
			why := "Unbuffered channels block the sender until a receiver is ready. This is correct for synchronization but may cause deadlocks if misused."
			fix := "If synchronization is intentional, add comment: // Intentional: synchronization point. Otherwise, consider adding a buffer: make(chan T, size)"

			issues = append(issues, Issue{
				Rule:     r.Name(),
				Category: r.Category(),
				Severity: severity,
				Line:     pos.Line,
				Column:   pos.Column,
				Message:  message,
				Why:      why,
				Fix:      fix,
			})
		}

		return true
	})

	return issues
}

// findUnbufferedChannelVars finds variables that hold unbuffered channels
func findUnbufferedChannelVars(body *ast.BlockStmt, fset *token.FileSet) map[string]token.Position {
	chans := make(map[string]token.Position)

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

			// Check if it's a channel type
			_, ok = call.Args[0].(*ast.ChanType)
			if !ok {
				continue
			}

			// Unbuffered if no second argument or second arg is 0
			isUnbuffered := false
			if len(call.Args) == 1 {
				isUnbuffered = true
			} else if len(call.Args) >= 2 {
				if lit, ok := call.Args[1].(*ast.BasicLit); ok {
					if lit.Kind == token.INT && lit.Value == "0" {
						isUnbuffered = true
					}
				}
			}

			if isUnbuffered && i < len(assign.Lhs) {
				if lhsIdent, ok := assign.Lhs[i].(*ast.Ident); ok {
					chans[lhsIdent.Name] = fset.Position(call.Pos())
				}
			}
		}

		return true
	})

	return chans
}

// findChannelsInSelect finds channel variables used in select statements
func findChannelsInSelect(body *ast.BlockStmt) map[string]bool {
	selectChans := make(map[string]bool)

	ast.Inspect(body, func(n ast.Node) bool {
		selectStmt, ok := n.(*ast.SelectStmt)
		if !ok {
			return true
		}

		// Check each case in the select
		for _, stmt := range selectStmt.Body.List {
			commClause, ok := stmt.(*ast.CommClause)
			if !ok {
				continue
			}

			// Extract channel from comm statement
			switch comm := commClause.Comm.(type) {
			case *ast.SendStmt:
				if ident, ok := comm.Chan.(*ast.Ident); ok {
					selectChans[ident.Name] = true
				}
			case *ast.ExprStmt:
				if unary, ok := comm.X.(*ast.UnaryExpr); ok && unary.Op == token.ARROW {
					if ident, ok := unary.X.(*ast.Ident); ok {
						selectChans[ident.Name] = true
					}
				}
			case *ast.AssignStmt:
				for _, rhs := range comm.Rhs {
					if unary, ok := rhs.(*ast.UnaryExpr); ok && unary.Op == token.ARROW {
						if ident, ok := unary.X.(*ast.Ident); ok {
							selectChans[ident.Name] = true
						}
					}
				}
			}
		}

		return true
	})

	return selectChans
}

// findSignalChannels finds channels named with common signal patterns
func findSignalChannels(body *ast.BlockStmt) map[string]bool {
	signals := make(map[string]bool)

	signalNames := map[string]bool{
		"done": true, "quit": true, "stop": true, "cancel": true,
		"sig": true, "signal": true, "shutdown": true, "closed": true,
		"ready": true, "started": true, "finished": true,
	}

	ast.Inspect(body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}

		for i := range assign.Rhs {
			if i < len(assign.Lhs) {
				if ident, ok := assign.Lhs[i].(*ast.Ident); ok {
					if signalNames[ident.Name] {
						signals[ident.Name] = true
					}
				}
			}
		}

		return true
	})

	return signals
}

// findEmptyStructChannels finds channels of type chan struct{}
func findEmptyStructChannels(body *ast.BlockStmt) map[string]bool {
	emptyStructChans := make(map[string]bool)

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

			chanType, ok := call.Args[0].(*ast.ChanType)
			if !ok {
				continue
			}

			// Check if it's chan struct{}
			if structType, ok := chanType.Value.(*ast.StructType); ok {
				if structType.Fields == nil || len(structType.Fields.List) == 0 {
					if i < len(assign.Lhs) {
						if lhsIdent, ok := assign.Lhs[i].(*ast.Ident); ok {
							emptyStructChans[lhsIdent.Name] = true
						}
					}
				}
			}
		}

		return true
	})

	return emptyStructChans
}

// MutexInLoopRule detects mutex Lock() calls inside loops
type MutexInLoopRule struct{}

func (r *MutexInLoopRule) Name() string     { return "mutex-in-loop" }
func (r *MutexInLoopRule) Category() string { return "concurrency" }

func (r *MutexInLoopRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
	var issues []Issue

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

		// Find Lock() calls in the loop body
		ast.Inspect(loopBody, func(inner ast.Node) bool {
			call, ok := inner.(*ast.CallExpr)
			if !ok {
				return true
			}

			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			if sel.Sel.Name == "Lock" || sel.Sel.Name == "RLock" {
				severity := SeverityMedium

				// Small bounded loops are less severe
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
					Message:  "Mutex " + sel.Sel.Name + "() inside loop - potential contention",
					Why:      "Acquiring/releasing locks repeatedly in a loop adds overhead and can cause contention. Other goroutines may be blocked waiting.",
					Fix:      "Consider: (1) Moving lock outside loop if safe, (2) Batching operations, (3) Using sync.Map for concurrent map access, (4) Reducing critical section size",
				})
			}
			return true
		})

		return true
	})

	return issues
}

// GoroutineLeakRule detects goroutines started without clear termination
type GoroutineLeakRule struct{}

func (r *GoroutineLeakRule) Name() string     { return "goroutine-leak" }
func (r *GoroutineLeakRule) Category() string { return "concurrency" }

func (r *GoroutineLeakRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
	var issues []Issue

	ast.Inspect(file, func(n ast.Node) bool {
		goStmt, ok := n.(*ast.GoStmt)
		if !ok {
			return true
		}

		// Check if the goroutine has context or done channel
		hasContextOrDone := false

		ast.Inspect(goStmt.Call, func(inner ast.Node) bool {
			switch node := inner.(type) {
			case *ast.Ident:
				if node.Name == "ctx" || node.Name == "done" || node.Name == "cancel" || node.Name == "quit" || node.Name == "stop" {
					hasContextOrDone = true
				}
			case *ast.SelectorExpr:
				if ident, ok := node.X.(*ast.Ident); ok {
					if ident.Name == "ctx" || node.Sel.Name == "Done" {
						hasContextOrDone = true
					}
				}
			case *ast.SelectStmt:
				// Has a select statement - likely has proper termination
				hasContextOrDone = true
			}
			return true
		})

		// Check if it's a simple infinite loop pattern
		if funcLit, ok := goStmt.Call.Fun.(*ast.FuncLit); ok {
			if hasInfiniteLoop(funcLit.Body) && !hasContextOrDone {
				pos := fset.Position(goStmt.Pos())
				issues = append(issues, Issue{
					Rule:     r.Name(),
					Category: r.Category(),
					Severity: SeverityHigh,
					Line:     pos.Line,
					Column:   pos.Column,
					Message:  "Goroutine with potential infinite loop and no cancellation mechanism",
					Why:      "Goroutines without termination conditions leak memory and CPU. They persist even after the parent function returns.",
					Fix:      "Add context.Context or done channel: select { case <-ctx.Done(): return; case ... }",
				})
			}
		}

		return true
	})

	return issues
}

func hasInfiniteLoop(body *ast.BlockStmt) bool {
	for _, stmt := range body.List {
		if forStmt, ok := stmt.(*ast.ForStmt); ok {
			// for { } without condition is infinite
			if forStmt.Cond == nil {
				return true
			}
		}
	}
	return false
}
