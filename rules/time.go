package rules

import (
	"go/ast"
	"go/token"
)

func init() {
	RegisterRule("allocation", &TimeParseInLoopRule{})
	RegisterRule("allocation", &TimeLocationInLoopRule{})
	RegisterRule("io", &TimeFormatInLoopRule{})
}

// TimeParseInLoopRule detects time.Parse in loops
type TimeParseInLoopRule struct{}

func (r *TimeParseInLoopRule) Name() string     { return "time-parse-in-loop" }
func (r *TimeParseInLoopRule) Category() string { return "allocation" }

func (r *TimeParseInLoopRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
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

			if ident.Name == "time" && sel.Sel.Name == "Parse" {
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
					Message:  "time.Parse() in loop - repeated parsing overhead",
					Why:      "time.Parse parses the layout string on each call. While not as expensive as regexp.Compile, it still adds up in hot loops.",
					Fix:      "If parsing the same format, consider: (1) Caching parsed time.Location, (2) Using time.ParseInLocation with cached location",
				})
			}

			return true
		})

		return true
	})

	return issues
}

// TimeLocationInLoopRule detects time.LoadLocation in loops
type TimeLocationInLoopRule struct{}

func (r *TimeLocationInLoopRule) Name() string     { return "time-location-in-loop" }
func (r *TimeLocationInLoopRule) Category() string { return "allocation" }

func (r *TimeLocationInLoopRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
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

			if ident.Name == "time" && sel.Sel.Name == "LoadLocation" {
				pos := fset.Position(call.Pos())
				issues = append(issues, Issue{
					Rule:     r.Name(),
					Category: r.Category(),
					Severity: SeverityMedium,
					Line:     pos.Line,
					Column:   pos.Column,
					Message:  "time.LoadLocation() in loop - expensive I/O operation",
					Why:      "LoadLocation reads timezone data from disk or tzdata. This is slow and should be cached.",
					Fix:      "Cache the location: var loc, _ = time.LoadLocation(\"America/New_York\") at package level or function start",
				})
			}

			return true
		})

		return true
	})

	return issues
}

// TimeFormatInLoopRule detects time.Time.Format with complex layouts in loops
type TimeFormatInLoopRule struct{}

func (r *TimeFormatInLoopRule) Name() string     { return "time-format-loop" }
func (r *TimeFormatInLoopRule) Category() string { return "io" }

func (r *TimeFormatInLoopRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
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

		// Count Format calls in loop
		formatCount := 0
		var lastFormatPos token.Position

		ast.Inspect(loopBody, func(inner ast.Node) bool {
			call, ok := inner.(*ast.CallExpr)
			if !ok {
				return true
			}

			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			if sel.Sel.Name == "Format" {
				formatCount++
				lastFormatPos = fset.Position(call.Pos())
			}

			return true
		})

		// Only flag if multiple format calls in same loop
		if formatCount > 1 {
			issues = append(issues, Issue{
				Rule:     r.Name(),
				Category: r.Category(),
				Severity: SeverityLow,
				Line:     lastFormatPos.Line,
				Column:   lastFormatPos.Column,
				Message:  "Multiple time.Format() calls in loop - consider caching or batching",
				Why:      "time.Format allocates strings. Multiple calls per iteration multiply allocations.",
				Fix:      "Consider: (1) Format once with all needed data, (2) Use strconv for simple number conversions, (3) Use a strings.Builder",
			})
		}

		return true
	})

	return issues
}
