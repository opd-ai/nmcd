# GitHub Actions CI/CD Pipeline Documentation

This repository uses a multi-file GitHub Actions CI/CD pipeline with separated workflows for testing, building, and releasing.

## Workflow Files

### 1. `test.yml` - Continuous Testing

**Purpose**: Run tests and linting on every pull request and push to main.

**Triggers**:
- Pull requests targeting the `main` branch
- Pushes to the `main` branch

**What it does**:
1. **Test Job**: Runs tests with Go versions 1.21.x and 1.22.x
   - Checks out the code
   - Sets up Go with caching
   - Downloads and verifies dependencies
   - Runs tests with race detection and coverage
   - Uploads coverage reports to Codecov (optional)

2. **Lint Job**: Runs code quality checks
   - Runs `gofmt` to check code formatting
   - Runs `go vet` to check for common mistakes
   - Runs `staticcheck` for advanced static analysis

**How to use**:
- Tests run automatically on every PR and push to main
- Check the "Actions" tab to see test results
- Fix any failing tests or linting issues before merging

---

### 2. `build.yml` - Cross-Platform Builds

**Purpose**: Build binaries for multiple platforms on every push to main.

**Triggers**:
- Pushes to the `main` branch

**What it does**:
1. **Build Job**: Builds `nmcd` binaries for:
   - Linux (amd64, arm64)
   - macOS (amd64 Intel, arm64 Apple Silicon)
   - Windows (amd64)

2. **Build Permamail Job**: Builds `permamail` binaries for the same platforms

**Artifacts**:
- All binaries are uploaded as GitHub Actions artifacts
- Artifacts are retained for 7 days
- Useful for testing builds before creating a release

**How to use**:
- Builds run automatically after merging to main
- Download artifacts from the "Actions" tab → Select workflow run → "Artifacts" section
- Test the binaries before creating a release

---

### 3. `release.yml` - Releases and Nightly Builds

**Purpose**: Create versioned releases and automatic nightly builds.

**Triggers**:
1. **Scheduled (Nightly)**: Daily at 00:00 UTC
2. **Tag Push**: When pushing tags matching `v[0-9]+.[0-9]+.[0-9]+` (e.g., v1.2.3)

**What it does**:

#### For Nightly Builds (scheduled):
1. **Prepare**: Deletes existing `nightly` tag and release
2. **Build**: Creates fresh binaries from latest main branch
3. **Release**: Creates a new `nightly` pre-release with:
   - Binaries for all platforms
   - SHA256 checksums
   - Release notes indicating it's a nightly build
4. **Docker**: Builds and pushes `nightly` Docker image

#### For Versioned Releases (tag push):
1. **Prepare**: Determines version from tag
2. **Build**: Creates binaries with version embedded
3. **Release**: Creates a stable release with:
   - Binaries for all platforms
   - SHA256 checksums
   - Generated release notes
   - Links to changelog and documentation
4. **Docker**: Builds and pushes versioned Docker images with semantic version tags

**How to create a versioned release**:

```bash
# 1. Update version in your code (if needed)
# 2. Update CHANGELOG.md with release notes
# 3. Commit changes
git add .
git commit -m "Prepare v1.2.3 release"
git push origin main

# 4. Create and push a version tag
git tag v1.2.3
git push origin v1.2.3

# 5. GitHub Actions will automatically:
#    - Build binaries for all platforms
#    - Create GitHub release with assets
#    - Build and push Docker images
```

**How nightly builds work**:
- Runs automatically every day at midnight UTC
- No manual intervention needed
- Always reflects the latest `main` branch
- Useful for testing cutting-edge features
- Marked as "pre-release" to distinguish from stable versions

**How to trigger a nightly build manually**:
```bash
# Go to GitHub Actions tab → Release workflow → Run workflow → Select "main" branch
# Or wait for the scheduled run at 00:00 UTC
```

---

## Verification Checklist

After implementation, verify:

- ✅ All three workflow files exist in `.github/workflows/`
- ✅ YAML syntax is valid (no errors, only warnings about line length)
- ✅ `test.yml` triggers on pull requests and pushes to main
- ✅ `build.yml` triggers on pushes to main
- ✅ `release.yml` has both schedule and tag triggers
- ✅ Nightly builds scheduled for daily at 00:00 UTC
- ✅ Version tags use proper regex pattern `v[0-9]+.[0-9]+.[0-9]+`
- ✅ Cross-platform builds cover Linux, macOS, and Windows
- ✅ Both amd64 and arm64 architectures supported
- ✅ Proper permissions declared (`contents: write` for releases)
- ✅ Secrets used securely (`GITHUB_TOKEN`)

---

## Platform Support Matrix

| Platform | Architecture | Binary Name |
|----------|-------------|-------------|
| Linux | amd64 | `nmcd-linux-amd64` |
| Linux | arm64 | `nmcd-linux-arm64` |
| macOS | amd64 (Intel) | `nmcd-darwin-amd64` |
| macOS | arm64 (Apple Silicon) | `nmcd-darwin-arm64` |
| Windows | amd64 | `nmcd-windows-amd64.exe` |

Same matrix applies to `permamail` binaries.

---

## Docker Images

**Registries**:
- GitHub Container Registry: `ghcr.io/opd-ai/nmcd`

**Tags**:
- Nightly: `nightly`
- Versioned: `v1.2.3`, `1.2.3`, `1.2`, `1`, `latest`

**Pull images**:
```bash
# Latest stable release
docker pull ghcr.io/opd-ai/nmcd:latest

# Specific version
docker pull ghcr.io/opd-ai/nmcd:v1.2.3

# Nightly build
docker pull ghcr.io/opd-ai/nmcd:nightly
```

---

## Troubleshooting

**Q: Test workflow fails with "gofmt found formatting issues"**
- Run `make fmt` locally to fix formatting
- Commit and push the changes

**Q: Build workflow fails with "build error"**
- Check the build logs in Actions tab
- Reproduce locally: `GOOS=linux GOARCH=amd64 go build ./cmd/nmcd`

**Q: Nightly build doesn't update**
- Check if the workflow is enabled (Actions tab → Release → Enable workflow)
- Verify the cron schedule is correct
- Check workflow run history for errors

**Q: Release doesn't trigger on tag push**
- Ensure tag matches pattern: `v1.2.3` (not `1.2.3` or `v1.2.3-rc1`)
- Check if tag was pushed: `git push origin v1.2.3`
- Verify workflow file has correct tag pattern

**Q: Docker image doesn't push**
- Check if GitHub Container Registry is enabled for the repository
- Verify `GITHUB_TOKEN` has `packages: write` permission
- Check Docker build logs in workflow run

---

## Best Practices

1. **Always test locally before pushing**:
   ```bash
   make test      # Run tests
   make fmt       # Format code
   make vet       # Run static analysis
   make build     # Build locally
   ```

2. **Use semantic versioning for releases**:
   - MAJOR.MINOR.PATCH (e.g., v1.2.3)
   - Increment MAJOR for breaking changes
   - Increment MINOR for new features
   - Increment PATCH for bug fixes

3. **Update CHANGELOG.md before releases**:
   - Document all changes since last release
   - Group changes by type (Added, Changed, Fixed, etc.)

4. **Test nightly builds before stable releases**:
   - Use nightly builds to test features
   - Get community feedback
   - Fix issues before tagging stable release

5. **Monitor workflow runs**:
   - Check Actions tab regularly
   - Enable email notifications for failures
   - Fix issues promptly

---

## Additional Resources

- [GitHub Actions Documentation](https://docs.github.com/en/actions)
- [Go Release Process](https://go.dev/doc/contribute#release)
- [Semantic Versioning](https://semver.org/)
- [Docker Multi-Platform Builds](https://docs.docker.com/build/building/multi-platform/)
