package wallet

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/opd-ai/nmcd/config"
)

func TestNewWallet(t *testing.T) {
	tmpDir := t.TempDir()
	w, err := NewWallet(tmpDir, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}

	addrs := w.GetAddresses()
	if len(addrs) != 0 {
		t.Errorf("expected empty wallet, got %d addresses", len(addrs))
	}
}

func TestGenerateKey(t *testing.T) {
	tmpDir := t.TempDir()
	w, err := NewWallet(tmpDir, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}

	addr, err := w.GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	if addr == "" {
		t.Error("generated address is empty")
	}

	if !w.HasKey(addr) {
		t.Error("wallet doesn't have generated key")
	}

	addrs := w.GetAddresses()
	if len(addrs) != 1 {
		t.Errorf("expected 1 address, got %d", len(addrs))
	}
}

func TestWalletPersistence(t *testing.T) {
	tmpDir := t.TempDir()

	// Create wallet and generate key
	w1, err := NewWallet(tmpDir, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}

	addr, err := w1.GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	// Create new wallet instance with same path
	w2, err := NewWallet(tmpDir, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("failed to load wallet: %v", err)
	}

	if !w2.HasKey(addr) {
		t.Error("loaded wallet doesn't have the key")
	}

	addrs := w2.GetAddresses()
	if len(addrs) != 1 {
		t.Errorf("expected 1 address, got %d", len(addrs))
	}
}

func TestBuildNameUpdateScript(t *testing.T) {
	pubKeyHash := make([]byte, 20)
	for i := range pubKeyHash {
		pubKeyHash[i] = byte(i)
	}

	tests := []struct {
		name       string
		nameVal    string
		value      string
		pubKeyHash []byte
		wantErr    bool
	}{
		{
			name:       "valid name and value",
			nameVal:    "d/example",
			value:      `{"ip":"192.168.1.1"}`,
			pubKeyHash: pubKeyHash,
			wantErr:    false,
		},
		{
			name:       "empty name",
			nameVal:    "",
			value:      "test",
			pubKeyHash: pubKeyHash,
			wantErr:    true,
		},
		{
			name:       "name too long",
			nameVal:    string(make([]byte, 256)),
			value:      "test",
			pubKeyHash: pubKeyHash,
			wantErr:    true,
		},
		{
			name:       "value too long",
			nameVal:    "d/test",
			value:      string(make([]byte, 1024)),
			pubKeyHash: pubKeyHash,
			wantErr:    true,
		},
		{
			name:       "invalid pubkey hash",
			nameVal:    "d/test",
			value:      "test",
			pubKeyHash: make([]byte, 10),
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			script, err := BuildNameUpdateScript(tt.nameVal, tt.value, tt.pubKeyHash)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			// Check script starts with NAME_UPDATE opcode
			if len(script) == 0 || script[0] != opNameUpdate {
				t.Error("script doesn't start with OP_NAME_UPDATE")
			}
		})
	}
}

func TestBuildNameNewScript(t *testing.T) {
	hash := make([]byte, 20)
	for i := range hash {
		hash[i] = byte(i + 100)
	}

	pubKeyHash := make([]byte, 20)
	for i := range pubKeyHash {
		pubKeyHash[i] = byte(i)
	}

	tests := []struct {
		name       string
		hash       []byte
		pubKeyHash []byte
		wantErr    bool
	}{
		{
			name:       "valid hash and pubKeyHash",
			hash:       hash,
			pubKeyHash: pubKeyHash,
			wantErr:    false,
		},
		{
			name:       "invalid hash length",
			hash:       make([]byte, 10),
			pubKeyHash: pubKeyHash,
			wantErr:    true,
		},
		{
			name:       "invalid pubkey hash",
			hash:       hash,
			pubKeyHash: make([]byte, 10),
			wantErr:    true,
		},
		{
			name:       "nil hash",
			hash:       nil,
			pubKeyHash: pubKeyHash,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			script, err := BuildNameNewScript(tt.hash, tt.pubKeyHash)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			// Check script starts with NAME_NEW opcode
			if len(script) == 0 || script[0] != opNameNew {
				t.Error("script doesn't start with OP_NAME_NEW")
			}

			// Verify script contains the hash
			// Basic check: script should contain the hash data
			if len(script) < 20 {
				t.Errorf("script too short: %d bytes", len(script))
			}
		})
	}
}

func TestBuildNameFirstUpdateScript(t *testing.T) {
	pubKeyHash := make([]byte, 20)
	for i := range pubKeyHash {
		pubKeyHash[i] = byte(i)
	}

	tests := []struct {
		name       string
		nameVal    string
		rand       string
		value      string
		pubKeyHash []byte
		wantErr    bool
	}{
		{
			name:       "valid registration",
			nameVal:    "d/example",
			rand:       "randomsalt123",
			value:      `{"ip":"192.168.1.1"}`,
			pubKeyHash: pubKeyHash,
			wantErr:    false,
		},
		{
			name:       "empty name",
			nameVal:    "",
			rand:       "salt",
			value:      "test",
			pubKeyHash: pubKeyHash,
			wantErr:    true,
		},
		{
			name:       "name too long",
			nameVal:    string(make([]byte, 256)),
			rand:       "salt",
			value:      "test",
			pubKeyHash: pubKeyHash,
			wantErr:    true,
		},
		{
			name:       "value too long",
			nameVal:    "d/test",
			rand:       "salt",
			value:      string(make([]byte, 1024)),
			pubKeyHash: pubKeyHash,
			wantErr:    true,
		},
		{
			name:       "invalid pubkey hash",
			nameVal:    "d/test",
			rand:       "salt",
			value:      "test",
			pubKeyHash: make([]byte, 10),
			wantErr:    true,
		},
		{
			name:       "empty rand is allowed",
			nameVal:    "d/test",
			rand:       "",
			value:      "test",
			pubKeyHash: pubKeyHash,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			script, err := BuildNameFirstUpdateScript(tt.nameVal, tt.rand, tt.value, tt.pubKeyHash)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			// Check script starts with NAME_FIRSTUPDATE opcode
			if len(script) == 0 || script[0] != opNameFirstUpdate {
				t.Error("script doesn't start with OP_NAME_FIRSTUPDATE")
			}
		})
	}
}

func TestPushData(t *testing.T) {
	tests := []struct {
		name      string
		data      []byte
		wantLen   int
		firstByte byte
	}{
		{
			name:      "empty data",
			data:      []byte{},
			wantLen:   1,
			firstByte: 0x00,
		},
		{
			name:      "short data (1 byte)",
			data:      []byte{0x42},
			wantLen:   2,
			firstByte: 0x01,
		},
		{
			name:      "75 bytes",
			data:      make([]byte, 75),
			wantLen:   76,
			firstByte: 0x4b, // 75
		},
		{
			name:      "76 bytes (OP_PUSHDATA1)",
			data:      make([]byte, 76),
			wantLen:   78, // 1 + 1 + 76
			firstByte: 0x4c,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pushData(tt.data)
			if len(result) != tt.wantLen {
				t.Errorf("expected length %d, got %d", tt.wantLen, len(result))
			}
			if result[0] != tt.firstByte {
				t.Errorf("expected first byte 0x%02x, got 0x%02x", tt.firstByte, result[0])
			}
		})
	}
}

func TestGenerateRand(t *testing.T) {
	r1, err := GenerateRand()
	if err != nil {
		t.Fatalf("failed to generate rand: %v", err)
	}
	if len(r1) != 20 {
		t.Errorf("expected 20 bytes, got %d", len(r1))
	}

	r2, err := GenerateRand()
	if err != nil {
		t.Fatalf("failed to generate rand: %v", err)
	}

	// Should be different
	match := true
	for i := range r1 {
		if r1[i] != r2[i] {
			match = false
			break
		}
	}
	if match {
		t.Error("two random values are identical")
	}
}

func TestComputeNameNewHash(t *testing.T) {
	rand := make([]byte, 20)
	for i := range rand {
		rand[i] = byte(i)
	}

	// Import config package for Namecoin chain params
	mainnetParams := &config.NamecoinMainNetParams
	testnetParams := &config.NamecoinTestNetParams
	regtestParams := &config.NamecoinRegTestParams

	// Test basic hash generation
	hash1 := ComputeNameNewHash(rand, "d/example", mainnetParams)
	if len(hash1) != 20 {
		t.Errorf("expected 20-byte hash, got %d bytes", len(hash1))
	}

	// Different name should produce different hash
	hash2 := ComputeNameNewHash(rand, "d/different", mainnetParams)
	if string(hash1) == string(hash2) {
		t.Error("different names produced same hash")
	}

	// Different rand should produce different hash
	rand2 := make([]byte, 20)
	for i := range rand2 {
		rand2[i] = byte(i + 1)
	}
	hash3 := ComputeNameNewHash(rand2, "d/example", mainnetParams)
	if string(hash1) == string(hash3) {
		t.Error("different rand values produced same hash")
	}

	// CRITICAL: Different networks should produce different hashes
	// This prevents cross-chain replay attacks
	hashMainnet := ComputeNameNewHash(rand, "d/example", mainnetParams)
	hashTestnet := ComputeNameNewHash(rand, "d/example", testnetParams)
	hashRegtest := ComputeNameNewHash(rand, "d/example", regtestParams)

	if string(hashMainnet) == string(hashTestnet) {
		t.Error("mainnet and testnet produced same hash - chain ID not working!")
	}
	if string(hashMainnet) == string(hashRegtest) {
		t.Error("mainnet and regtest produced same hash - chain ID not working!")
	}
	if string(hashTestnet) == string(hashRegtest) {
		t.Error("testnet and regtest produced same hash - chain ID not working!")
	}
}

func TestWalletFilePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	w, err := NewWallet(tmpDir, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}

	_, err = w.GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	// Check file permissions
	walletPath := filepath.Join(tmpDir, "wallet.json")
	info, err := os.Stat(walletPath)
	if err != nil {
		t.Fatalf("failed to stat wallet file: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0o600 {
		t.Errorf("expected permissions 0600, got %04o", perm)
	}
}

func TestCreateNameNewTx(t *testing.T) {
	tmpDir := t.TempDir()
	w, err := NewWallet(tmpDir, &config.NamecoinMainNetParams)
	if err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}

	// Generate a key for the wallet
	addr, err := w.GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	kp, err := w.GetKey(addr)
	if err != nil {
		t.Fatalf("failed to get key: %v", err)
	}

	tests := []struct {
		name        string
		nameToReg   string
		randBytes   []byte
		utxos       []UTXO
		feeRate     int64
		wantErr     bool
		errContains string
	}{
		{
			name:      "valid NAME_NEW",
			nameToReg: "d/example",
			randBytes: []byte("01234567890123456789"), // 20 bytes
			utxos: []UTXO{
				{
					TxHash:   mustParseHash(t, "0000000000000000000000000000000000000000000000000000000000000001"),
					Vout:     0,
					Value:    100000,
					PkScript: mustP2PKHScript(t, kp.Address),
					Address:  addr,
				},
			},
			feeRate: 1,
			wantErr: false,
		},
		{
			name:        "empty name",
			nameToReg:   "",
			randBytes:   []byte("01234567890123456789"),
			utxos:       []UTXO{{Value: 100000, Address: addr, PkScript: mustP2PKHScript(t, kp.Address)}},
			feeRate:     1,
			wantErr:     true,
			errContains: "invalid name length",
		},
		{
			name:        "name too long",
			nameToReg:   string(make([]byte, 256)),
			randBytes:   []byte("01234567890123456789"),
			utxos:       []UTXO{{Value: 100000, Address: addr, PkScript: mustP2PKHScript(t, kp.Address)}},
			feeRate:     1,
			wantErr:     true,
			errContains: "invalid name length",
		},
		{
			name:        "empty random bytes",
			nameToReg:   "d/example",
			randBytes:   []byte{},
			utxos:       []UTXO{{Value: 100000, Address: addr, PkScript: mustP2PKHScript(t, kp.Address)}},
			feeRate:     1,
			wantErr:     true,
			errContains: "random bytes cannot be empty",
		},
		{
			name:        "no UTXOs",
			nameToReg:   "d/example",
			randBytes:   []byte("01234567890123456789"),
			utxos:       []UTXO{},
			feeRate:     1,
			wantErr:     true,
			errContains: "no UTXOs provided",
		},
		{
			name:      "insufficient funds",
			nameToReg: "d/example",
			randBytes: []byte("01234567890123456789"),
			utxos: []UTXO{
				{
					TxHash:   mustParseHash(t, "0000000000000000000000000000000000000000000000000000000000000001"),
					Vout:     0,
					Value:    100, // Too small
					PkScript: mustP2PKHScript(t, kp.Address),
					Address:  addr,
				},
			},
			feeRate:     1,
			wantErr:     true,
			errContains: "insufficient funds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx, rand, err := w.CreateNameNewTx(tt.randBytes, tt.nameToReg, tt.utxos, tt.feeRate, kp.Address)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errContains)
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tx == nil {
				t.Fatal("transaction is nil")
			}

			if rand == nil {
				t.Fatal("random bytes are nil")
			}

			// Verify transaction structure
			if len(tx.TxIn) != len(tt.utxos) {
				t.Errorf("expected %d inputs, got %d", len(tt.utxos), len(tx.TxIn))
			}

			if len(tx.TxOut) == 0 {
				t.Fatal("transaction has no outputs")
			}

			// First output should be NAME_NEW
			if tx.TxOut[0].Value != 1000 {
				t.Errorf("expected NAME_NEW output value 1000, got %d", tx.TxOut[0].Value)
			}

			// Verify signature scripts are present (transaction is signed)
			for i, txIn := range tx.TxIn {
				if len(txIn.SignatureScript) == 0 {
					t.Errorf("input %d has no signature script", i)
				}
			}
		})
	}
}

func TestCreateNameFirstUpdateTx(t *testing.T) {
	tmpDir := t.TempDir()
	w, err := NewWallet(tmpDir, &config.NamecoinMainNetParams)
	if err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}

	// Generate a key for the wallet
	addr, err := w.GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	kp, err := w.GetKey(addr)
	if err != nil {
		t.Fatalf("failed to get key: %v", err)
	}

	tests := []struct {
		name           string
		nameToReg      string
		randHex        string
		value          string
		utxos          []UTXO
		nameNewUtxoIdx int
		feeRate        int64
		wantErr        bool
		errContains    string
	}{
		{
			name:      "valid NAME_FIRSTUPDATE",
			nameToReg: "d/example",
			randHex:   "0123456789abcdef0123456789abcdef01234567",
			value:     `{"ip":"1.2.3.4"}`,
			utxos: []UTXO{
				{
					TxHash:   mustParseHash(t, "0000000000000000000000000000000000000000000000000000000000000001"),
					Vout:     0,
					Value:    2000000, // Sufficient for name output (1000) + burn fee (1,000,000) + change
					PkScript: mustP2PKHScript(t, kp.Address),
					Address:  addr,
				},
			},
			nameNewUtxoIdx: 0,
			feeRate:        1,
			wantErr:        false,
		},
		{
			name:           "empty name",
			nameToReg:      "",
			randHex:        "0123456789abcdef0123456789abcdef01234567",
			value:          `{"ip":"1.2.3.4"}`,
			utxos:          []UTXO{{Value: 100000, Address: addr, PkScript: mustP2PKHScript(t, kp.Address)}},
			nameNewUtxoIdx: 0,
			feeRate:        1,
			wantErr:        true,
			errContains:    "invalid name length",
		},
		{
			name:           "name too long",
			nameToReg:      string(make([]byte, 256)),
			randHex:        "0123456789abcdef0123456789abcdef01234567",
			value:          "test",
			utxos:          []UTXO{{Value: 100000, Address: addr, PkScript: mustP2PKHScript(t, kp.Address)}},
			nameNewUtxoIdx: 0,
			feeRate:        1,
			wantErr:        true,
			errContains:    "invalid name length",
		},
		{
			name:           "value too large",
			nameToReg:      "d/example",
			randHex:        "0123456789abcdef0123456789abcdef01234567",
			value:          string(make([]byte, 1024)),
			utxos:          []UTXO{{Value: 100000, Address: addr, PkScript: mustP2PKHScript(t, kp.Address)}},
			nameNewUtxoIdx: 0,
			feeRate:        1,
			wantErr:        true,
			errContains:    "value too large",
		},
		{
			name:           "invalid UTXO index",
			nameToReg:      "d/example",
			randHex:        "0123456789abcdef0123456789abcdef01234567",
			value:          "test",
			utxos:          []UTXO{{Value: 100000, Address: addr, PkScript: mustP2PKHScript(t, kp.Address)}},
			nameNewUtxoIdx: 5,
			feeRate:        1,
			wantErr:        true,
			errContains:    "invalid NAME_NEW UTXO index",
		},
		{
			name:           "empty randHex",
			nameToReg:      "d/example",
			randHex:        "",
			value:          "test",
			utxos:          []UTXO{{Value: 2000000, Address: addr, PkScript: mustP2PKHScript(t, kp.Address)}},
			nameNewUtxoIdx: 0,
			feeRate:        1,
			wantErr:        true,
			errContains:    "randHex cannot be empty",
		},
		{
			name:           "invalid randHex",
			nameToReg:      "d/example",
			randHex:        "not-valid-hex",
			value:          "test",
			utxos:          []UTXO{{Value: 2000000, Address: addr, PkScript: mustP2PKHScript(t, kp.Address)}},
			nameNewUtxoIdx: 0,
			feeRate:        1,
			wantErr:        true,
			errContains:    "invalid randHex",
		},
		{
			name:      "insufficient funds",
			nameToReg: "d/example",
			randHex:   "0123456789abcdef0123456789abcdef01234567",
			value:     "test",
			utxos: []UTXO{
				{
					TxHash:   mustParseHash(t, "0000000000000000000000000000000000000000000000000000000000000001"),
					Vout:     0,
					Value:    100, // Too small
					PkScript: mustP2PKHScript(t, kp.Address),
					Address:  addr,
				},
			},
			nameNewUtxoIdx: 0,
			feeRate:        1,
			wantErr:        true,
			errContains:    "insufficient funds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx, err := w.CreateNameFirstUpdateTx(
				tt.nameToReg,
				tt.randHex,
				tt.value,
				tt.utxos,
				tt.nameNewUtxoIdx,
				tt.feeRate,
				kp.Address,
			)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errContains)
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tx == nil {
				t.Fatal("transaction is nil")
			}

			// Verify transaction structure
			if len(tx.TxIn) != len(tt.utxos) {
				t.Errorf("expected %d inputs, got %d", len(tt.utxos), len(tx.TxIn))
			}

			if len(tx.TxOut) == 0 {
				t.Fatal("transaction has no outputs")
			}

			// First output should be NAME_FIRSTUPDATE
			if tx.TxOut[0].Value != 1000 {
				t.Errorf("expected NAME_FIRSTUPDATE output value 1000, got %d", tx.TxOut[0].Value)
			}

			// Verify signature scripts are present (transaction is signed)
			for i, txIn := range tx.TxIn {
				if len(txIn.SignatureScript) == 0 {
					t.Errorf("input %d has no signature script", i)
				}
			}
		})
	}
}

// mustParseHash parses a hex string into a chainhash.Hash and panics on error
func mustParseHash(t *testing.T, hexStr string) chainhash.Hash {
	t.Helper()
	hash, err := chainhash.NewHashFromStr(hexStr)
	if err != nil {
		t.Fatalf("failed to parse hash: %v", err)
	}
	return *hash
}

// mustP2PKHScript creates a P2PKH script for an address and panics on error
func mustP2PKHScript(t *testing.T, addr btcutil.Address) []byte {
	t.Helper()
	script, err := txscript.PayToAddrScript(addr)
	if err != nil {
		t.Fatalf("failed to create P2PKH script: %v", err)
	}
	return script
}
