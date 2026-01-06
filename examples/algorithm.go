// Package examples contains sample code demonstrating performance anti-patterns.
package examples

// NestedLoopLookup demonstrates O(n*m) complexity that could be O(n+m).
// This pattern is common when checking membership in a slice.
func NestedLoopLookup(users []User, allowedIDs []string) []User {
	var allowed []User
	for _, user := range users { // O(n)
		for _, id := range allowedIDs { // O(m) - BAD: linear search
			if user.ID == id {
				allowed = append(allowed, user)
				break
			}
		}
	}
	return allowed
}

// NestedLoopLookupFixed shows the O(n+m) version using a map.
func NestedLoopLookupFixed(users []User, allowedIDs []string) []User {
	// Build lookup map: O(m)
	allowedSet := make(map[string]bool, len(allowedIDs))
	for _, id := range allowedIDs {
		allowedSet[id] = true
	}

	// Filter users: O(n)
	allowed := make([]User, 0, len(users))
	for _, user := range users {
		if allowedSet[user.ID] { // O(1) lookup - GOOD
			allowed = append(allowed, user)
		}
	}
	return allowed
}

// QuadraticStringMatch demonstrates O(n*m) substring matching.
func QuadraticStringMatch(texts []string, patterns []string) []string {
	var matches []string
	for _, text := range texts { // O(n)
		for _, pattern := range patterns { // O(m) - potentially O(n²)
			if containsPattern(text, pattern) {
				matches = append(matches, text)
				break
			}
		}
	}
	return matches
}

// User is a sample data type.
type User struct {
	ID   string
	Name string
}

func containsPattern(text, pattern string) bool {
	return false // placeholder
}
