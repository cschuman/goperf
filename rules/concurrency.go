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
type UnbufferedChannelRule struct{}

func (r *UnbufferedChannelRule) Name() string     { return "unbuffered-channel" }
func (r *UnbufferedChannelRule) Category() string { return "concurrency" }

func (r *UnbufferedChannelRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
	var issues []Issue

	positions := FindUnbufferedChannels(file, fset)
	for _, pos := range positions {
		issues = append(issues, Issue{
			Rule:     r.Name(),
			Category: r.Category(),
			Severity: SeverityLow,
			Line:     pos.Line,
			Column:   pos.Column,
			Message:  "Unbuffered channel - may cause goroutine blocking",
			Why:      "Unbuffered channels block the sender until a receiver is ready. This can cause deadlocks or reduce parallelism if not carefully designed.",
			Fix:      "Consider adding a buffer: make(chan T, bufferSize). Use unbuffered only when synchronization is intentional.",
		})
	}

	return issues
}

// MutexInLoopRule detects mutex Lock() calls inside loops
type MutexInLoopRule struct{}

func (r *MutexInLoopRule) Name() string     { return "mutex-in-loop" }
func (r *MutexInLoopRule) Category() string { return "concurrency" }

func (r *MutexInLoopRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
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
				pos := fset.Position(call.Pos())
				issues = append(issues, Issue{
					Rule:     r.Name(),
					Category: r.Category(),
					Severity: SeverityMedium,
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
				if node.Name == "ctx" || node.Name == "done" || node.Name == "cancel" || node.Name == "quit" {
					hasContextOrDone = true
				}
			case *ast.SelectorExpr:
				if ident, ok := node.X.(*ast.Ident); ok {
					if ident.Name == "ctx" || node.Sel.Name == "Done" {
						hasContextOrDone = true
					}
				}
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
