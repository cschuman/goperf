# Contributing to goperf

Thank you for your interest in contributing to goperf! This document provides guidelines and information for contributors.

## Code of Conduct

This project adheres to the [Contributor Covenant Code of Conduct](CODE_OF_CONDUCT.md). By participating, you are expected to uphold this code.

## How to Contribute

### Reporting Bugs

Before creating a bug report, please check existing issues to avoid duplicates. When creating a bug report, include:

- **Go version** (`go version`)
- **OS and architecture**
- **goperf version** (`goperf --version`)
- **Minimal reproducible example**
- **Expected vs actual behavior**

### Suggesting Features

Feature requests are welcome! Please include:

- **Use case**: What problem does this solve?
- **Proposed solution**: How should it work?
- **Alternatives considered**: What else did you think about?

### Pull Requests

1. **Fork the repository** and create your branch from `main`
2. **Write tests** for any new functionality
3. **Run the test suite**: `make test`
4. **Run the linter**: `make lint`
5. **Run self-audit**: `make audit` (we dogfood!)
6. **Update documentation** if needed
7. **Write a clear PR description**

## Development Setup

```bash
# Clone your fork
git clone https://github.com/YOUR_USERNAME/goperf.git
cd goperf

# Install dependencies
go mod download

# Build
make build

# Run tests
make test

# Run linter
make lint

# Self-audit (dogfooding)
make audit
```

## Project Structure

```
goperf/
├── main.go           # CLI entry point
├── rules/            # Detection rules
│   ├── analyzer.go   # Core analysis engine
│   ├── types.go      # Issue, Severity types
│   ├── algorithm.go  # O(n²) detection
│   ├── allocation.go # Memory allocation patterns
│   ├── database.go   # N+1 query detection
│   └── ...
├── fixer/            # Fix suggestions
├── reporter/         # Output formatters (console, JSON)
└── examples/         # Example problematic code
```

## Adding a New Rule

1. **Create or edit a file** in `rules/` (e.g., `rules/mypattern.go`)

2. **Implement the Rule interface**:
```go
type MyRule struct{}

func (r *MyRule) Name() string     { return "my-rule-name" }
func (r *MyRule) Category() string { return "category" }

func (r *MyRule) Check(file *ast.File, fset *token.FileSet, src []byte) []Issue {
    issues := make([]Issue, 0, 4)
    // Your detection logic here
    return issues
}
```

3. **Register in init()**:
```go
func init() {
    RegisterRule("category", &MyRule{})
}
```

4. **Add tests** in `rules/mypattern_test.go`

5. **Update documentation** in README.md

## Code Style

- Follow standard Go conventions (`gofmt`, `go vet`)
- Use meaningful variable names
- Add comments for non-obvious logic
- Preallocate slices when size is known: `make([]T, 0, n)`
- Keep functions focused and small

## Testing

```bash
# Run all tests
make test

# Run with coverage
make coverage

# Run specific test
go test -v -run TestMyRule ./rules/
```

## Commit Messages

Follow conventional commits:

- `feat:` New feature
- `fix:` Bug fix
- `docs:` Documentation only
- `test:` Adding tests
- `refactor:` Code change that neither fixes a bug nor adds a feature
- `perf:` Performance improvement
- `chore:` Maintenance tasks

Examples:
```
feat(rules): add detection for sync.Pool misuse
fix(analyzer): handle nil pointer in nested loops
docs: update contributing guide
```

## Release Process

Releases are automated via GitHub Actions when a tag is pushed:

```bash
git tag -a v0.2.0 -m "v0.2.0 - Description"
git push origin v0.2.0
```

## Getting Help

- **Questions**: Open a [Discussion](https://github.com/unsaid-dev/goperf/discussions)
- **Bugs**: Open an [Issue](https://github.com/unsaid-dev/goperf/issues)
- **Security**: See [SECURITY.md](SECURITY.md)

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
