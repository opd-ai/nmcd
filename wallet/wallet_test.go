package wallet

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
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

	hash1 := ComputeNameNewHash(rand, "d/example")
	if len(hash1) != 20 {
		t.Errorf("expected 20-byte hash, got %d bytes", len(hash1))
	}

	// Different name should produce different hash
	hash2 := ComputeNameNewHash(rand, "d/different")
	if string(hash1) == string(hash2) {
		t.Error("different names produced same hash")
	}

	// Different rand should produce different hash
	rand2 := make([]byte, 20)
	for i := range rand2 {
		rand2[i] = byte(i + 1)
	}
	hash3 := ComputeNameNewHash(rand2, "d/example")
	if string(hash1) == string(hash3) {
		t.Error("different rand values produced same hash")
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
	if perm != 0600 {
		t.Errorf("expected permissions 0600, got %04o", perm)
	}
}
