// Package examples contains sample code demonstrating performance anti-patterns.
package examples

import (
	"context"
	"database/sql"
)

// NPlusOneQuery demonstrates the N+1 query anti-pattern.
// This executes 1 query for users + N queries for their orders.
func NPlusOneQuery(ctx context.Context, db *sql.DB) ([]UserWithOrders, error) {
	// Query 1: Get all users
	rows, err := db.QueryContext(ctx, "SELECT id, name FROM users")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []UserWithOrders
	for rows.Next() {
		var u UserWithOrders
		if err := rows.Scan(&u.ID, &u.Name); err != nil {
			return nil, err
		}

		// BAD: Query N+1, N+2, ... for each user's orders
		orderRows, err := db.QueryContext(ctx,
			"SELECT id, amount FROM orders WHERE user_id = ?", u.ID)
		if err != nil {
			return nil, err
		}

		for orderRows.Next() {
			var o Order
			if err := orderRows.Scan(&o.ID, &o.Amount); err != nil {
				orderRows.Close()
				return nil, err
			}
			u.Orders = append(u.Orders, o)
		}
		orderRows.Close()

		results = append(results, u)
	}
	return results, nil
}

// BatchQuery shows the corrected version using a JOIN or batch query.
func BatchQuery(ctx context.Context, db *sql.DB) ([]UserWithOrders, error) {
	// GOOD: Single query with JOIN
	rows, err := db.QueryContext(ctx, `
		SELECT u.id, u.name, o.id, o.amount
		FROM users u
		LEFT JOIN orders o ON u.id = o.user_id
		ORDER BY u.id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Process joined results...
	var results []UserWithOrders
	// (implementation omitted for brevity)
	_ = rows
	return results, nil
}

// QueryInLoop demonstrates query execution inside a loop.
func QueryInLoop(ctx context.Context, db *sql.DB, userIDs []string) ([]User, error) {
	var users []User
	for _, id := range userIDs { // BAD: N queries
		row := db.QueryRowContext(ctx, "SELECT id, name FROM users WHERE id = ?", id)
		var u User
		if err := row.Scan(&u.ID, &u.Name); err != nil {
			continue
		}
		users = append(users, u)
	}
	return users, nil
}

// UserWithOrders contains a user and their orders.
type UserWithOrders struct {
	ID     string
	Name   string
	Orders []Order
}

// Order represents an order.
type Order struct {
	ID     string
	Amount float64
}
