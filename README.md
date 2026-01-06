# goperf

**Preventive performance analysis for Go** - Catch O(n²) loops, N+1 queries, and other performance anti-patterns before they hit production.

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://go.dev)
[![Go Report Card](https://goreportcard.com/badge/github.com/cschuman/goperf)](https://goreportcard.com/report/github.com/cschuman/goperf)
[![GoDoc](https://pkg.go.dev/badge/github.com/cschuman/goperf)](https://pkg.go.dev/github.com/cschuman/goperf)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](CONTRIBUTING.md)

## Why goperf?

Most performance tools are **reactive** - they tell you what's slow after it's in production. `goperf` is **preventive** - it catches performance anti-patterns during development, before they become problems.

| Tool | Focus | Timing |
|------|-------|--------|
| `pprof` | Runtime profiling | After deployment |
| `golangci-lint` | Code correctness | Before commit |
| **`goperf`** | **Performance patterns** | **Before commit** |

## Installation

```bash
go install github.com/cschuman/goperf@latest
```

Or build from source:
```bash
git clone https://github.com/cschuman/goperf.git
cd goperf
go build -o goperf .
```

## Quick Start

```bash
# Audit entire project
goperf ./...

# Check only critical patterns (O(n²), N+1 queries)
goperf --rules=algorithm,database ./...

# CI mode - fail on high severity issues
goperf --fail-on=high --format=json ./...

# Show fix suggestions (does not modify files)
goperf --suggest ./...
```

## What It Detects

### Algorithm (O(n²) and worse)
- **Nested range loops** - Quadratic complexity that explodes with data size
- **Linear search in loops** - Should use maps for O(1) lookup

### Allocation (Memory pressure)
- **Unpreallocated slices** - Repeated allocations from slice growth
- **String concatenation in loops** - Creates O(n²) allocations
- **Maps without size hints** - Causes rehashing as map grows

### Database (N+1 queries)
- **SQL in loops** - Each iteration hits the database separately
- **Unbatched inserts** - Should use bulk operations

### Concurrency (Contention & leaks)
- **Unbuffered channels** - Can cause unexpected blocking
- **Mutex in loops** - Lock contention from repeated acquire/release
- **Goroutine leaks** - Goroutines without termination mechanism

### I/O (Serialization overhead)
- **JSON marshal in loops** - Reflection overhead multiplied
- **http.Client creation** - Should reuse clients for connection pooling
- **ReadAll usage** - Loads entire content into memory

### Cache (Repeated computation)
- **regexp.Compile in functions** - Should compile once at package level
- **template.Parse in functions** - Should parse once at startup

## Example Output

```
╭─────────────────────────────────────────────────────────────╮
│ PERF-AUDIT: 4 issues found (1 critical, 2 high, 1 medium)   │
╰─────────────────────────────────────────────────────────────╯

CRITICAL │ Database Exec() called inside loop - N+1 query pattern
         │ internal/db/repository.go:156:13
         │
         │   153│  for _, item := range items {
         │   154│      // Process each item
         │ → 155│      _, err := db.Exec(query, item.ID, item.Value)
         │   156│      if err != nil {
         │   157│          return err
         │
         │ WHY: Each iteration makes a separate database round-trip. With 100
         │      items, that's 100 queries instead of 1. Network latency
         │      dominates, making this extremely slow.
         │ FIX: Use batch operations: SELECT ... WHERE id IN (...), bulk INSERT,
         │      or collect IDs and query once outside the loop
```

## CI Integration

### GitHub Actions

```yaml
name: Performance Audit
on: [push, pull_request]

jobs:
  goperf:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.21'
      - run: go install github.com/cschuman/goperf@latest
      - run: goperf --fail-on=high ./...
```

### Pre-commit Hook

```bash
#!/bin/sh
goperf --fail-on=critical ./...
```

## Configuration

### Config File

`goperf` will load `.goperf.yml` from the current directory. CLI flags override config values.

Example:

```yaml
rules:
  - algorithm
  - database
ignore_paths:
  - vendor
  - testdata
fail_on: high
format: console
context: 3
verbose: false
```

See `.goperf.yml.example` for a fully documented template.

### Command Line Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--rules` | `all` | Rules to run: `algorithm,allocation,database,concurrency,io,cache,context,memory,benchmark` |
| `--format` | `console` | Output format: `console`, `json`, `diff` |
| `--fail-on` | - | Exit code 1 if issues at this level: `low,medium,high,critical` |
| `--context` | `3` | Lines of code context to show |
| `--ignore` | - | Comma-separated paths to ignore |
| `--verbose` | `false` | Show verbose output |
| `--suggest` | `false` | Show fix suggestions (does not modify files) |

## Severity Levels

| Level | Meaning | Action |
|-------|---------|--------|
| **CRITICAL** | Will cause production issues | Fix immediately |
| **HIGH** | Significant performance impact | Fix before release |
| **MEDIUM** | Moderate impact | Should fix |
| **LOW** | Minor optimization | Nice to have |

## Ignoring Issues

Sometimes you need to suppress a warning - perhaps it's a false positive, or you've verified the code is intentional. Use `// perf:ignore` comments:

### Line-level Ignore

```go
// perf:ignore
for _, item := range items {
    db.Exec(query, item) // This line is ignored
}
```

Or on the same line:
```go
db.Exec(query, item) // perf:ignore
```

### Ignore Specific Rule

```go
// perf:ignore sql-in-loop
for _, item := range items {
    db.Exec(query, item) // Only sql-in-loop is ignored
    result = append(result, item) // Still flagged for append-in-loop
}
```

### Block Ignore

```go
// perf:ignore-start
for _, item := range items {
    db.Exec(query, item)
}
for _, other := range others {
    db.Query(q, other)
}
// perf:ignore-end
```

## Dogfooding: goperf on Itself

We run `goperf` on its own codebase. Here's what happened:

```
$ goperf ./...
╭───────────────────────────────────────────────────────────────────╮
│ PERF-AUDIT: 147 issues found (36 medium, 111 low)                 │
╰───────────────────────────────────────────────────────────────────╯
```

**We manually addressed 34 actionable issues based on suggestions:**

| Issue Type | Count | Fix Applied |
|------------|-------|-------------|
| Unpreallocated slices | 31 | `make([]T, 0, N)` with capacity hints |
| String concat in loops | 2 | `strings.Builder` |
| Map without size hint | 1 | `make(map[K]V, size)` |

**After applying those changes:**
```
$ goperf ./...
╭───────────────────────────────────────────────────────────────────╮
│ PERF-AUDIT: 113 issues found (3 medium, 110 low)                  │
╰───────────────────────────────────────────────────────────────────╯
```

The remaining 113 issues are:
- **3 medium**: Nested loops for AST traversal (intentional, not O(n²) on data)
- **110 low**: "Consider adding benchmarks" suggestions

This demonstrates that `goperf` finds real issues - including in itself - and that acting on suggestions is straightforward.

## Contributing

Contributions welcome! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

Areas we'd love help with:

- [ ] More detection rules
- [ ] False positive reduction
- [ ] IDE integrations (VS Code, GoLand)
- [ ] Benchmark integration
- [ ] Fix suggestions

Please read our [Code of Conduct](CODE_OF_CONDUCT.md) before contributing.

## Security

Found a security issue? Please report it responsibly. See [SECURITY.md](SECURITY.md) for details.

## License

MIT License - see [LICENSE](LICENSE) for details.

## Acknowledgments

Inspired by the philosophy that **preventing performance problems is better than fixing them**.

Built with Go's excellent `go/ast` package for static analysis.

Note: automatic code fixing is not yet implemented. `goperf` only produces suggestions for manual changes.
