# Contributing to nmcd

Thank you for your interest in contributing to nmcd! This document provides guidelines and workflows for contributing to the project.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Workflow](#development-workflow)
- [Code Standards](#code-standards)
- [Testing Requirements](#testing-requirements)
- [Submitting Changes](#submitting-changes)
- [Review Process](#review-process)
- [Release Process](#release-process)
- [Getting Help](#getting-help)

## Code of Conduct

This project adheres to a Code of Conduct that all contributors are expected to follow. Please read [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) before contributing.

## Getting Started

### Prerequisites

- **Go**: Version 1.24.11 or later
- **Git**: For version control
- **Make**: For build automation (optional but recommended)
- **Operating System**: Linux, macOS, or Windows

### Initial Setup

1. **Fork the repository** on GitHub
2. **Clone your fork** locally:
   ```bash
   git clone https://github.com/YOUR_USERNAME/nmcd.git
   cd nmcd
   ```
3. **Add upstream remote**:
   ```bash
   git remote add upstream https://github.com/opd-ai/nmcd.git
   ```
4. **Install dependencies**:
   ```bash
   go mod download
   ```
5. **Build the project**:
   ```bash
   make build
   # Or: go build -v ./cmd/nmcd
   ```
6. **Run tests** to verify setup:
   ```bash
   make test
   # Or: go test ./...
   ```

## Development Workflow

### 1. Sync with Upstream

Before starting work, ensure your fork is up-to-date:

```bash
git checkout main
git fetch upstream
git merge upstream/main
git push origin main
```

### 2. Create a Feature Branch

Use descriptive branch names:

```bash
# Feature branches
git checkout -b feature/add-name-expiry-notification

# Bug fix branches
git checkout -b fix/rpc-rate-limit-overflow

# Documentation branches
git checkout -b docs/update-api-examples
```

### 3. Make Your Changes

- Write clear, focused commits
- Follow the [Code Standards](#code-standards)
- Add or update tests for your changes
- Update documentation as needed

### 4. Commit Your Changes

Write meaningful commit messages following these guidelines:

```
Short (50 chars or less) summary

More detailed explanatory text, if necessary. Wrap at 72 characters.
Explain the problem this commit solves and why you chose this solution.

- Bullet points are okay
- Use present tense: "Add feature" not "Added feature"
- Reference issues: "Fixes #123" or "Relates to #456"
```

**Examples:**

```bash
# Good commit messages
git commit -m "Add LRU cache for name lookups

Implements a 10,000-entry LRU cache to reduce database reads for
frequently accessed names. Benchmarks show 6.4x performance improvement
for GetName operations.

Fixes #234"

# Concise commits for simple changes
git commit -m "Fix typo in API documentation"
```

### 5. Push to Your Fork

```bash
git push origin feature/add-name-expiry-notification
```

### 6. Open a Pull Request

- Navigate to your fork on GitHub
- Click "Compare & pull request"
- Fill out the PR template completely
- Link related issues

## Code Standards

### Go Style Guide

We follow standard Go conventions with some project-specific rules:

#### General Principles

1. **Use Standard Library First**: Prefer `net/http`, `encoding/json`, etc. over external dependencies
2. **Composition over Reimplementation**: Leverage btcd libraries instead of reimplementing blockchain logic
3. **Interface-Based Network Types**: Always use `net.Conn`, `net.Listener`, `net.Addr` (never concrete types like `*net.TCPConn`)
4. **Thread Safety**: Protect shared state with appropriate mutexes (`sync.RWMutex` for read-heavy, `sync.Mutex` for write-heavy)

#### Code Quality

1. **Functions**: Keep functions under 30 lines when possible; single responsibility principle
2. **Error Handling**: Handle all errors explicitly; wrap errors with context using `fmt.Errorf("%w", err)`
3. **Naming**: Use descriptive names over abbreviations (e.g., `blockHeight` not `bh`)
4. **Comments**: Add GoDoc comments for all exported types, functions, and methods
5. **Complexity**: Avoid more than 3 levels of abstraction; prefer clarity over cleverness

#### Formatting

All code must pass these checks:

```bash
# Format code (required before commit)
make fmt
# Or: gofmt -s -w .

# Check for common issues
make vet
# Or: go vet ./...

# Run linters (if applicable)
golangci-lint run
```

### Documentation Standards

1. **Public APIs**: All exported functions/types require GoDoc comments
2. **Complex Algorithms**: Add explanatory comments for non-obvious logic
3. **Security-Sensitive Code**: Document threat model and security assumptions
4. **Examples**: Update `examples/` directory when adding new API features
5. **README Updates**: Reflect feature additions in README.md

### Network Interface Guidelines

**Critical Rule**: Always use interface types for network variables:

```go
// ✅ CORRECT - Use interface types
func handleConnection(conn net.Conn) error { }
func startServer(listener net.Listener) error { }
func getRemoteAddr() net.Addr { }

// ❌ WRONG - Never use concrete types
func handleConnection(conn *net.TCPConn) error { }
func startServer(listener *net.TCPListener) error { }
func getRemoteAddr() *net.TCPAddr { }
```

This enhances testability and allows for different network implementations.

## Testing Requirements

All contributions must include appropriate tests.

### Test Coverage Requirements

- **New Features**: Minimum 80% test coverage for new code
- **Bug Fixes**: Add regression test demonstrating the bug and verifying the fix
- **Critical Packages**: Maintain >= 80% coverage for `namedb`, `chain`, `wallet`, `client`

### Running Tests

```bash
# Run all tests
make test
# Or: go test ./...

# Run with race detector (required before submitting PR)
go test -race ./...

# Run tests with coverage
go test -cover ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Test Organization

1. **Unit Tests**: Test individual functions/methods in isolation
2. **Integration Tests**: Test component interactions (see `integration_test.go`)
3. **Benchmarks**: Add benchmarks for performance-critical code paths
4. **Fuzz Tests**: Add fuzzing for parsers and input validation (see `docs/FUZZING.md`)

### Writing Tests

Follow these patterns:

```go
// Table-driven tests for multiple scenarios
func TestNameValidation(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    bool
        wantErr error
    }{
        {"valid name", "d/example", true, nil},
        {"empty name", "", false, ErrEmptyName},
        {"too long", strings.Repeat("a", 256), false, ErrNameTooLong},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := ValidateName(tt.input)
            if err != tt.wantErr {
                t.Errorf("got error %v, want %v", err, tt.wantErr)
            }
            if got != tt.want {
                t.Errorf("got %v, want %v", got, tt.want)
            }
        })
    }
}

// Use t.TempDir() for temporary files
func TestDatabaseOperations(t *testing.T) {
    dir := t.TempDir()
    db, err := OpenDatabase(dir)
    if err != nil {
        t.Fatalf("failed to open database: %v", err)
    }
    defer db.Close()
    
    // Test operations...
}
```

### Flakiness Prevention

Tests must be deterministic and pass consistently:

```bash
# Run tests multiple times to check for flakiness
go test -race -count=10 ./...
```

## Submitting Changes

### Before Submitting

Complete this checklist before opening a PR:

- [ ] Code follows the [Code Standards](#code-standards)
- [ ] All tests pass: `go test -race ./...`
- [ ] Code is formatted: `make fmt && make vet`
- [ ] Test coverage >= 80% for new code
- [ ] Documentation updated (GoDoc, README, examples)
- [ ] Commit messages are clear and descriptive
- [ ] Branch is rebased on latest `main`

### Pull Request Process

1. **Fill Out PR Template**: Provide all requested information
2. **Link Issues**: Use "Fixes #123" or "Relates to #456" in PR description
3. **Keep PRs Focused**: One feature/fix per PR (easier to review)
4. **Respond to Feedback**: Address review comments promptly
5. **Keep CI Green**: Ensure all automated checks pass

### PR Size Guidelines

- **Small PRs** (< 200 lines): Preferred; faster to review
- **Medium PRs** (200-500 lines): Acceptable for features
- **Large PRs** (> 500 lines): Should be broken into smaller PRs if possible

### Breaking Changes

If your PR introduces breaking changes:

1. Document in commit message and PR description
2. Update CHANGELOG.md under "Unreleased" section
3. Follow deprecation policy (deprecate for one MINOR version before removal)
4. Provide migration guide for users

## Review Process

### What to Expect

1. **Initial Review**: Within 48 hours (usually faster)
2. **Feedback**: Reviewers will check code quality, tests, and documentation
3. **Iteration**: You may be asked to make changes
4. **Approval**: Requires approval from at least one maintainer
5. **Merge**: Maintainers will merge once approved and CI passes

### Review Criteria

Reviewers will evaluate:

- **Correctness**: Does the code do what it claims?
- **Thread Safety**: Are race conditions prevented?
- **Error Handling**: Are all error paths tested?
- **Documentation**: Is the code self-documenting with clear comments?
- **Tests**: Do tests cover normal, edge, and error cases?
- **Performance**: Are there obvious performance issues?
- **Security**: Does the code introduce vulnerabilities?

### Addressing Review Feedback

- Be receptive to suggestions (reviewers want to help)
- Ask questions if feedback is unclear
- Make requested changes in new commits (don't force-push during review)
- Re-request review when ready

## Release Process

nmcd follows semantic versioning (MAJOR.MINOR.PATCH):

- **MAJOR**: Breaking API changes
- **MINOR**: New features, backward-compatible
- **PATCH**: Bug fixes, backward-compatible

### Release Cadence

- **Minor Releases**: Quarterly (every 3 months)
- **Patch Releases**: As needed for critical bug fixes
- **Major Releases**: When breaking changes are necessary

### Release Workflow

1. **Freeze `main`**: Stop merging features 1 week before release
2. **Testing**: Run full test suite including 72-hour stability test
3. **Documentation**: Update CHANGELOG.md and version numbers
4. **Tagging**: Create annotated tag: `git tag -a v1.0.0 -m "Release v1.0.0"`
5. **Automation**: GitHub Actions builds binaries and Docker images
6. **Announcement**: Post release notes on GitHub and community channels

Contributors don't need to manage releases; maintainers handle this.

## Getting Help

### Communication Channels

- **GitHub Issues**: Bug reports and feature requests
- **GitHub Discussions**: Questions, ideas, and general discussion
- **Pull Request Comments**: Code-specific questions during review

### Issue Templates

When creating issues, use the provided templates:

- **Bug Report**: For reproducible bugs
- **Feature Request**: For new functionality
- **Question**: For usage questions

### Asking Good Questions

When asking for help:

1. **Search First**: Check existing issues and documentation
2. **Provide Context**: What are you trying to accomplish?
3. **Include Details**: Error messages, code snippets, logs
4. **Minimal Reproducible Example**: Simplest code that demonstrates the issue
5. **Environment**: Go version, OS, nmcd version

## Additional Resources

- **Documentation**:
  - [README.md](README.md) - Project overview
  - [docs/API.md](docs/API.md) - Complete API reference
  - [docs/EMBEDDING.md](docs/EMBEDDING.md) - Embedding nmcd in applications
  - [docs/EXAMPLES.md](docs/EXAMPLES.md) - Usage examples
  - [docs/OPERATIONS.md](docs/OPERATIONS.md) - Operations guide

- **Testing**:
  - [docs/COVERAGE.md](docs/COVERAGE.md) - Test coverage details
  - [docs/FUZZING.md](docs/FUZZING.md) - Fuzz testing guide
  - [docs/INTEGRATION_TESTS.md](docs/INTEGRATION_TESTS.md) - Integration test guide

- **Project Planning**:
  - [PLAN.md](PLAN.md) - Production readiness roadmap
  - [CHANGELOG.md](CHANGELOG.md) - Version history and changes

---

**Thank you for contributing to nmcd!** Your contributions help make Namecoin more accessible to the Go ecosystem.
