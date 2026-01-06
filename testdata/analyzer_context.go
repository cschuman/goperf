package main

func contextExample(items []int) []int {
	out := []int{}
	for _, item := range items {
		out = append(out, item)
	}
	return out
}
