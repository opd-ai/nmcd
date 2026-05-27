// Package version provides a single source of truth for the nmcd daemon version.
package version

// Version is the current release version of the nmcd daemon.
// It may be overridden at build time via:
// -ldflags "-X github.com/opd-ai/nmcd/internal/version.Version=x.y.z"
var Version = "0.1.0"
