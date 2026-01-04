package mail

import (
	"fmt"
	"strings"
)

// parseBitAddress validates and parses a .bit email address into its localpart component.
// The .bit address format is: localpart@domain.bit
//
// The localpart is used for Namecoin name lookup. For example, "alice@mail.bit" resolves
// to the Namecoin name "alice" (without namespace prefix).
//
// Validation rules:
//   - Address must contain exactly one @ symbol
//   - Domain must end with .bit suffix
//   - Localpart must not be empty
//   - Domain (excluding .bit) must not be empty
//
// Parameters:
//   - addr: Email address in localpart@domain.bit format
//
// Returns:
//   - string: The localpart for Namecoin lookup
//   - error: Validation error if address is malformed
//
// Example:
//
//	name, err := parseBitAddress("alice@mail.bit")
//	// Returns: "alice", nil
//
//	name, err := parseBitAddress("bob@example.com")
//	// Returns: "", error (not a .bit address)
func parseBitAddress(addr string) (string, error) {
	// Validate address has @ symbol
	parts := strings.Split(addr, "@")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid email address: must contain exactly one @ symbol")
	}

	localpart := parts[0]
	domain := parts[1]

	// Validate localpart
	if localpart == "" {
		return "", fmt.Errorf("invalid email address: localpart cannot be empty")
	}

	// Validate domain ends with .bit
	if !strings.HasSuffix(domain, ".bit") {
		return "", fmt.Errorf("invalid .bit address: domain must end with .bit")
	}

	// Validate domain has content before .bit
	domainWithoutSuffix := strings.TrimSuffix(domain, ".bit")
	if domainWithoutSuffix == "" {
		return "", fmt.Errorf("invalid .bit address: domain cannot be just .bit")
	}

	return localpart, nil
}
