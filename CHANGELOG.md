# Changelog

All notable changes to goperf will be documented in this file.

## [0.1.0] - 2026-01-05

### Added
- Initial release with 9 rule categories:
  - **algorithm**: Nested range loops, linear search in loops
  - **allocation**: Unpreallocated slices, string concatenation, map size hints
  - **database**: SQL in loops (N+1), unbatched inserts, connection pool issues
  - **concurrency**: Unbuffered channels, mutex in loops, goroutine leaks
  - **io**: JSON in loops, HTTP client creation, ReadAll usage, body close
  - **cache**: Repeated regexp/template compilation
  - **context**: Background in handlers, missing timeouts, context leaks
  - **memory**: pprof in hot paths, large struct copies
  - **benchmark**: Functions needing benchmarks

- Output formats: `console`, `json`, `diff`
- CI integration with `--fail-on` threshold
- Fix suggestions with `--suggest` and `--dry-run`
- Ignore comments: `// perf:ignore`, `// perf:ignore-start/end`
- Smart detection to reduce false positives:
  - Recognizes prepared statements in database loops
  - Detects signal channels (`done`, `quit`, `ctx`)
  - Identifies bounded loops with small iteration counts

### Dogfooding Results
Ran goperf on itself:
- **Before**: 147 issues (36 medium, 111 low)
- **Addressed**: 34 issues (slice preallocation, strings.Builder, map hints)
- **After**: 113 issues (3 medium, 110 low)

The 3 remaining medium issues are intentional AST traversal patterns, and the 110 low issues are benchmark suggestions.
