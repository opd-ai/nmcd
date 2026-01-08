# Contributing to nmcd

Thank you for your interest in contributing to nmcd! This document provides guidelines and instructions for contributing to the project.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [Contribution Process](#contribution-process)
- [Code Standards](#code-standards)
- [Testing Requirements](#testing-requirements)
- [Pull Request Guidelines](#pull-request-guidelines)
- [Code Review Process](#code-review-process)
- [Release Process](#release-process)
- [Getting Help](#getting-help)

## Code of Conduct

This project adheres to the Contributor Covenant Code of Conduct. By participating, you are expected to uphold this code. Please read [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) for details.

## Getting Started

### Prerequisites

- **Go**: Version 1.24.11 or later
- **Git**: For version control
- **Make**: For build automation (optional but recommended)
- **Operating System**: Linux, macOS, or Windows

### Fork and Clone

1. **Fork the repository** on GitHub by clicking the "Fork" button
2. **Clone your fork** locally:
   ```bash
   git clone https://github.com/YOUR_USERNAME/nmcd.git
   cd nmcd
   ```
3. **Add upstream remote** to track the original repository:
   ```bash
   git remote add upstream https://github.com/opd-ai/nmcd.git
   ```

## Development Setup

### 1. Install Dependencies

All dependencies are managed via Go modules. Simply run:

```bash
go mod download
```

### 2. Build the Project

```bash
# Build all binaries
make build

# Or build manually
go build -v ./cmd/nmcd
go build -v ./cmd/permamail
```

### 3. Run Tests

```bash
# Run all tests
make test

# Or with verbose output
go test -v ./...

# Run with race detector (recommended before submitting PR)
go test -race ./...

# Run specific package tests
go test -v ./namedb
```

### 4. Code Formatting

Always format your code before committing:

```bash
# Format all files
make fmt

# Or manually
gofmt -s -w .
```

### 5. Static Analysis

Run `go vet` to catch common mistakes:

```bash
make vet

# Or manually
go vet ./...
```

## Contribution Process

### 1. Create a Feature Branch

Always create a new branch for your work:

```bash
# Update main branch
git checkout main
git pull upstream main

# Create and switch to feature branch
git checkout -b feature/my-awesome-feature

# Or for bug fixes
git checkout -b fix/issue-123
```

**Branch Naming Convention:**
- `feature/descriptive-name` - New features
- `fix/issue-number` or `fix/descriptive-name` - Bug fixes
- `docs/what-changed` - Documentation updates
- `refactor/component-name` - Code refactoring
- `test/what-tested` - Test additions/improvements

### 2. Make Your Changes

- Write clean, readable code following the [Code Standards](#code-standards)
- Add or update tests for your changes
- Update documentation if you're changing behavior
- Keep commits focused and atomic

### 3. Commit Your Changes

Write clear, descriptive commit messages:

```bash
git add .
git commit -m "Add feature: brief description

Longer explanation of what changed and why.
Reference any relevant issues: Fixes #123"
```

**Commit Message Guidelines:**
- Use present tense ("Add feature" not "Added feature")
- First line should be 50 characters or less
- Add detailed explanation after blank line if needed
- Reference issues and PRs when relevant

### 4. Push to Your Fork

```bash
git push origin feature/my-awesome-feature
```

### 5. Submit a Pull Request

1. Go to your fork on GitHub
2. Click "New Pull Request"
3. Select your feature branch
4. Fill out the PR template completely
5. Submit the pull request

## Code Standards

### General Principles

1. **Use Standard Library First**: Prefer Go's standard library over external dependencies
2. **Keep Functions Small**: Functions should be under 30 lines with single responsibility
3. **Handle All Errors**: Never ignore error returns
4. **Write Self-Documenting Code**: Use descriptive names over abbreviations
5. **Follow Go Best Practices**: Read [Effective Go](https://golang.org/doc/effective_go.html)

### Code Style

- **Format**: Use `gofmt -s` (simplify mode) for all code
- **Imports**: Group into standard library, external, and internal packages
- **Line Length**: Keep lines under 100 characters when reasonable
- **Comments**: Write clear comments for exported functions (godoc style)

### Naming Conventions

```go
// Good: Descriptive names
func ProcessNameUpdate(name string, value string) error { }
var nameExpirationHeight int32

// Bad: Abbreviations
func ProcNmUpd(nm string, val string) error { }
var nmExpHt int32
```

### Error Handling

Always check and handle errors explicitly:

```go
// Good: Explicit error handling
result, err := namedb.GetName("d/example")
if err != nil {
    return fmt.Errorf("failed to get name: %w", err)
}

// Bad: Ignored errors
result, _ := namedb.GetName("d/example")
```

### Documentation

All exported functions, types, and constants must have godoc comments:

```go
// RegisterName creates a new name registration with the given value.
// It implements Namecoin's two-phase registration process:
// NAME_NEW (commitment) → wait 12 blocks → NAME_FIRSTUPDATE (reveal).
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - name: Name to register (1-255 characters)
//   - value: Initial value (max 1023 bytes, typically JSON)
//   - opts: Optional registration options
//
// Returns the transaction result or an error if registration fails.
// Common errors: ErrNameExists, ErrInvalidName, ErrInsufficientFunds.
func (c *EmbeddedClient) RegisterName(ctx context.Context, name, value string, opts *RegisterOpts) (*TxResult, error) {
    // Implementation...
}
```

### Network Types

**CRITICAL**: Always use interface types for network variables (enhances testability):

```go
// Good: Interface types
func handleConnection(conn net.Conn) error { }
var listener net.Listener
var addr net.Addr

// Bad: Concrete types
func handleConnection(conn *net.TCPConn) error { }
var listener *net.TCPListener
var addr *net.TCPAddr
```

Never use type switches or type assertions to convert from interface to concrete types. Use the interface methods instead.

### Thread Safety

Protect all shared state with appropriate mutexes:

```go
type NameDatabase struct {
    mu   sync.RWMutex
    db   *bbolt.DB
    // ... other fields
}

func (ndb *NameDatabase) GetName(name string) (*NameRecord, error) {
    ndb.mu.RLock()
    defer ndb.mu.RUnlock()
    // ... implementation
}
```

## Testing Requirements

### Unit Tests

All new code must include unit tests:

- **Coverage**: Business logic must have >80% test coverage
- **Table-Driven Tests**: Use table-driven tests for multiple scenarios
- **Error Cases**: Test both success and failure paths
- **Hermetic Tests**: Tests must not depend on external state

Example table-driven test:

```go
func TestNameValidation(t *testing.T) {
    tests := []struct {
        name      string
        input     string
        wantError bool
    }{
        {"valid domain", "d/example", false},
        {"valid identity", "id/alice", false},
        {"empty name", "", true},
        {"too long", strings.Repeat("a", 256), true},
        {"invalid namespace", "x/test", true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := validateName(tt.input)
            if (err != nil) != tt.wantError {
                t.Errorf("validateName(%q) error = %v, wantError %v", 
                    tt.input, err, tt.wantError)
            }
        })
    }
}
```

### Integration Tests

For changes affecting multiple components, add integration tests to `integration_test.go`.

### Race Detection

All tests must pass with the race detector:

```bash
go test -race ./...
```

### Running Tests Before PR

```bash
# Full test suite with race detection
go test -race -count=3 ./...

# Check test coverage
go test -cover ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## Pull Request Guidelines

### PR Checklist

Before submitting a PR, ensure:

- [ ] Code follows the project's code standards
- [ ] All tests pass: `go test -v ./...`
- [ ] Race detector passes: `go test -race ./...`
- [ ] Code is formatted: `make fmt` or `gofmt -s -w .`
- [ ] No vet warnings: `make vet` or `go vet ./...`
- [ ] New code has unit tests with >80% coverage
- [ ] Documentation updated (godoc comments, README, etc.)
- [ ] Examples updated if API changed
- [ ] Commit messages are clear and descriptive
- [ ] PR description explains what and why

### PR Template

Your PR will be pre-filled with our template. Please fill out all sections:

- **Description**: What does this PR do?
- **Motivation**: Why is this change needed?
- **Type of Change**: Bug fix, feature, docs, refactor, etc.
- **Testing**: How was this tested?
- **Breaking Changes**: Does this break the API?
- **Checklist**: All items checked

### PR Size Guidelines

- **Small PRs are better**: Keep PRs focused and under 500 lines when possible
- **One concern per PR**: Don't mix features, fixes, and refactoring
- **Break large changes**: Split large features into smaller, reviewable PRs

## Code Review Process

### What to Expect

1. **Initial Review**: A maintainer will review within 48 hours (business days)
2. **Feedback**: You may receive requests for changes or questions
3. **Iteration**: Make requested changes and push to your branch
4. **Approval**: Once approved, a maintainer will merge your PR
5. **Recognition**: Your contribution will be acknowledged in release notes

### Review Criteria

Reviewers will check:

- **Correctness**: Does the code do what it claims?
- **Tests**: Are there adequate tests?
- **Performance**: Are there any performance concerns?
- **Security**: Are there any security implications?
- **Style**: Does it follow project conventions?
- **Documentation**: Is the code well-documented?

### Addressing Feedback

- Respond to all comments (even if just "Done" or "Fixed")
- Push new commits to your branch (don't force push during review)
- Ask questions if feedback is unclear
- Be open to suggestions and alternative approaches

## Release Process

### Version Numbering

nmcd follows [Semantic Versioning](https://semver.org/):

- **MAJOR**: Breaking API changes (e.g., 1.0.0 → 2.0.0)
- **MINOR**: New features, backward-compatible (e.g., 1.0.0 → 1.1.0)
- **PATCH**: Bug fixes, backward-compatible (e.g., 1.0.0 → 1.0.1)

### Release Cadence

- **MINOR releases**: Quarterly (every 3 months)
- **PATCH releases**: As needed for critical bug fixes
- **MAJOR releases**: As needed for breaking changes (with prior notice)

### Release Cycle

1. **Feature Freeze**: 2 weeks before release
2. **Release Candidate**: 1 week before release (v1.1.0-rc1)
3. **Community Testing**: RC tested by users
4. **Final Release**: After RC testing period
5. **Announcement**: Release notes published on GitHub

### API Stability Commitment

Starting from v1.0.0:

- **Public API**: Backward-compatible within MAJOR version
- **Deprecation**: Deprecated features marked for one MINOR version before removal
- **Breaking Changes**: Only in MAJOR version updates
- **Internal APIs**: May change in MINOR versions (use at your own risk)

## Getting Help

### Resources

- **Documentation**: See [docs/](docs/) directory for detailed guides
- **Examples**: See [examples/](examples/) for working code samples
- **API Reference**: See [docs/API.md](docs/API.md)

### Communication Channels

- **GitHub Issues**: For bug reports and feature requests
- **GitHub Discussions**: For questions and general discussion
- **Pull Requests**: For code review and contribution discussion

### Asking Questions

When asking for help:

1. **Search first**: Check existing issues and documentation
2. **Be specific**: Provide details about your environment and what you've tried
3. **Minimal example**: Provide a minimal reproducible example if possible
4. **Be patient**: Maintainers respond as time permits (usually within 48 hours)

### Reporting Bugs

Use the bug report template and include:

- nmcd version (`nmcd --version` or client version)
- Go version (`go version`)
- Operating system and architecture
- Steps to reproduce
- Expected behavior
- Actual behavior
- Relevant logs or error messages

### Suggesting Features

Use the feature request template and include:

- **Problem**: What problem does this solve?
- **Proposed Solution**: How should it work?
- **Alternatives**: What alternatives have you considered?
- **Use Case**: Describe your specific use case

## Recognition

Contributors are recognized in:

- **Release Notes**: All contributors listed in each release
- **GitHub Contributors**: Automatic recognition on the repository
- **Code Ownership**: Your authorship is preserved in git history

Thank you for contributing to nmcd! 🎉

---

**Questions?** Open a [GitHub Discussion](https://github.com/opd-ai/nmcd/discussions) or comment on a relevant issue.
