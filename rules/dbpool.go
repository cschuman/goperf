package rules

import (
	"go/ast"
	"go/token"
)

func init() {
	RegisterRule("database", &MissingConnectionPoolRule{})
	RegisterRule("database", &UnlimitedConnectionPoolRule{})
}

// MissingConnectionPoolRule detects sql.Open without pool configuration
type MissingConnectionPoolRule struct{}

func (r *MissingConnectionPoolRule) Name() string     { return "missing-connection-pool-config" }
func (r *MissingConnectionPoolRule) Category() string { return "database" }

func (r *MissingConnectionPoolRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
	var issues []Issue

	// Find sql.Open calls
	sqlOpenVars := make(map[string]token.Position)

	ast.Inspect(file, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
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

			ident, ok := sel.X.(*ast.Ident)
			if !ok {
				continue
			}

			if ident.Name == "sql" && sel.Sel.Name == "Open" {
				if i < len(assign.Lhs) {
					if lhsIdent, ok := assign.Lhs[i].(*ast.Ident); ok {
						sqlOpenVars[lhsIdent.Name] = fset.Position(assign.Pos())
					}
				}
			}
		}

		return true
	})

	// Check if pool configuration methods are called
	configuredDBs := make(map[string]bool)
	poolMethods := map[string]bool{
		"SetMaxOpenConns":    true,
		"SetMaxIdleConns":    true,
		"SetConnMaxLifetime": true,
		"SetConnMaxIdleTime": true,
	}

	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		if poolMethods[sel.Sel.Name] {
			if ident, ok := sel.X.(*ast.Ident); ok {
				configuredDBs[ident.Name] = true
			}
		}

		return true
	})

	// Report unconfigured database connections
	for dbVar, pos := range sqlOpenVars {
		if !configuredDBs[dbVar] {
			issues = append(issues, Issue{
				Rule:     r.Name(),
				Category: r.Category(),
				Severity: SeverityMedium,
				Line:     pos.Line,
				Column:   pos.Column,
				Message:  "Database connection '" + dbVar + "' opened without pool configuration",
				Why:      "Without configuration, the connection pool uses defaults which may not be suitable. This can lead to connection exhaustion or resource waste.",
				Fix:      "Configure the pool:\n  " + dbVar + ".SetMaxOpenConns(25)\n  " + dbVar + ".SetMaxIdleConns(5)\n  " + dbVar + ".SetConnMaxLifetime(5 * time.Minute)",
			})
		}
	}

	return issues
}

// UnlimitedConnectionPoolRule detects sql.DB with SetMaxOpenConns(0)
type UnlimitedConnectionPoolRule struct{}

func (r *UnlimitedConnectionPoolRule) Name() string     { return "unlimited-connection-pool" }
func (r *UnlimitedConnectionPoolRule) Category() string { return "database" }

func (r *UnlimitedConnectionPoolRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
	var issues []Issue

	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		if sel.Sel.Name == "SetMaxOpenConns" && len(call.Args) > 0 {
			if lit, ok := call.Args[0].(*ast.BasicLit); ok {
				if lit.Kind == token.INT && lit.Value == "0" {
					pos := fset.Position(call.Pos())
					issues = append(issues, Issue{
						Rule:     r.Name(),
						Category: r.Category(),
						Severity: SeverityHigh,
						Line:     pos.Line,
						Column:   pos.Column,
						Message:  "SetMaxOpenConns(0) allows unlimited connections - potential resource exhaustion",
						Why:      "Unlimited connections can exhaust database server resources during traffic spikes. Most databases have connection limits.",
						Fix:      "Set a reasonable limit: SetMaxOpenConns(25) - adjust based on your database's max_connections and number of app instances",
					})
				}
			}
		}

		return true
	})

	return issues
}
