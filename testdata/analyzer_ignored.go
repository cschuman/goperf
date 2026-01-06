package main

func ignored(items []int) []int {
	var out []int
	for _, item := range items {
		out = append(out, item)
	}
	return out
}
