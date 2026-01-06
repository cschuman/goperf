# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 0.1.x   | :white_check_mark: |

## Reporting a Vulnerability

We take security seriously. If you discover a security vulnerability in goperf, please report it responsibly.

### How to Report

**DO NOT** create a public GitHub issue for security vulnerabilities.

Instead, please email **security@unsaid.io** with:

1. **Description** of the vulnerability
2. **Steps to reproduce** the issue
3. **Potential impact** assessment
4. **Suggested fix** (if you have one)

### What to Expect

- **Acknowledgment**: We will acknowledge receipt within 48 hours
- **Assessment**: We will assess the vulnerability within 7 days
- **Fix timeline**: Critical issues will be addressed within 30 days
- **Disclosure**: We will coordinate disclosure timing with you

### Scope

Security issues we care about:

- **Code execution**: Arbitrary code execution via malicious Go files
- **Path traversal**: Reading/writing files outside the target directory
- **Denial of service**: Crashes or hangs on crafted input
- **Information disclosure**: Leaking sensitive information

### Out of Scope

- Issues in dependencies (report to the dependency maintainer)
- Issues requiring physical access to the machine
- Social engineering attacks

### Recognition

We appreciate responsible disclosure and will:

- Credit you in the release notes (unless you prefer anonymity)
- Add you to our security acknowledgments

## Security Design

goperf is designed with security in mind:

1. **Read-only by default**: Only analyzes code and does not modify files (automatic fixing is not implemented)
2. **Path validation**: Refuses to operate outside the current working directory
3. **Symlink protection**: Validates symlinks don't escape the working directory
4. **Resource limits**: Caps file count, file size, and directory depth
5. **No network access**: Operates entirely locally

## Best Practices for Users

1. **Review suggestions**: Use `--suggest` to review proposed changes
2. **Trust but verify**: Treat suggestions as guidance and apply manually
3. **Keep updated**: Use the latest version for security fixes
