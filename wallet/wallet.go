// Package wallet provides basic wallet functionality for nmcd.
// It supports key generation, storage, and NAME_UPDATE transaction creation.
package wallet

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/opd-ai/nmcd/config"
)

// Wallet manages keys and creates transactions for name operations.
type Wallet struct {
	dataDir     string
	chainParams *chaincfg.Params
	keys        map[string]*KeyPair // address -> key pair
	mu          sync.RWMutex

	// Encryption state
	encrypted      bool   // Whether the wallet is encrypted
	locked         bool   // Whether the wallet is currently locked
	passwordHash   []byte // Hash of the password for verification (not the password itself)
	passwordSalt   []byte // Random salt for password hashing (unique per wallet)
	unlockPassword string // Cached password when unlocked (cleared on lock)
}

// KeyPair represents a private/public key pair.
type KeyPair struct {
	PrivateKey *btcec.PrivateKey
	PublicKey  *btcec.PublicKey
	Address    btcutil.Address
}

// walletData is the serializable format for wallet storage.
// Version 1: Unencrypted keys (legacy format)
// Version 2: Encrypted keys with password protection
type walletData struct {
	Version      int       `json:"version"`                 // Wallet format version (1 or 2)
	Encrypted    bool      `json:"encrypted"`               // Whether keys are encrypted
	PasswordHash string    `json:"password_hash,omitempty"` // Hash of password (version 2 only)
	PasswordSalt string    `json:"password_salt,omitempty"` // Salt for password hashing (version 2 only)
	Keys         []keyData `json:"keys"`                    // Key pairs (encrypted or unencrypted)
}

// keyData represents a serializable key pair.
// For version 1 (unencrypted): PrivateKeyHex is hex-encoded private key
// For version 2 (encrypted): PrivateKeyHex is encrypted private key in format "salt:nonce:ciphertext"
type keyData struct {
	PrivateKeyHex string `json:"private_key"`
	Address       string `json:"address"`
}

// NewWallet creates a new wallet instance.
func NewWallet(dataDir string, chainParams *chaincfg.Params) (*Wallet, error) {
	w := &Wallet{
		dataDir:     dataDir,
		chainParams: chainParams,
		keys:        make(map[string]*KeyPair),
		encrypted:   false,
		locked:      false,
	}

	// Try to load existing wallet
	if err := w.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load wallet: %w", err)
	}

	return w, nil
}

// walletPath returns the path to the wallet file.
func (w *Wallet) walletPath() string {
	return filepath.Join(w.dataDir, "wallet.json")
}

// load reads the wallet from disk.
// Supports both version 1 (unencrypted) and version 2 (encrypted) wallet formats.
func (w *Wallet) load() error {
	data, err := os.ReadFile(w.walletPath())
	if err != nil {
		return err
	}

	var wd walletData
	if err := json.Unmarshal(data, &wd); err != nil {
		return fmt.Errorf("failed to parse wallet: %w", err)
	}

	normalizeWalletVersion(&wd)

	if err := w.applyEncryptionState(&wd); err != nil {
		return err
	}

	if !w.encrypted {
		return w.loadKeys(&wd)
	}
	return nil
}

// normalizeWalletVersion handles legacy wallet format without version field.
func normalizeWalletVersion(wd *walletData) {
	if wd.Version == 0 {
		wd.Version = 1
		wd.Encrypted = false
	}
}

// applyEncryptionState sets the wallet's encryption fields from wallet data.
func (w *Wallet) applyEncryptionState(wd *walletData) error {
	w.encrypted = wd.Encrypted
	if !w.encrypted {
		w.locked = false
		return nil
	}

	w.locked = true
	var err error
	w.passwordHash, err = hex.DecodeString(wd.PasswordHash)
	if err != nil {
		return fmt.Errorf("failed to decode password hash: %w", err)
	}
	w.passwordSalt, err = hex.DecodeString(wd.PasswordSalt)
	if err != nil {
		return fmt.Errorf("failed to decode password salt: %w", err)
	}
	return nil
}

// loadKeys loads key pairs from wallet data.
// For encrypted wallets, this must be called after unlock with the password.
func (w *Wallet) loadKeys(wd *walletData) error {
	for _, kd := range wd.Keys {
		privKeyBytes, err := w.decodePrivateKey(kd.PrivateKeyHex)
		if err != nil {
			return err
		}

		privKey, pubKey := btcec.PrivKeyFromBytes(privKeyBytes)
		pubKeyHash := btcutil.Hash160(pubKey.SerializeCompressed())
		addr, err := btcutil.NewAddressPubKeyHash(pubKeyHash, w.chainParams)
		if err != nil {
			return fmt.Errorf("failed to create address: %w", err)
		}

		w.keys[addr.EncodeAddress()] = &KeyPair{
			PrivateKey: privKey,
			PublicKey:  pubKey,
			Address:    addr,
		}
	}
	return nil
}

// decodePrivateKey decodes a private key from its stored format based on encryption state.
func (w *Wallet) decodePrivateKey(keyHex string) ([]byte, error) {
	if w.encrypted && w.unlockPassword != "" {
		encData, err := decodeEncryptedData(keyHex)
		if err != nil {
			return nil, fmt.Errorf("failed to decode encrypted private key: %w", err)
		}
		privKeyBytes, err := decrypt(encData, w.unlockPassword)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt private key: %w", err)
		}
		return privKeyBytes, nil
	}
	if !w.encrypted {
		privKeyBytes, err := hex.DecodeString(keyHex)
		if err != nil {
			return nil, fmt.Errorf("failed to decode private key: %w", err)
		}
		return privKeyBytes, nil
	}
	return nil, fmt.Errorf("cannot load keys from encrypted wallet without password")
}

// save writes the wallet to disk.
// For encrypted wallets, private keys are encrypted before saving.
// For unencrypted wallets, private keys are stored as hex-encoded bytes.
func (w *Wallet) save() error {
	wd := walletData{
		Version:   2,
		Encrypted: w.encrypted,
		Keys:      make([]keyData, 0, len(w.keys)),
	}

	if w.encrypted {
		wd.PasswordHash = hex.EncodeToString(w.passwordHash)
		wd.PasswordSalt = hex.EncodeToString(w.passwordSalt)
	}

	for addr, kp := range w.keys {
		privKeyHex, err := w.encodePrivateKey(kp)
		if err != nil {
			return err
		}
		wd.Keys = append(wd.Keys, keyData{
			PrivateKeyHex: privKeyHex,
			Address:       addr,
		})
	}

	data, err := json.MarshalIndent(wd, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize wallet: %w", err)
	}

	if err := os.MkdirAll(w.dataDir, 0o700); err != nil {
		return fmt.Errorf("failed to create wallet directory: %w", err)
	}

	path := w.walletPath()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("failed to write wallet file %s: %w", path, err)
	}
	return nil
}

// encodePrivateKey encodes a private key for storage based on encryption state.
func (w *Wallet) encodePrivateKey(kp *KeyPair) (string, error) {
	if !w.encrypted {
		return hex.EncodeToString(kp.PrivateKey.Serialize()), nil
	}
	if w.unlockPassword == "" {
		return "", fmt.Errorf("cannot save encrypted wallet while locked")
	}
	encData, err := encrypt(kp.PrivateKey.Serialize(), w.unlockPassword)
	if err != nil {
		return "", fmt.Errorf("failed to encrypt private key: %w", err)
	}
	return encodeEncryptedData(encData), nil
}

// GenerateKey creates a new key pair and returns its address.
func (w *Wallet) GenerateKey() (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	privKey, err := btcec.NewPrivateKey()
	if err != nil {
		return "", fmt.Errorf("failed to generate private key: %w", err)
	}

	pubKey := privKey.PubKey()
	pubKeyHash := btcutil.Hash160(pubKey.SerializeCompressed())
	addr, err := btcutil.NewAddressPubKeyHash(pubKeyHash, w.chainParams)
	if err != nil {
		return "", fmt.Errorf("failed to create address: %w", err)
	}

	addrStr := addr.EncodeAddress()
	w.keys[addrStr] = &KeyPair{
		PrivateKey: privKey,
		PublicKey:  pubKey,
		Address:    addr,
	}

	if err := w.save(); err != nil {
		return "", fmt.Errorf("failed to save wallet: %w", err)
	}

	return addrStr, nil
}

// GetAddresses returns all addresses in the wallet.
func (w *Wallet) GetAddresses() []string {
	w.mu.RLock()
	defer w.mu.RUnlock()

	addrs := make([]string, 0, len(w.keys))
	for addr := range w.keys {
		addrs = append(addrs, addr)
	}
	return addrs
}

// HasKey checks if the wallet has a key for the given address.
func (w *Wallet) HasKey(address string) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	_, ok := w.keys[address]
	return ok
}

// GetKey returns the key pair for the given address.
func (w *Wallet) GetKey(address string) (*KeyPair, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	kp, ok := w.keys[address]
	if !ok {
		return nil, fmt.Errorf("no key found for address: %s", address)
	}
	return kp, nil
}

// EncryptWallet encrypts the wallet with a password.
// This migrates an unencrypted wallet to encrypted format.
// Returns an error if the wallet is already encrypted.
func (w *Wallet) EncryptWallet(password string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.encrypted {
		return fmt.Errorf("wallet is already encrypted")
	}

	// Validate password strength
	if err := validatePassword(password); err != nil {
		return fmt.Errorf("invalid password: %w", err)
	}

	// Generate random salt for password hashing
	salt, err := generatePasswordSalt()
	if err != nil {
		return fmt.Errorf("failed to generate password salt: %w", err)
	}

	// Set encryption state
	w.encrypted = true
	w.locked = false // Starts unlocked after encryption
	w.unlockPassword = password
	w.passwordSalt = salt
	w.passwordHash = hashPassword(password, salt)

	// Save wallet with encryption
	if err := w.save(); err != nil {
		// Rollback encryption state on save failure
		w.encrypted = false
		w.locked = false
		w.unlockPassword = ""
		w.passwordHash = nil
		w.passwordSalt = nil
		w.passwordHash = nil
		return fmt.Errorf("failed to save encrypted wallet: %w", err)
	}

	return nil
}

// Lock locks an encrypted wallet, clearing keys from memory.
// Returns an error if the wallet is not encrypted.
func (w *Wallet) Lock() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.encrypted {
		return fmt.Errorf("wallet is not encrypted")
	}

	if w.locked {
		return fmt.Errorf("wallet is already locked")
	}

	// Clear sensitive data from memory
	// Zero private key bytes before removing references
	for _, kp := range w.keys {
		if kp.PrivateKey != nil {
			// Zero the private key bytes
			privKeyBytes := kp.PrivateKey.Serialize()
			for i := range privKeyBytes {
				privKeyBytes[i] = 0
			}
		}
	}

	// Clear the map and password
	w.keys = make(map[string]*KeyPair)

	// Zero password bytes (convert string to byte slice and zero it)
	// Note: This is best-effort as Go strings are immutable
	if w.unlockPassword != "" {
		passwordBytes := []byte(w.unlockPassword)
		for i := range passwordBytes {
			passwordBytes[i] = 0
		}
		w.unlockPassword = ""
	}

	w.locked = true

	return nil
}

// Unlock unlocks an encrypted wallet with a password.
// Returns an error if the password is incorrect or the wallet is not encrypted.
func (w *Wallet) Unlock(password string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.encrypted {
		return fmt.Errorf("wallet is not encrypted")
	}

	if !w.locked {
		return fmt.Errorf("wallet is already unlocked")
	}

	// Verify password using constant-time comparison to prevent timing attacks
	passwordHash := hashPassword(password, w.passwordSalt)
	if len(passwordHash) != len(w.passwordHash) || subtle.ConstantTimeCompare(passwordHash, w.passwordHash) != 1 {
		return fmt.Errorf("incorrect password")
	}

	// Load wallet file to decrypt keys
	data, err := os.ReadFile(w.walletPath())
	if err != nil {
		return fmt.Errorf("failed to read wallet file: %w", err)
	}

	var wd walletData
	if err := json.Unmarshal(data, &wd); err != nil {
		return fmt.Errorf("failed to parse wallet: %w", err)
	}

	// Set password for decryption
	w.unlockPassword = password

	// Load and decrypt keys
	if err := w.loadKeys(&wd); err != nil {
		w.unlockPassword = ""
		return fmt.Errorf("failed to load keys: %w", err)
	}

	w.locked = false
	return nil
}

// IsEncrypted returns true if the wallet is encrypted.
func (w *Wallet) IsEncrypted() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.encrypted
}

// IsLocked returns true if the wallet is locked.
func (w *Wallet) IsLocked() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.locked
}

// Namecoin-specific opcodes for name operations.
// These opcodes extend Bitcoin's script language for Namecoin's naming system.
const (
	// opNameNew (0xd0) begins a NAME_NEW operation to pre-register a name commitment
	opNameNew = 0xd0

	// opNameFirstUpdate (0xd1) completes name registration by revealing the name
	opNameFirstUpdate = 0xd1

	// opNameUpdate (0xd2) begins a NAME_UPDATE operation to update an existing name's value
	opNameUpdate = 0xd2

	// opDrop (0x75) removes the top stack item
	opDrop = 0x75

	// op2Drop (0x6d) removes the top two stack items
	op2Drop = 0x6d

	// opDup (0x76) duplicates the top stack item (part of P2PKH script)
	opDup = 0x76

	// opHash160 (0xa9) computes RIPEMD160(SHA256(x)) of the top stack item
	opHash160 = 0xa9

	// opEqualVerify (0x88) verifies equality and fails if not equal
	opEqualVerify = 0x88

	// opCheckSig (0xac) verifies a signature against the transaction
	opCheckSig = 0xac
)

// validateNameScriptParams validates common parameters for name scripts.
// Enforces relay policy limit (520 bytes) instead of consensus limit (1023 bytes).
func validateNameScriptParams(name, value string, pubKeyHash []byte) error {
	if len(name) == 0 || len(name) > 255 {
		return fmt.Errorf("invalid name length: %d", len(name))
	}
	if len(value) > config.NameValueRelayLimit {
		return fmt.Errorf("value too large: %d bytes (max %d for relay policy)", len(value), config.NameValueRelayLimit)
	}
	if len(pubKeyHash) != 20 {
		return fmt.Errorf("invalid pubkey hash length: %d", len(pubKeyHash))
	}
	return nil
}

// appendP2PKHScript appends standard P2PKH script to the given script buffer.
func appendP2PKHScript(script, pubKeyHash []byte) []byte {
	script = append(script, opDup, opHash160)
	script = append(script, 0x14) // Push 20 bytes
	script = append(script, pubKeyHash...)
	script = append(script, opEqualVerify, opCheckSig)
	return script
}

// BuildNameUpdateScript creates a NAME_UPDATE output script.
// The script format is: OP_NAME_UPDATE <name> <value> OP_2DROP OP_DROP <P2PKH script>
func BuildNameUpdateScript(name, value string, pubKeyHash []byte) ([]byte, error) {
	if err := validateNameScriptParams(name, value, pubKeyHash); err != nil {
		return nil, err
	}

	script := make([]byte, 0, 128)
	script = append(script, opNameUpdate)
	script = append(script, pushData([]byte(name))...)
	script = append(script, pushData([]byte(value))...)
	script = append(script, op2Drop, opDrop)
	script = appendP2PKHScript(script, pubKeyHash)

	return script, nil
}

// BuildNameNewScript creates a NAME_NEW output script for name pre-registration.
// The script format is: OP_NAME_NEW <hash> OP_2DROP <P2PKH script>
//
// Parameters:
//   - hash: The commitment hash (typically Hash(name || rand))
//   - pubKeyHash: The 20-byte public key hash for the receiving address
//
// Returns the complete script bytes or an error if parameters are invalid.
func BuildNameNewScript(hash, pubKeyHash []byte) ([]byte, error) {
	if len(hash) != 20 {
		return nil, fmt.Errorf("invalid hash length: %d (expected 20)", len(hash))
	}
	if len(pubKeyHash) != 20 {
		return nil, fmt.Errorf("invalid pubkey hash length: %d", len(pubKeyHash))
	}

	script := make([]byte, 0, 64)
	script = append(script, opNameNew)
	script = append(script, pushData(hash)...)
	script = append(script, op2Drop)
	script = appendP2PKHScript(script, pubKeyHash)

	return script, nil
}

// BuildNameFirstUpdateScript creates a NAME_FIRSTUPDATE output script for completing name registration.
// The script format is: OP_NAME_FIRSTUPDATE <name> <rand> <value> OP_2DROP OP_2DROP <P2PKH script>
//
// Parameters:
//   - name: The name being registered (e.g., "d/example")
//   - rand: The random salt used in the NAME_NEW commitment
//   - value: The initial value for the name (typically JSON)
//   - pubKeyHash: The 20-byte public key hash for the receiving address
//
// Returns the complete script bytes or an error if parameters are invalid.
func BuildNameFirstUpdateScript(name, rand, value string, pubKeyHash []byte) ([]byte, error) {
	if err := validateNameScriptParams(name, value, pubKeyHash); err != nil {
		return nil, err
	}

	script := make([]byte, 0, 256)
	script = append(script, opNameFirstUpdate)
	script = append(script, pushData([]byte(name))...)
	script = append(script, pushData([]byte(rand))...)
	script = append(script, pushData([]byte(value))...)
	script = append(script, op2Drop, op2Drop)
	script = appendP2PKHScript(script, pubKeyHash)

	return script, nil
}

// pushData returns the script bytes to push data.
func pushData(data []byte) []byte {
	l := len(data)
	if l == 0 {
		return []byte{0x00} // OP_0
	}
	if l <= 75 {
		return append([]byte{byte(l)}, data...)
	}
	if l <= 255 {
		return append([]byte{0x4c, byte(l)}, data...) // OP_PUSHDATA1
	}
	// OP_PUSHDATA2 for larger data
	return append([]byte{0x4d, byte(l), byte(l >> 8)}, data...)
}

// UTXO represents an unspent transaction output.
type UTXO struct {
	TxHash   chainhash.Hash
	Vout     uint32
	Value    int64  // satoshis
	PkScript []byte // the output script
	Address  string // owner address
}

// buildNameTxBase creates the base transaction structure with inputs and name output.
func buildNameTxBase(utxos []UTXO, nameScript []byte, nameOutValue int64) *wire.MsgTx {
	tx := wire.NewMsgTx(wire.TxVersion)

	for _, utxo := range utxos {
		outPoint := wire.NewOutPoint(&utxo.TxHash, utxo.Vout)
		txIn := wire.NewTxIn(outPoint, nil, nil)
		tx.AddTxIn(txIn)
	}

	tx.AddTxOut(wire.NewTxOut(nameOutValue, nameScript))
	return tx
}

// addChangeOutput adds a change output to the transaction if above dust limit.
func addChangeOutput(tx *wire.MsgTx, changeValue int64, changeAddr btcutil.Address) error {
	if changeValue >= config.DustLimit {
		changeScript, err := txscript.PayToAddrScript(changeAddr)
		if err != nil {
			return fmt.Errorf("failed to create change script: %w", err)
		}
		tx.AddTxOut(wire.NewTxOut(changeValue, changeScript))
	}
	return nil
}

// signTransactionInputs signs all inputs of a transaction using the wallet's keys.
func (w *Wallet) signTransactionInputs(tx *wire.MsgTx, utxos []UTXO) error {
	for i, utxo := range utxos {
		inputKp, ok := w.keys[utxo.Address]
		if !ok {
			return fmt.Errorf("no key for input address: %s", utxo.Address)
		}

		sigHash, err := txscript.CalcSignatureHash(
			utxo.PkScript,
			txscript.SigHashAll,
			tx,
			i,
		)
		if err != nil {
			return fmt.Errorf("failed to calculate signature hash: %w", err)
		}

		sig := ecdsa.Sign(inputKp.PrivateKey, sigHash)
		sigWithHashType := append(sig.Serialize(), byte(txscript.SigHashAll))
		pubKeyBytes := inputKp.PublicKey.SerializeCompressed()

		sigScript, err := txscript.NewScriptBuilder().
			AddData(sigWithHashType).
			AddData(pubKeyBytes).
			Script()
		if err != nil {
			return fmt.Errorf("failed to build sig script: %w", err)
		}

		tx.TxIn[i].SignatureScript = sigScript
	}
	return nil
}

// CreateNameUpdateTx creates a NAME_UPDATE transaction.
// Parameters:
//   - name: the name to update
//   - newValue: the new value for the name
//   - utxos: available UTXOs to spend (must include the name UTXO)
//   - nameUtxoIndex: index of the UTXO that currently holds the name
//   - feeRate: satoshis per byte for the transaction fee
//   - destAddress: optional destination address for the name (nil to keep at current address)
//
// Returns the signed transaction and any error.
func (w *Wallet) CreateNameUpdateTx(
	name, newValue string,
	utxos []UTXO,
	nameUtxoIndex int,
	feeRate int64,
	destAddress btcutil.Address,
) (*wire.MsgTx, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if nameUtxoIndex < 0 || nameUtxoIndex >= len(utxos) {
		return nil, fmt.Errorf("invalid name UTXO index: %d", nameUtxoIndex)
	}

	nameUtxo := utxos[nameUtxoIndex]

	// Get the key for the name UTXO (needed for signing)
	kp, ok := w.keys[nameUtxo.Address]
	if !ok {
		return nil, fmt.Errorf("no key for name owner address: %s", nameUtxo.Address)
	}

	// Calculate total input value
	var totalIn int64
	for _, utxo := range utxos {
		totalIn += utxo.Value
	}

	// Determine the destination pubkey hash
	// If destAddress is provided, the name will be transferred to that address.
	// Otherwise, the name remains at the current owner's address.
	// This enables both simple value updates and ownership transfers.
	var pubKeyHash []byte
	var changeAddr btcutil.Address
	if destAddress != nil {
		// Use provided destination address for name transfer
		switch addr := destAddress.(type) {
		case *btcutil.AddressPubKeyHash:
			pubKeyHash = addr.ScriptAddress()
		default:
			return nil, fmt.Errorf("unsupported destination address type: %T", destAddress)
		}
		changeAddr = destAddress
	} else {
		// Keep at current address (simple value update without ownership transfer)
		pubKeyHash = btcutil.Hash160(kp.PublicKey.SerializeCompressed())
		changeAddr = kp.Address
	}

	// Build NAME_UPDATE output script
	nameScript, err := BuildNameUpdateScript(name, newValue, pubKeyHash)
	if err != nil {
		return nil, fmt.Errorf("failed to build name script: %w", err)
	}

	// Estimate transaction size (rough estimate)
	// Inputs: ~148 bytes each (with signature)
	// Outputs: name output + change output
	estimatedSize := int64(10 + len(utxos)*148 + len(nameScript) + 34 + 34)
	fee := feeRate * estimatedSize

	// Name output value (just above dust)
	nameOutValue := int64(1000)

	// Change calculation
	changeValue := totalIn - nameOutValue - fee
	if changeValue < 0 {
		return nil, fmt.Errorf("insufficient funds: need %d, have %d", nameOutValue+fee, totalIn)
	}

	// Create transaction with inputs and name output
	tx := buildNameTxBase(utxos, nameScript, nameOutValue)

	// Add change output if above dust
	if err := addChangeOutput(tx, changeValue, changeAddr); err != nil {
		return nil, err
	}

	// Sign all inputs
	if err := w.signTransactionInputs(tx, utxos); err != nil {
		return nil, err
	}

	return tx, nil
}

// CreateNameUpdateTxRaw creates a raw NAME_UPDATE transaction that can be signed.
// This is useful when the wallet doesn't have all the private keys.
func CreateNameUpdateTxRaw(
	name, newValue string,
	destAddress btcutil.Address,
	utxos []UTXO,
	feeRate int64,
) (*wire.MsgTx, error) {
	// Get pubkey hash from destination address
	var pubKeyHash []byte
	switch addr := destAddress.(type) {
	case *btcutil.AddressPubKeyHash:
		pubKeyHash = addr.ScriptAddress()
	default:
		return nil, fmt.Errorf("unsupported address type: %T", destAddress)
	}

	// Calculate total input value
	var totalIn int64
	for _, utxo := range utxos {
		totalIn += utxo.Value
	}

	// Build NAME_UPDATE output script
	nameScript, err := BuildNameUpdateScript(name, newValue, pubKeyHash)
	if err != nil {
		return nil, fmt.Errorf("failed to build name script: %w", err)
	}

	// Estimate transaction size
	estimatedSize := int64(10 + len(utxos)*148 + len(nameScript) + 34 + 34)
	fee := feeRate * estimatedSize

	// Name output value
	nameOutValue := int64(1000)

	// Change calculation
	changeValue := totalIn - nameOutValue - fee
	if changeValue < 0 {
		return nil, fmt.Errorf("insufficient funds: need %d, have %d", nameOutValue+fee, totalIn)
	}

	// Create transaction with inputs and name output
	tx := buildNameTxBase(utxos, nameScript, nameOutValue)

	// Add change output if above dust
	if err := addChangeOutput(tx, changeValue, destAddress); err != nil {
		return nil, err
	}

	return tx, nil
}

// CreateNameNewTx creates a NAME_NEW transaction for pre-registering a name commitment.
// This is the first step in the two-phase name registration process to prevent front-running.
//
// Parameters:
//   - randBytes: Random salt value (20 bytes) for the commitment
//   - name: Name to be registered (e.g., "d/example")
//   - utxos: Available UTXOs to fund the transaction
//   - feeRate: Satoshis per byte for transaction fee
//   - ownerAddress: Address that will own the name (used for commitment and change)
//
// Returns:
//   - *wire.MsgTx: Signed NAME_NEW transaction ready to broadcast
//   - []byte: Random bytes used (must be saved for NAME_FIRSTUPDATE)
//   - error: Any error encountered during transaction creation
func (w *Wallet) CreateNameNewTx(
	randBytes []byte,
	name string,
	utxos []UTXO,
	feeRate int64,
	ownerAddress btcutil.Address,
) (*wire.MsgTx, []byte, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if err := validateNameNewInputs(name, randBytes, utxos); err != nil {
		return nil, nil, err
	}

	pubKeyHash, err := extractPubKeyHash(ownerAddress)
	if err != nil {
		return nil, nil, err
	}

	commitHash := ComputeNameNewHash(randBytes, name, w.chainParams)
	nameScript, err := BuildNameNewScript(commitHash, pubKeyHash)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build NAME_NEW script: %w", err)
	}

	nameOutValue := int64(1000)
	tx, err := w.buildNameTransaction(utxos, nameScript, nameOutValue, feeRate, ownerAddress, 0)
	if err != nil {
		return nil, nil, err
	}

	if err := w.signTransactionInputs(tx, utxos); err != nil {
		return nil, nil, err
	}

	return tx, randBytes, nil
}

// validateNameNewInputs validates the inputs for a NAME_NEW transaction.
func validateNameNewInputs(name string, randBytes []byte, utxos []UTXO) error {
	if len(name) == 0 || len(name) > 255 {
		return fmt.Errorf("invalid name length: %d (must be 1-255)", len(name))
	}
	if len(randBytes) == 0 {
		return fmt.Errorf("random bytes cannot be empty")
	}
	if len(utxos) == 0 {
		return fmt.Errorf("no UTXOs provided")
	}
	return nil
}

// extractPubKeyHash extracts the public key hash from a P2PKH address.
func extractPubKeyHash(addr btcutil.Address) ([]byte, error) {
	switch a := addr.(type) {
	case *btcutil.AddressPubKeyHash:
		return a.ScriptAddress(), nil
	default:
		return nil, fmt.Errorf("unsupported address type: %T", addr)
	}
}

// buildNameTransaction creates a transaction with a name output, optional change output, and fee.
// minBurnFee specifies a minimum protocol-mandated burn fee (0 for standard fee-rate only).
func (w *Wallet) buildNameTransaction(
	utxos []UTXO, nameScript []byte, nameOutValue, feeRate int64,
	ownerAddress btcutil.Address, minBurnFee int64,
) (*wire.MsgTx, error) {
	var totalIn int64
	for _, utxo := range utxos {
		totalIn += utxo.Value
	}

	estimatedSize := int64(10 + len(utxos)*148 + len(nameScript) + 34)
	fee := feeRate * estimatedSize
	if fee < minBurnFee {
		fee = minBurnFee
	}

	changeValue := totalIn - nameOutValue - fee
	if changeValue < 0 {
		return nil, fmt.Errorf("insufficient funds: need %d, have %d", nameOutValue+fee, totalIn)
	}

	tx := wire.NewMsgTx(wire.TxVersion)
	for _, utxo := range utxos {
		outPoint := wire.NewOutPoint(&utxo.TxHash, utxo.Vout)
		tx.AddTxIn(wire.NewTxIn(outPoint, nil, nil))
	}

	tx.AddTxOut(wire.NewTxOut(nameOutValue, nameScript))

	if changeValue >= config.DustLimit {
		changeScript, err := txscript.PayToAddrScript(ownerAddress)
		if err != nil {
			return nil, fmt.Errorf("failed to create change script: %w", err)
		}
		tx.AddTxOut(wire.NewTxOut(changeValue, changeScript))
	}

	return tx, nil
}

// CreateNameFirstUpdateTx creates a NAME_FIRSTUPDATE transaction to complete name registration.
// This is the second step in the two-phase registration process, revealing the name and setting
// its initial value. Must be called at least 12 blocks after the NAME_NEW transaction.
//
// Parameters:
//   - name: Name being registered (must match the NAME_NEW commitment)
//   - randHex: Hex-encoded random bytes from the NAME_NEW transaction (for commitment verification)
//   - value: Initial value for the name (max 1023 bytes)
//   - nameNewUtxo: UTXO from the NAME_NEW transaction (must be in utxos slice)
//   - utxos: Available UTXOs to spend (must include nameNewUtxo)
//   - nameNewUtxoIndex: Index of nameNewUtxo in the utxos slice
//   - feeRate: Satoshis per byte for transaction fee
//   - ownerAddress: Address that will own the name (can be different from NAME_NEW address)
//
// Returns:
//   - *wire.MsgTx: Signed NAME_FIRSTUPDATE transaction ready to broadcast
//   - error: Any error encountered during transaction creation
func (w *Wallet) CreateNameFirstUpdateTx(
	name, randHex, value string,
	utxos []UTXO,
	nameNewUtxoIndex int,
	feeRate int64,
	ownerAddress btcutil.Address,
) (*wire.MsgTx, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if err := validateNameFirstUpdateInputs(name, randHex, value, utxos, nameNewUtxoIndex); err != nil {
		return nil, err
	}

	if _, ok := w.keys[utxos[nameNewUtxoIndex].Address]; !ok {
		return nil, fmt.Errorf("no key for NAME_NEW address: %s", utxos[nameNewUtxoIndex].Address)
	}

	pubKeyHash, err := extractPubKeyHash(ownerAddress)
	if err != nil {
		return nil, err
	}

	nameScript, err := BuildNameFirstUpdateScript(name, randHex, value, pubKeyHash)
	if err != nil {
		return nil, fmt.Errorf("failed to build NAME_FIRSTUPDATE script: %w", err)
	}

	nameOutValue := int64(1000)
	tx, err := w.buildNameTransaction(utxos, nameScript, nameOutValue, feeRate, ownerAddress, config.MinNameOperationFee)
	if err != nil {
		return nil, err
	}

	if err := w.signTransactionInputs(tx, utxos); err != nil {
		return nil, err
	}

	return tx, nil
}

// validateNameFirstUpdateInputs validates the inputs for a NAME_FIRSTUPDATE transaction.
func validateNameFirstUpdateInputs(name, randHex, value string, utxos []UTXO, nameNewUtxoIndex int) error {
	if len(name) == 0 || len(name) > 255 {
		return fmt.Errorf("invalid name length: %d (must be 1-255)", len(name))
	}
	if len(randHex) == 0 {
		return fmt.Errorf("randHex cannot be empty")
	}
	if _, err := hex.DecodeString(randHex); err != nil {
		return fmt.Errorf("invalid randHex: must be valid hex string: %w", err)
	}
	if len(value) > config.NameValueRelayLimit {
		return fmt.Errorf("value too large: %d bytes (max %d for relay policy)", len(value), config.NameValueRelayLimit)
	}
	if nameNewUtxoIndex < 0 || nameNewUtxoIndex >= len(utxos) {
		return fmt.Errorf("invalid NAME_NEW UTXO index: %d", nameNewUtxoIndex)
	}
	return nil
}

// SignTransaction signs all inputs in a transaction.
// This is the public API that acquires the lock and validates input count.
func (w *Wallet) SignTransaction(tx *wire.MsgTx, utxos []UTXO) error {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if len(tx.TxIn) != len(utxos) {
		return fmt.Errorf("UTXO count mismatch: tx has %d inputs, got %d UTXOs",
			len(tx.TxIn), len(utxos))
	}

	return w.signTransactionInputs(tx, utxos)
}

// GenerateRand generates random bytes for NAME_NEW commitment.
func GenerateRand() ([]byte, error) {
	randBytes := make([]byte, 20)
	if _, err := rand.Read(randBytes); err != nil {
		return nil, fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return randBytes, nil
}

// ComputeNameNewHash computes the commitment hash for NAME_NEW.
// The commitment is RIPEMD160(SHA256(rand || name || chainID)) to prevent
// cross-chain replay attacks. The chain ID is derived from the network magic bytes.
//
// Parameters:
//   - randBytes: Random salt value (20 bytes recommended)
//   - name: Name to be registered
//   - chainParams: Network parameters containing the unique network magic bytes
//
// Returns: 20-byte commitment hash (RIPEMD160(SHA256(data)))
//
// CRITICAL: This must match the computeCommitHash function in chain/blockchain.go
// for NAME_FIRSTUPDATE validation to succeed. The chain ID prevents replay attacks
// across different Namecoin networks (mainnet, testnet, regtest).
func ComputeNameNewHash(randBytes []byte, name string, chainParams *chaincfg.Params) []byte {
	nameBytes := []byte(name)

	// Extract network magic bytes as chain ID (4 bytes)
	// This must match the logic in chain/blockchain.go:computeCommitHash
	chainID := make([]byte, 4)
	chainID[0] = byte(chainParams.Net)
	chainID[1] = byte(chainParams.Net >> 8)
	chainID[2] = byte(chainParams.Net >> 16)
	chainID[3] = byte(chainParams.Net >> 24)

	// Concatenate: rand || name || chainID
	data := make([]byte, len(randBytes)+len(nameBytes)+len(chainID))
	copy(data, randBytes)
	copy(data[len(randBytes):], nameBytes)
	copy(data[len(randBytes)+len(nameBytes):], chainID)

	return btcutil.Hash160(data)
}
