package config

import (
	"math/big"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

// Namecoin network magic bytes
var (
	// MainNetMagic is the magic bytes for Namecoin mainnet: 0xfeb4bef9
	MainNetMagic = wire.BitcoinNet(0xf9beb4fe)
	// TestNetMagic is the magic bytes for Namecoin testnet
	TestNetMagic = wire.BitcoinNet(0x0709110b)
	// RegTestMagic is the magic bytes for Namecoin regtest
	RegTestMagic = wire.BitcoinNet(0xdab5bffa)
)

// Namecoin genesis block hashes
var (
	// MainNetGenesisHash is the hash of the Namecoin mainnet genesis block
	MainNetGenesisHash = chainhash.Hash([chainhash.HashSize]byte{
		0x70, 0xc7, 0xa9, 0xf0, 0xa2, 0xfb, 0x3d, 0x48,
		0xe6, 0x35, 0xa7, 0x0d, 0x5b, 0x15, 0x7c, 0x80,
		0x7e, 0x58, 0xc8, 0xfb, 0x45, 0xeb, 0x2c, 0x5e,
		0x2c, 0xb7, 0x62, 0x00, 0x00, 0x00, 0x00, 0x00,
	})
	// MainNetGenesisMerkleRoot is the merkle root of the Namecoin mainnet genesis block
	MainNetGenesisMerkleRoot = chainhash.Hash([chainhash.HashSize]byte{
		0x09, 0x58, 0x6a, 0x61, 0xb9, 0xee, 0xff, 0xf1,
		0xb6, 0x2c, 0x9d, 0x84, 0x7e, 0xec, 0x6e, 0xf2,
		0xa9, 0xc1, 0x66, 0x84, 0x20, 0x8f, 0x6a, 0x7b,
		0xe8, 0xfa, 0x4c, 0x7b, 0x7c, 0xfa, 0xb4, 0xd4,
	})
)

// Namecoin network ports
const (
	// MainNetDefaultPort is the default port for Namecoin mainnet P2P
	MainNetDefaultPort = "8334"
	// TestNetDefaultPort is the default port for Namecoin testnet P2P
	TestNetDefaultPort = "18334"
	// RegTestDefaultPort is the default port for Namecoin regtest P2P
	RegTestDefaultPort = "18445"
	// MainNetRPCPort is the default port for Namecoin mainnet RPC
	MainNetRPCPort = "8336"
	// TestNetRPCPort is the default port for Namecoin testnet RPC
	TestNetRPCPort = "18336"
)

// Namecoin genesis block timestamp
var genesisTimestamp = time.Unix(1303000001, 0)

// NamecoinMainNetParams defines the Namecoin mainnet network parameters.
// These override btcd's Bitcoin mainnet parameters for Namecoin compatibility.
var NamecoinMainNetParams = chaincfg.Params{
	Name:        "mainnet",
	Net:         MainNetMagic,
	DefaultPort: MainNetDefaultPort,

	// Human-readable part for Bech32 encoded addresses
	Bech32HRPSegwit: "nc",

	// Address encoding magics
	PubKeyHashAddrID:        0x34, // starts with N
	ScriptHashAddrID:        0x0d, // starts with 6
	PrivateKeyID:            0xb4, // starts with 6 or K/L (compressed)
	WitnessPubKeyHashAddrID: 0x00, // starts with nc1q
	WitnessScriptHashAddrID: 0x00, // starts with nc1q

	// BIP32 hierarchical deterministic extended key magics
	HDPublicKeyID:  [4]byte{0x04, 0x88, 0xb2, 0x1e}, // xpub
	HDPrivateKeyID: [4]byte{0x04, 0x88, 0xad, 0xe4}, // xprv

	// BIP44 coin type
	HDCoinType: 7, // Namecoin

	// Genesis block
	GenesisBlock: &wire.MsgBlock{
		Header: wire.BlockHeader{
			Version:    1,
			PrevBlock:  chainhash.Hash{},
			MerkleRoot: MainNetGenesisMerkleRoot,
			Timestamp:  genesisTimestamp,
			Bits:       0x1c007fff, // Namecoin initial difficulty
			Nonce:      0xa21ea192,
		},
	},
	GenesisHash: &MainNetGenesisHash,

	// Proof of work parameters
	PowLimit:                 new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 224), big.NewInt(1)),
	PowLimitBits:             0x1d00ffff,
	ReduceMinDifficulty:      false,
	MinDiffReductionTime:     0,
	GenerateSupported:        false,
	CoinbaseMaturity:         100,
	SubsidyReductionInterval: 210000,

	// Checkpoints (empty for now - can be added later for faster sync)
	Checkpoints: nil,

	// Consensus rule change deployments
	RuleChangeActivationThreshold: 1916,
	MinerConfirmationWindow:       2016,

	// Mempool parameters
	RelayNonStdTxs: false,
}

// NamecoinTestNetParams defines the Namecoin testnet network parameters.
var NamecoinTestNetParams = chaincfg.Params{
	Name:        "testnet",
	Net:         TestNetMagic,
	DefaultPort: TestNetDefaultPort,

	// Human-readable part for Bech32 encoded addresses
	Bech32HRPSegwit: "tn",

	// Address encoding magics
	PubKeyHashAddrID:        0x6f, // starts with m or n
	ScriptHashAddrID:        0xc4, // starts with 2
	PrivateKeyID:            0xef, // starts with 9
	WitnessPubKeyHashAddrID: 0x00,
	WitnessScriptHashAddrID: 0x00,

	// BIP32 hierarchical deterministic extended key magics
	HDPublicKeyID:  [4]byte{0x04, 0x35, 0x87, 0xcf}, // tpub
	HDPrivateKeyID: [4]byte{0x04, 0x35, 0x83, 0x94}, // tprv

	// BIP44 coin type
	HDCoinType: 1, // Testnet (all coins)

	// Proof of work parameters
	PowLimit:                 new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 224), big.NewInt(1)),
	PowLimitBits:             0x1d00ffff,
	ReduceMinDifficulty:      true,
	MinDiffReductionTime:     time.Minute * 20,
	GenerateSupported:        false,
	CoinbaseMaturity:         100,
	SubsidyReductionInterval: 210000,

	// Checkpoints
	Checkpoints: nil,

	// Consensus rule change deployments
	RuleChangeActivationThreshold: 1512,
	MinerConfirmationWindow:       2016,

	// Mempool parameters
	RelayNonStdTxs: true,
}

// NamecoinRegTestParams defines the Namecoin regtest network parameters.
var NamecoinRegTestParams = chaincfg.Params{
	Name:        "regtest",
	Net:         RegTestMagic,
	DefaultPort: RegTestDefaultPort,

	// Human-readable part for Bech32 encoded addresses
	Bech32HRPSegwit: "ncrt",

	// Address encoding magics
	PubKeyHashAddrID:        0x6f, // starts with m or n
	ScriptHashAddrID:        0xc4, // starts with 2
	PrivateKeyID:            0xef, // starts with 9
	WitnessPubKeyHashAddrID: 0x00,
	WitnessScriptHashAddrID: 0x00,

	// BIP32 hierarchical deterministic extended key magics
	HDPublicKeyID:  [4]byte{0x04, 0x35, 0x87, 0xcf}, // tpub
	HDPrivateKeyID: [4]byte{0x04, 0x35, 0x83, 0x94}, // tprv

	// BIP44 coin type
	HDCoinType: 1, // Testnet (all coins)

	// Proof of work parameters
	// Regtest uses a much higher PowLimit (255 bits vs 224 for mainnet/testnet)
	// to allow instant block generation without actual mining hardware.
	// This matches Bitcoin Core's regtest behavior.
	PowLimit:                 new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 255), big.NewInt(1)),
	PowLimitBits:             0x207fffff,
	ReduceMinDifficulty:      true,
	MinDiffReductionTime:     time.Minute * 20,
	GenerateSupported:        true,
	CoinbaseMaturity:         100,
	SubsidyReductionInterval: 150,

	// Checkpoints
	Checkpoints: nil,

	// Consensus rule change deployments
	RuleChangeActivationThreshold: 108,
	MinerConfirmationWindow:       144,

	// Mempool parameters
	RelayNonStdTxs: true,
}

func init() {
	// Register Namecoin networks with btcd's chaincfg
	// This allows proper address encoding/decoding.
	// Note: Registration errors are intentionally ignored because:
	// 1. The network may already be registered from a previous import
	// 2. Tests may register networks multiple times
	// 3. btcd's chaincfg does not provide an "is registered" check
	// The actual error would only occur if a different network with the same
	// magic bytes was already registered, which would be a build-time bug.
	_ = chaincfg.Register(&NamecoinMainNetParams)
	_ = chaincfg.Register(&NamecoinTestNetParams)
	_ = chaincfg.Register(&NamecoinRegTestParams)
}
