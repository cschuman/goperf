# goperf Examples

This directory contains sample Go code demonstrating performance anti-patterns that `goperf` can detect.

## Running goperf on Examples

```bash
# From the project root
goperf ./examples/...

# Or with the Makefile
make example
```

## Example Files

| File | Patterns Demonstrated |
|------|----------------------|
| `allocation.go` | Slice preallocation, map size hints, string concatenation |
| `algorithm.go` | O(n*m) nested loops, quadratic complexity |
| `database.go` | N+1 queries, queries in loops |
| `concurrency.go` | Mutex in loops, unbounded goroutines, goroutine leaks |
| `io.go` | Unbuffered I/O, small buffers, repeated file opens |
| `http.go` | HTTP client in loops, response body leaks |

## Pattern Structure

Each file contains:

1. **Bad Example**: Function demonstrating the anti-pattern
2. **Good Example**: Function showing the corrected version
3. **Comments**: Explaining why it's a problem and how to fix it

## Sample Output

```
$ goperf ./examples/...

examples/allocation.go:11:2: [medium] slice 'results' appended in loop without preallocation (allocation)
  Suggestion: preallocate with make([]string, 0, len(items))

examples/allocation.go:23:2: [low] map created without size hint (allocation)
  Suggestion: use make(map[string]Item, len(items)) for known size

examples/database.go:19:3: [high] potential N+1 query: SQL query inside loop (database)
  Suggestion: batch queries or use JOIN

examples/concurrency.go:15:3: [medium] mutex Lock() called inside loop body (concurrency)
  Suggestion: consider restructuring to reduce lock contention

Found 15 issues (3 high, 5 medium, 7 low)
```

## Adding Examples

When contributing new detection rules, please:

1. Add example code here demonstrating the pattern
2. Include both BAD (anti-pattern) and GOOD (fixed) versions
3. Add clear comments explaining the performance impact
