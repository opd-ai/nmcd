package bridge

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/opd-ai/nmcd/client"
)

// MailConfig contains email forwarding configuration extracted from a Namecoin name record.
// This is the primary data structure returned by the bridge adapter when resolving
// .bit addresses to their email forwarding targets.
type MailConfig struct {
	// ForwardTo is the primary email address where mail should be forwarded
	ForwardTo string `json:"forward_to"`

	// BackupAddrs contains fallback email addresses to use if the primary fails
	BackupAddrs []string `json:"backup_addrs,omitempty"`

	// PublicKey contains an optional public key for sender verification (base64-encoded)
	PublicKey []byte `json:"public_key,omitempty"`
}

// Resolver defines the interface for looking up email configuration from names.
// This abstraction allows the mail routing logic to be independent of the
// specific Namecoin client implementation used.
type Resolver interface {
	// LookupMail retrieves email configuration for a given name.
	// Returns ErrNameNotFound if the name doesn't exist or has expired.
	// Returns ErrInvalidMailConfig if the name exists but doesn't contain valid mail configuration.
	LookupMail(name string) (MailConfig, error)
}

// NamecoinBridge implements the Resolver interface by querying a Namecoin client.
// It serves as the adapter layer between Namecoin's name database and the email
// routing system, translating name records into mail configuration.
//
// Thread-safety: NamecoinBridge is safe for concurrent use as long as the
// underlying Namecoin client is thread-safe (which it is for all provided clients).
type NamecoinBridge struct {
	nc client.NameClient
}

// NewNamecoinBridge creates a new bridge adapter with the given Namecoin client.
//
// Parameters:
//   - nc: A NameClient implementation (EmbeddedClient or DaemonClient)
//
// Returns:
//   - *NamecoinBridge: Initialized bridge ready for mail config lookups
//
// Example:
//
//	// Create a Namecoin client
//	nc, err := client.NewClient(&client.Config{
//	    Mode:    client.ModeEmbedded,
//	    Network: "mainnet",
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer nc.Close()
//
//	// Create bridge adapter
//	bridge := NewNamecoinBridge(nc)
//
//	// Lookup mail config for a name
//	config, err := bridge.LookupMail("alice")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Forward to: %s\n", config.ForwardTo)
func NewNamecoinBridge(nc client.NameClient) *NamecoinBridge {
	return &NamecoinBridge{
		nc: nc,
	}
}

// LookupMail retrieves the email configuration for a name by querying the Namecoin database.
//
// The name record's value must be valid JSON containing at minimum an "email" field.
// Optional fields include "backup" (array of backup addresses) and "pubkey" (base64-encoded public key).
//
// Expected JSON format:
//
//	{
//	    "email": "user@gmail.com",
//	    "backup": ["backup@proton.me", "another@example.com"],
//	    "pubkey": "base64encodedkey..."
//	}
//
// Parameters:
//   - name: The Namecoin name to lookup (without namespace prefix like "d/")
//
// Returns:
//   - MailConfig: Parsed email configuration
//   - error: ErrNameNotFound if name doesn't exist, ErrInvalidMailConfig if JSON is invalid
//
// Example:
//
//	config, err := bridge.LookupMail("alice")
//	if err == client.ErrNameNotFound {
//	    fmt.Println("Name not registered")
//	} else if err == ErrInvalidMailConfig {
//	    fmt.Println("Name exists but doesn't contain valid mail config")
//	} else if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Mail forwards to: %s\n", config.ForwardTo)
func (b *NamecoinBridge) LookupMail(name string) (MailConfig, error) {
	// Query Namecoin for the name record
	record, err := b.nc.ResolveName(nil, name)
	if err != nil {
		// ResolveName returns client.ErrNameNotFound for missing/expired names
		// Pass this through directly so callers can distinguish not-found from other errors
		return MailConfig{}, err
	}

	// Parse the mail configuration from the name's value
	config, err := parseMailConfig(record.Value)
	if err != nil {
		return MailConfig{}, fmt.Errorf("%w: %v", ErrInvalidMailConfig, err)
	}

	return config, nil
}

// parseMailConfig parses a Namecoin name value into a MailConfig structure.
// The value must be valid JSON with at minimum an "email" field.
//
// Supported JSON fields:
//   - "email" (string, required): Primary email address for forwarding
//   - "backup" (array of strings, optional): Backup email addresses
//   - "pubkey" (string, optional): Base64-encoded public key for sender verification
//
// Parameters:
//   - value: JSON string from Namecoin name record
//
// Returns:
//   - MailConfig: Parsed configuration
//   - error: JSON parsing or validation error
func parseMailConfig(value string) (MailConfig, error) {
	// Define intermediate structure for JSON unmarshaling
	// This allows us to handle optional fields and validate required ones
	var raw struct {
		Email  string   `json:"email"`
		Backup []string `json:"backup,omitempty"`
		PubKey string   `json:"pubkey,omitempty"`
	}

	// Parse JSON value
	if err := json.Unmarshal([]byte(value), &raw); err != nil {
		return MailConfig{}, fmt.Errorf("invalid JSON: %w", err)
	}

	// Validate required field
	if raw.Email == "" {
		return MailConfig{}, fmt.Errorf("missing required field: email")
	}

	// Decode public key if present
	var pubKey []byte
	if raw.PubKey != "" {
		decoded, err := base64.StdEncoding.DecodeString(raw.PubKey)
		if err != nil {
			return MailConfig{}, fmt.Errorf("invalid base64 pubkey: %w", err)
		}
		pubKey = decoded
	}

	// Build and return config
	config := MailConfig{
		ForwardTo:   raw.Email,
		BackupAddrs: raw.Backup,
		PublicKey:   pubKey,
	}

	return config, nil
}
