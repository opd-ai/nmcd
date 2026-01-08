# Contributing to nmcd

Thank you for your interest in contributing to nmcd! This document provides guidelines and instructions for contributing to the project.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [How Can I Contribute?](#how-can-i-contribute)
- [Development Setup](#development-setup)
- [Coding Standards](#coding-standards)
- [Testing Requirements](#testing-requirements)
- [Pull Request Process](#pull-request-process)
- [Release Process](#release-process)
- [Getting Help](#getting-help)

## Code of Conduct

This project adheres to a [Code of Conduct](CODE_OF_CONDUCT.md). By participating, you are expected to uphold this code. Please report unacceptable behavior to the project maintainers.

## How Can I Contribute?

### Reporting Bugs

Before creating bug reports, please check existing issues to avoid duplicates. When creating a bug report, include:

- **Clear title and description** of the issue
- **Steps to reproduce** the behavior
- **Expected vs actual behavior**
- **Environment details**: OS, Go version, nmcd version
- **Relevant logs or error messages**
- **Code samples** if applicable

Use the bug report issue template when available.

### Suggesting Enhancements

Enhancement suggestions are welcome! Please:

- **Use a clear title** describing the enhancement
- **Provide detailed description** of the proposed feature
- **Explain the use case** and why this would be valuable
- **Consider alternatives** and mention them
- **Be open to discussion** about implementation approach

Use the feature request issue template when available.

### Contributing Code

We welcome code contributions! Areas where contributions are especially valuable:

- **Bug fixes** for open issues
- **Performance improvements** with benchmarks showing improvement
- **Documentation improvements** (typo fixes, clarifications, examples)
- **Test coverage** for untested code paths
- **Security enhancements** (always responsibly disclosed)

## Development Setup

### Prerequisites

- **Go 1.24.11 or later**: Required for building nmcd
- **Git**: For version control
- **Make**: Optional but recommended for build automation

### Building from Source

```bash
# Clone the repository
git clone https://github.com/opd-ai/nmcd.git
cd nmcd

# Install dependencies
go mod download

# Build the daemon
make build

# Run tests
make test

# Format code
make fmt

# Run static analysis
make vet
```

### Project Structure

- `client/` - Public library API (stable interface)
- `cmd/nmcd/` - Daemon binary entry point
- `cmd/permamail/` - Permamail CLI tool
- `namedb/` - Name database implementation
- `chain/` - Blockchain wrapper (btcd integration)
- `network/` - P2P networking and sync
- `rpc/` - JSON-RPC server
- `wallet/` - Wallet functionality
- `config/` - Network configuration
- `docs/` - Documentation
- `examples/` - Working code examples

## Coding Standards

### Go Best Practices

1. **Use Standard Library First**: Prefer stdlib over external dependencies
2. **Keep Functions Small**: < 30 lines with single responsibility
3. **Handle All Errors**: No ignored error returns (`_ = err` is forbidden)
4. **Write Self-Documenting Code**: Descriptive names over abbreviations
5. **Thread Safety**: Protect shared state with appropriate mutexes

### Code Style

- **Follow `gofmt` formatting**: Run `make fmt` before committing
- **Pass `go vet`**: Run `make vet` to catch common issues
- **No concrete network types**: Use `net.Conn` not `*net.TCPConn`, `net.Listener` not `*net.TCPListener`
- **Add GoDoc comments**: All exported functions must have documentation
- **Use meaningful variable names**: `blockHeight` not `bh`, `transaction` not `tx` in public APIs

### Documentation Standards

- **Package-level documentation**: Explain purpose and usage patterns
- **Function documentation**: Include purpose, parameters, return values, special considerations
- **Complex algorithms**: Add explanatory comments for non-obvious logic
- **Security-sensitive code**: Document threat model and assumptions
- **Examples**: Update `examples/` when adding new library patterns

### Error Handling

```go
// ✅ Good: Wrap errors with context
if err := db.Save(record); err != nil {
    return fmt.Errorf("failed to save name record: %w", err)
}

// ❌ Bad: Return raw error without context
if err := db.Save(record); err != nil {
    return err
}

// ❌ Bad: Ignore errors
_ = db.Save(record)
```

### Thread Safety

```go
// ✅ Good: Lock at start of public method, defer unlock
func (ndb *NameDatabase) GetName(name string) (*NameRecord, error) {
    ndb.mu.RLock()
    defer ndb.mu.RUnlock()
    // ... perform operations
}

// ❌ Bad: Accessing shared state without lock
func (ndb *NameDatabase) GetName(name string) (*NameRecord, error) {
    return ndb.names[name], nil // Race condition!
}
```

## Testing Requirements

### Unit Tests

- **Coverage**: Aim for > 80% coverage on business logic
- **Error cases**: Test both success and failure paths
- **Table-driven tests**: Use for multiple scenarios
- **Hermetic tests**: No shared state between tests
- **Use `t.TempDir()`**: For temporary files/directories

Example:

```go
func TestNameValidation(t *testing.T) {
    tests := []struct {
        name      string
        input     string
        wantErr   bool
        errType   error
    }{
        {"valid name", "d/example", false, nil},
        {"empty name", "", true, ErrInvalidName},
        {"too long", strings.Repeat("a", 256), true, ErrInvalidName},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ValidateName(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("ValidateName() error = %v, wantErr %v", err, tt.wantErr)
            }
            if tt.wantErr && !errors.Is(err, tt.errType) {
                t.Errorf("ValidateName() error = %v, want %v", err, tt.errType)
            }
        })
    }
}
```

### Running Tests

```bash
# Run all tests
go test ./...

# Run with race detector (required before PR)
go test -race ./...

# Run with coverage
go test -cover ./...

# Run specific package
go test -v ./client/...

# Run tests multiple times to check for flakiness
go test -race -count=10 ./...
```

### Benchmark Tests

When making performance changes, include benchmarks:

```go
func BenchmarkNameLookup(b *testing.B) {
    db := setupTestDB(b)
    defer db.Close()
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, _ = db.GetName("d/example")
    }
}
```

Run with: `go test -bench=. -benchmem ./...`

## Pull Request Process

### Before Submitting

1. **Create a feature branch**: `git checkout -b feature/my-feature` or `git checkout -b fix/issue-123`
2. **Make your changes**: Follow coding standards above
3. **Add/update tests**: Ensure tests pass and coverage is maintained
4. **Update documentation**: README, API docs, examples as needed
5. **Format code**: Run `make fmt`
6. **Check static analysis**: Run `make vet`
7. **Run tests**: `go test -race -count=3 ./...` (must pass)
8. **Commit with clear messages**: See commit message guidelines below

### Commit Message Guidelines

Follow the [Conventional Commits](https://www.conventionalcommits.org/) format:

```
<type>(<scope>): <subject>

<body>

<footer>
```

**Types:**
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation only
- `style`: Formatting, missing semicolons, etc (no code change)
- `refactor`: Code change that neither fixes a bug nor adds a feature
- `perf`: Performance improvement
- `test`: Adding or correcting tests
- `chore`: Changes to build process, dependencies, etc

**Examples:**

```
feat(client): add support for batch name resolution

Adds BatchResolveName method to NameClient interface for efficient
lookup of multiple names in a single call.

Closes #123
```

```
fix(namedb): prevent race condition in cache eviction

The cache eviction goroutine was accessing the map without proper
locking. Added RWMutex protection to ensure thread safety.

Fixes #456
```

### Pull Request Checklist

Before submitting, ensure:

- [ ] Code follows project style guidelines (`make fmt` and `make vet` pass)
- [ ] All tests pass (`go test -race ./...`)
- [ ] New tests added for new functionality
- [ ] Test coverage is maintained or improved
- [ ] Documentation is updated (README, godoc, examples)
- [ ] Commit messages are clear and descriptive
- [ ] PR description explains the changes and motivation
- [ ] No merge conflicts with main branch
- [ ] Security implications considered (if applicable)

### Review Process

1. **Submit PR**: Create PR against `main` branch using the PR template
2. **CI Checks**: Automated tests and linting must pass
3. **Code Review**: At least one maintainer review required
4. **Address Feedback**: Make requested changes, push updates
5. **Approval**: Once approved, a maintainer will merge

**Review Timeline:**
- Initial review: Within 48 hours for most PRs
- Follow-up reviews: Within 24 hours
- Merge: After approval and passing CI

## Release Process

nmcd follows [Semantic Versioning](https://semver.org/):

- **MAJOR** (v2.0.0): Breaking API changes
- **MINOR** (v1.1.0): New features, backward compatible
- **PATCH** (v1.0.1): Bug fixes, backward compatible

### Release Cadence

- **Minor releases**: Quarterly (every 3 months)
- **Patch releases**: As needed for critical bugs
- **Major releases**: When breaking changes are necessary

### Deprecation Policy

- Features are deprecated in one MINOR version before removal
- Deprecated features include migration guide
- Breaking changes are documented in CHANGELOG.md

See [CHANGELOG.md](CHANGELOG.md) for version history and stability guarantees.

## Getting Help

### Documentation

- **README**: Quick start and overview
- **docs/API.md**: Complete API reference
- **docs/EXAMPLES.md**: Code examples and walkthroughs
- **docs/INSTALLATION.md**: Installation instructions
- **docs/OPERATIONS.md**: Configuration and monitoring

### Community

- **GitHub Issues**: Bug reports and feature requests
- **GitHub Discussions**: General questions and discussions
- **Code Review**: Ask questions in PR comments

### Maintainer Contact

For security issues, please email the maintainers privately rather than creating a public issue.

---

## Attribution

This Contributing Guide is adapted from open source contributing guides and best practices from the Go community.

Thank you for contributing to nmcd! 🚀
