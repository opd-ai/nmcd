// Package wallet provides basic wallet functionality for nmcd.
// It supports key generation, storage, and NAME_UPDATE transaction creation.
package wallet

import (
	"crypto/rand"
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
}

// KeyPair represents a private/public key pair.
type KeyPair struct {
	PrivateKey *btcec.PrivateKey
	PublicKey  *btcec.PublicKey
	Address    btcutil.Address
}

// walletData is the serializable format for wallet storage.
type walletData struct {
	Keys []keyData `json:"keys"`
}

// keyData represents a serializable key pair.
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
func (w *Wallet) load() error {
	data, err := os.ReadFile(w.walletPath())
	if err != nil {
		return err
	}

	var wd walletData
	if err := json.Unmarshal(data, &wd); err != nil {
		return fmt.Errorf("failed to parse wallet: %w", err)
	}

	for _, kd := range wd.Keys {
		privKeyBytes, err := hex.DecodeString(kd.PrivateKeyHex)
		if err != nil {
			return fmt.Errorf("failed to decode private key: %w", err)
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

// save writes the wallet to disk.
func (w *Wallet) save() error {
	wd := walletData{
		Keys: make([]keyData, 0, len(w.keys)),
	}

	for addr, kp := range w.keys {
		wd.Keys = append(wd.Keys, keyData{
			PrivateKeyHex: hex.EncodeToString(kp.PrivateKey.Serialize()),
			Address:       addr,
		})
	}

	data, err := json.MarshalIndent(wd, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize wallet: %w", err)
	}

	// Ensure directory exists
	if err := os.MkdirAll(w.dataDir, 0700); err != nil {
		return fmt.Errorf("failed to create wallet directory: %w", err)
	}

	// Write with restricted permissions
	if err := os.WriteFile(w.walletPath(), data, 0600); err != nil {
		return fmt.Errorf("failed to write wallet: %w", err)
	}

	return nil
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

// Namecoin-specific opcodes for name operations.
// These opcodes extend Bitcoin's script language for Namecoin's naming system.
const (
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

// BuildNameUpdateScript creates a NAME_UPDATE output script.
// The script format is: OP_NAME_UPDATE <name> <value> OP_2DROP OP_DROP <P2PKH script>
func BuildNameUpdateScript(name, value string, pubKeyHash []byte) ([]byte, error) {
	if len(name) == 0 || len(name) > 255 {
		return nil, fmt.Errorf("invalid name length: %d", len(name))
	}
	if len(value) > 1023 {
		return nil, fmt.Errorf("value too large: %d bytes", len(value))
	}
	if len(pubKeyHash) != 20 {
		return nil, fmt.Errorf("invalid pubkey hash length: %d", len(pubKeyHash))
	}

	// Build the script manually
	script := make([]byte, 0, 128)

	// OP_NAME_UPDATE
	script = append(script, opNameUpdate)

	// Push name
	script = append(script, pushData([]byte(name))...)

	// Push value
	script = append(script, pushData([]byte(value))...)

	// OP_2DROP OP_DROP (remove name op data from stack)
	script = append(script, op2Drop, opDrop)

	// Standard P2PKH: OP_DUP OP_HASH160 <pubkeyhash> OP_EQUALVERIFY OP_CHECKSIG
	script = append(script, opDup, opHash160)
	script = append(script, 0x14) // Push 20 bytes
	script = append(script, pubKeyHash...)
	script = append(script, opEqualVerify, opCheckSig)

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

// CreateNameUpdateTx creates a NAME_UPDATE transaction.
// Parameters:
//   - name: the name to update
//   - newValue: the new value for the name
//   - utxos: available UTXOs to spend (must include the name UTXO)
//   - nameUtxoIndex: index of the UTXO that currently holds the name
//   - feeRate: satoshis per byte for the transaction fee
//
// Returns the signed transaction and any error.
func (w *Wallet) CreateNameUpdateTx(
	name, newValue string,
	utxos []UTXO,
	nameUtxoIndex int,
	feeRate int64,
) (*wire.MsgTx, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if nameUtxoIndex < 0 || nameUtxoIndex >= len(utxos) {
		return nil, fmt.Errorf("invalid name UTXO index: %d", nameUtxoIndex)
	}

	nameUtxo := utxos[nameUtxoIndex]

	// Get the key for the name UTXO
	kp, ok := w.keys[nameUtxo.Address]
	if !ok {
		return nil, fmt.Errorf("no key for name owner address: %s", nameUtxo.Address)
	}

	// Calculate total input value
	var totalIn int64
	for _, utxo := range utxos {
		totalIn += utxo.Value
	}

	// Build NAME_UPDATE output script
	pubKeyHash := btcutil.Hash160(kp.PublicKey.SerializeCompressed())
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

	// Create transaction
	tx := wire.NewMsgTx(wire.TxVersion)

	// Add inputs
	for _, utxo := range utxos {
		outPoint := wire.NewOutPoint(&utxo.TxHash, utxo.Vout)
		txIn := wire.NewTxIn(outPoint, nil, nil)
		tx.AddTxIn(txIn)
	}

	// Add NAME_UPDATE output
	tx.AddTxOut(wire.NewTxOut(nameOutValue, nameScript))

	// Add change output if above dust
	if changeValue >= config.DustLimit {
		changeScript, err := txscript.PayToAddrScript(kp.Address)
		if err != nil {
			return nil, fmt.Errorf("failed to create change script: %w", err)
		}
		tx.AddTxOut(wire.NewTxOut(changeValue, changeScript))
	}

	// Sign all inputs
	for i, utxo := range utxos {
		inputKp, ok := w.keys[utxo.Address]
		if !ok {
			return nil, fmt.Errorf("no key for input address: %s", utxo.Address)
		}

		sigHash, err := txscript.CalcSignatureHash(
			utxo.PkScript,
			txscript.SigHashAll,
			tx,
			i,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to calculate signature hash: %w", err)
		}

		sig := ecdsa.Sign(inputKp.PrivateKey, sigHash)
		sigWithHashType := append(sig.Serialize(), byte(txscript.SigHashAll))
		pubKeyBytes := inputKp.PublicKey.SerializeCompressed()

		// Build signature script: <sig> <pubkey>
		sigScript, err := txscript.NewScriptBuilder().
			AddData(sigWithHashType).
			AddData(pubKeyBytes).
			Script()
		if err != nil {
			return nil, fmt.Errorf("failed to build sig script: %w", err)
		}

		tx.TxIn[i].SignatureScript = sigScript
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

	// Create transaction
	tx := wire.NewMsgTx(wire.TxVersion)

	// Add inputs
	for _, utxo := range utxos {
		outPoint := wire.NewOutPoint(&utxo.TxHash, utxo.Vout)
		txIn := wire.NewTxIn(outPoint, nil, nil)
		tx.AddTxIn(txIn)
	}

	// Add NAME_UPDATE output
	tx.AddTxOut(wire.NewTxOut(nameOutValue, nameScript))

	// Add change output if above dust
	if changeValue >= config.DustLimit {
		changeScript, err := txscript.PayToAddrScript(destAddress)
		if err != nil {
			return nil, fmt.Errorf("failed to create change script: %w", err)
		}
		tx.AddTxOut(wire.NewTxOut(changeValue, changeScript))
	}

	return tx, nil
}

// SignTransaction signs all inputs in a transaction.
func (w *Wallet) SignTransaction(tx *wire.MsgTx, utxos []UTXO) error {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if len(tx.TxIn) != len(utxos) {
		return fmt.Errorf("UTXO count mismatch: tx has %d inputs, got %d UTXOs",
			len(tx.TxIn), len(utxos))
	}

	for i, utxo := range utxos {
		kp, ok := w.keys[utxo.Address]
		if !ok {
			return fmt.Errorf("no key for address: %s", utxo.Address)
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

		sig := ecdsa.Sign(kp.PrivateKey, sigHash)
		sigWithHashType := append(sig.Serialize(), byte(txscript.SigHashAll))
		pubKeyBytes := kp.PublicKey.SerializeCompressed()

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

// GenerateRand generates random bytes for NAME_NEW commitment.
func GenerateRand() ([]byte, error) {
	randBytes := make([]byte, 20)
	if _, err := rand.Read(randBytes); err != nil {
		return nil, fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return randBytes, nil
}

// ComputeNameNewHash computes the commitment hash for NAME_NEW.
// hash = RIPEMD160(SHA256(rand || name))
func ComputeNameNewHash(randBytes []byte, name string) []byte {
	data := append(randBytes, []byte(name)...)
	return btcutil.Hash160(data)
}
