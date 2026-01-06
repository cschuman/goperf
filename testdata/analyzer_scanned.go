package main

func scanned(items []int) []int {
	var out []int
	for _, item := range items {
		out = append(out, item)
	}
	return out
}
