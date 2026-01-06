package main

func basic(items []int) []int {
	var out []int
	for _, item := range items {
		out = append(out, item)
	}
	return out
}

func concat(items []string) string {
	s := ""
	for _, item := range items {
		s += item + "!"
	}
	return s
}
