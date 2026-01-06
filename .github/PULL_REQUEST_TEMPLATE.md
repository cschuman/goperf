## Description

Brief description of the changes in this PR.

## Type of Change

- [ ] Bug fix (non-breaking change that fixes an issue)
- [ ] New feature (non-breaking change that adds functionality)
- [ ] New rule (adds a new performance pattern detector)
- [ ] Breaking change (fix or feature that would cause existing functionality to change)
- [ ] Documentation update
- [ ] Refactoring (no functional changes)

## Related Issues

Fixes #(issue number)

## Changes Made

- Change 1
- Change 2
- Change 3

## Testing

- [ ] I have run `make test` and all tests pass
- [ ] I have run `make lint` with no new warnings
- [ ] I have run `make audit` (dogfooding) and reviewed the output
- [ ] I have added tests for new functionality
- [ ] I have tested manually with sample code

## For New Rules

If adding a new detection rule:

- [ ] Rule is registered in `init()`
- [ ] Rule has corresponding tests in `*_test.go`
- [ ] Rule is documented in README.md
- [ ] Example code is provided in `examples/`

## Documentation

- [ ] I have updated the README if needed
- [ ] I have added/updated GoDoc comments for exported functions
- [ ] I have updated CHANGELOG.md

## Screenshots/Output

If applicable, show example output:

```
$ goperf ./examples/
examples/sample.go:10:5: [medium] description (category)
  Suggestion: how to fix
```

## Checklist

- [ ] My code follows the project's code style
- [ ] I have performed a self-review of my code
- [ ] I have commented my code where necessary
- [ ] My changes generate no new warnings
