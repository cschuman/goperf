// Package examples contains sample code demonstrating performance anti-patterns
// that goperf can detect. Run: goperf ./examples/...
package examples

// SliceAppendInLoop demonstrates the slice growth anti-pattern.
// Each append may cause a reallocation and copy.
// goperf suggests: preallocate with make([]string, 0, len(items))
func SliceAppendInLoop(items []int) []string {
	var results []string // BAD: no preallocation
	for _, item := range items {
		results = append(results, processItem(item))
	}
	return results
}

// SliceAppendFixed shows the corrected version.
func SliceAppendFixed(items []int) []string {
	results := make([]string, 0, len(items)) // GOOD: preallocated
	for _, item := range items {
		results = append(results, processItem(item))
	}
	return results
}

// MapWithoutSizeHint demonstrates map growth overhead.
// Maps resize when they exceed their capacity.
func MapWithoutSizeHint(items []Item) map[string]Item {
	result := make(map[string]Item) // BAD: no size hint
	for _, item := range items {
		result[item.ID] = item
	}
	return result
}

// MapWithSizeHint shows the corrected version.
func MapWithSizeHint(items []Item) map[string]Item {
	result := make(map[string]Item, len(items)) // GOOD: size hint
	for _, item := range items {
		result[item.ID] = item
	}
	return result
}

// StringConcatInLoop demonstrates inefficient string building.
// String concatenation creates new strings each iteration.
func StringConcatInLoop(parts []string) string {
	var result string // BAD: string concatenation in loop
	for _, part := range parts {
		result += part
	}
	return result
}

// Item is a sample data type for examples.
type Item struct {
	ID   string
	Name string
}

func processItem(i int) string {
	return ""
}
