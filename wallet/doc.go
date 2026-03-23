// Package wallet provides basic wallet functionality for nmcd.
//
// The wallet package implements key management, address generation, and
// transaction signing for Namecoin name operations. It supports both
// unencrypted (legacy) and encrypted wallet storage with password protection.
//
// # Features
//
// The wallet provides:
//
//   - ECDSA key pair generation using secp256k1 curve
//   - Address generation (P2PKH format)
//   - Transaction signing for name operations
//   - JSON file persistence with optional encryption
//   - Password-based encryption with Argon2id key derivation
//
// # Key Management
//
// Keys are stored as KeyPair structures containing:
//
//   - PrivateKey: secp256k1 private key (32 bytes)
//   - PublicKey: Corresponding public key
//   - Address: Derived P2PKH address
//
// Multiple key pairs can be stored in a single wallet, indexed by address.
//
// # Encryption
//
// Version 2 wallets support password encryption:
//
//   - Key derivation: Argon2id with configurable parameters
//   - Encryption: AES-256-GCM for authenticated encryption
//   - Salt: Unique random salt per wallet
//   - Password verification: Hash stored separately from encryption key
//
// The wallet must be unlocked to perform sensitive operations like signing
// or exporting private keys.
//
// # Transaction Creation
//
// The wallet can create unsigned transaction templates for:
//
//   - NAME_NEW: Pre-registration commitment
//   - NAME_FIRSTUPDATE: Initial name registration
//   - NAME_UPDATE: Value updates and expiration extension
//
// Note: Transaction broadcasting requires additional UTXO management not
// currently implemented in the wallet package.
//
// # Thread Safety
//
// Wallet is safe for concurrent use. All operations acquire appropriate
// locks via sync.RWMutex. However, Lock/Unlock operations should be
// coordinated at the application level to prevent unexpected state changes.
//
// # File Format
//
// Wallets are stored as JSON files:
//
// Version 1 (unencrypted):
//
//	{
//	  "version": 1,
//	  "keys": [{"private_key": "...", "address": "..."}]
//	}
//
// Version 2 (encrypted):
//
//	{
//	  "version": 2,
//	  "encrypted": true,
//	  "salt": "...",
//	  "password_hash": "...",
//	  "keys": "encrypted-base64-data"
//	}
//
// # Security Considerations
//
//   - Version 1 wallets store private keys in plaintext
//   - File permissions should be 0600 (owner read/write only)
//   - Password strength directly affects encryption security
//   - Wallet unlock caches password in memory until Lock is called
//   - Consider using hardware wallets for production funds
//
// # Example Usage
//
// Creating a new wallet:
//
//	w, err := wallet.NewWallet("/path/to/wallet.json", chainParams)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Generate a new address
//	addr, err := w.NewAddress()
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("New address: %s\n", addr.EncodeAddress())
//
// Encrypting a wallet:
//
//	// Encrypt with password
//	err := w.Encrypt("secure-password")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Later, unlock to use
//	err = w.Unlock("secure-password")
//	if err != nil {
//	    log.Fatal("Wrong password")
//	}
//	defer w.Lock()
//
// Signing a transaction:
//
//	// Create NAME_UPDATE transaction
//	tx, err := w.CreateNameUpdate("d/example", `{"ip":"1.2.3.4"}`)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Sign the transaction
//	err = w.SignTransaction(tx, addr)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// # Limitations
//
// Current limitations:
//
//   - No HD wallet support (BIP32/BIP44)
//   - No multi-signature support
//   - Limited UTXO management (requires external tracking)
//   - Transaction creation but not broadcasting
package wallet
