package config

import (
	"math/big"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

// Namecoin network magic bytes
// These values MUST match Namecoin Core's pchMessageStart in src/kernel/chainparams.cpp
var (
	// MainNetMagic is the magic value for Namecoin mainnet
	// Wire bytes: {0xf9, 0xbe, 0xb4, 0xfe}
	MainNetMagic = wire.BitcoinNet(0xf9beb4fe)

	// TestNetMagic is the magic bytes for Namecoin testnet
	// Wire bytes: {0xfa, 0xbf, 0xb5, 0xfe}
	TestNetMagic = wire.BitcoinNet(0xfabfb5fe)

	// RegTestMagic is the magic bytes for Namecoin regtest
	// Wire bytes: {0xfa, 0xbf, 0xb5, 0xda}
	RegTestMagic = wire.BitcoinNet(0xfabfb5da)
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

// genesisCoinbaseTx is the coinbase transaction for the genesis block
var genesisCoinbaseTx = wire.MsgTx{
	Version: 1,
	TxIn: []*wire.TxIn{
		{
			PreviousOutPoint: wire.OutPoint{
				Hash:  chainhash.Hash{},
				Index: 0xffffffff,
			},
			SignatureScript: []byte{
				0x04, 0xff, 0xff, 0x00, 0x1d, 0x01, 0x04, 0x45, /* |.......E| */
				0x54, 0x68, 0x65, 0x20, 0x54, 0x69, 0x6d, 0x65, /* |The Time| */
				0x73, 0x20, 0x30, 0x33, 0x2f, 0x4a, 0x61, 0x6e, /* |s 03/Jan| */
				0x2f, 0x32, 0x30, 0x30, 0x39, 0x20, 0x43, 0x68, /* |/2009 Ch| */
				0x61, 0x6e, 0x63, 0x65, 0x6c, 0x6c, 0x6f, 0x72, /* |ancellor| */
				0x20, 0x6f, 0x6e, 0x20, 0x62, 0x72, 0x69, 0x6e, /* | on brin| */
				0x6b, 0x20, 0x6f, 0x66, 0x20, 0x73, 0x65, 0x63, /* |k of sec| */
				0x6f, 0x6e, 0x64, 0x20, 0x62, 0x61, 0x69, 0x6c, /* |ond bail| */
				0x6f, 0x75, 0x74, 0x20, 0x66, 0x6f, 0x72, 0x20, /* |out for | */
				0x62, 0x61, 0x6e, 0x6b, 0x73, /* |banks| */
			},
			Sequence: 0xffffffff,
		},
	},
	TxOut: []*wire.TxOut{
		{
			Value: 0x12a05f200, // 50 NMC
			PkScript: []byte{
				0x41, 0x04, 0x67, 0x8a, 0xfd, 0xb0, 0xfe, 0x55, /* |A.g....U| */
				0x48, 0x27, 0x19, 0x67, 0xf1, 0xa6, 0x71, 0x30, /* |H'.g..q0| */
				0xb7, 0x10, 0x5c, 0xd6, 0xa8, 0x28, 0xe0, 0x39, /* |..\..(.9| */
				0x09, 0xa6, 0x79, 0x62, 0xe0, 0xea, 0x1f, 0x61, /* |..yb...a| */
				0xde, 0xb6, 0x49, 0xf6, 0xbc, 0x3f, 0x4c, 0xef, /* |..I..?L.| */
				0x38, 0xc4, 0xf3, 0x55, 0x04, 0xe5, 0x1e, 0xc1, /* |8..U....| */
				0x12, 0xde, 0x5c, 0x38, 0x4d, 0xf7, 0xba, 0x0b, /* |..\8M...| */
				0x8d, 0x57, 0x8a, 0x4c, 0x70, 0x2b, 0x6b, 0xf1, /* |.W.Lp+k.| */
				0x1d, 0x5f, 0xac, /* |._.| */
			},
		},
	},
	LockTime: 0,
}

// genesisBlock is the mainnet genesis block
var genesisBlock = wire.MsgBlock{
	Header: wire.BlockHeader{
		Version:    1,
		PrevBlock:  chainhash.Hash{},
		MerkleRoot: MainNetGenesisMerkleRoot,
		Timestamp:  genesisTimestamp,
		Bits:       0x1c007fff,
		Nonce:      0xa21ea192,
	},
	Transactions: []*wire.MsgTx{&genesisCoinbaseTx},
}

// Testnet genesis block data
var testNetGenesisTimestamp = time.Unix(1296688602, 0)

var testNetGenesisMerkleRoot = chainhash.Hash([chainhash.HashSize]byte{
	0x3b, 0xa3, 0xed, 0xfd, 0x7a, 0x7b, 0x12, 0xb2,
	0x7a, 0xc7, 0x2c, 0x3e, 0x67, 0x76, 0x8f, 0x61,
	0x7f, 0xc8, 0x1b, 0xc3, 0x88, 0x8a, 0x51, 0x32,
	0x3a, 0x9f, 0xb8, 0xaa, 0x4b, 0x1e, 0x5e, 0x4a,
})

var testNetGenesisHash = chainhash.Hash([chainhash.HashSize]byte{
	0x43, 0x49, 0x7f, 0xd7, 0xf8, 0x26, 0x95, 0x71,
	0x08, 0xf4, 0xa3, 0x0f, 0xd9, 0xce, 0xc3, 0xae,
	0xba, 0x79, 0x97, 0x20, 0x84, 0xe9, 0x0e, 0xad,
	0x01, 0xea, 0x33, 0x09, 0x00, 0x00, 0x00, 0x00,
})

var testNetGenesisBlock = wire.MsgBlock{
	Header: wire.BlockHeader{
		Version:    1,
		PrevBlock:  chainhash.Hash{},
		MerkleRoot: testNetGenesisMerkleRoot,
		Timestamp:  testNetGenesisTimestamp,
		Bits:       0x1d00ffff,
		Nonce:      0x18aea41a,
	},
	Transactions: []*wire.MsgTx{&genesisCoinbaseTx},
}

// Regtest genesis block data
var regTestGenesisTimestamp = time.Unix(1296688602, 0)

var regTestGenesisMerkleRoot = chainhash.Hash([chainhash.HashSize]byte{
	0x3b, 0xa3, 0xed, 0xfd, 0x7a, 0x7b, 0x12, 0xb2,
	0x7a, 0xc7, 0x2c, 0x3e, 0x67, 0x76, 0x8f, 0x61,
	0x7f, 0xc8, 0x1b, 0xc3, 0x88, 0x8a, 0x51, 0x32,
	0x3a, 0x9f, 0xb8, 0xaa, 0x4b, 0x1e, 0x5e, 0x4a,
})

var regTestGenesisHash = chainhash.Hash([chainhash.HashSize]byte{
	0x06, 0x22, 0x6e, 0x46, 0x11, 0x1a, 0x0b, 0x59,
	0xca, 0xaf, 0x12, 0x60, 0x43, 0xeb, 0x5b, 0xbf,
	0x28, 0xc3, 0x4f, 0x3a, 0x5e, 0x33, 0x2a, 0x1f,
	0xc7, 0xb2, 0xb7, 0x3c, 0xf1, 0x88, 0x91, 0x0f,
})

var regTestGenesisBlock = wire.MsgBlock{
	Header: wire.BlockHeader{
		Version:    1,
		PrevBlock:  chainhash.Hash{},
		MerkleRoot: regTestGenesisMerkleRoot,
		Timestamp:  regTestGenesisTimestamp,
		Bits:       0x207fffff,
		Nonce:      0,
	},
	Transactions: []*wire.MsgTx{&genesisCoinbaseTx},
}

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
	ScriptHashAddrID:        0x0d, // starts with 7
	PrivateKeyID:            0xb4, // starts with 7 (uncompressed) or T (compressed)
	WitnessPubKeyHashAddrID: 0x00, // starts with nc1q
	WitnessScriptHashAddrID: 0x00, // starts with nc1q

	// BIP32 hierarchical deterministic extended key magics
	HDPublicKeyID:  [4]byte{0x04, 0x88, 0xb2, 0x1e}, // xpub
	HDPrivateKeyID: [4]byte{0x04, 0x88, 0xad, 0xe4}, // xprv

	// BIP44 coin type
	HDCoinType: 7, // Namecoin

	// Genesis block
	GenesisBlock: &genesisBlock,
	GenesisHash:  &MainNetGenesisHash,

	// Proof of work parameters
	PowLimit:                 new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 224), big.NewInt(1)),
	PowLimitBits:             0x1d00ffff,
	ReduceMinDifficulty:      false,
	MinDiffReductionTime:     0,
	GenerateSupported:        false,
	CoinbaseMaturity:         100,
	SubsidyReductionInterval: 210000,

	// Checkpoints ordered by height. These protect against long-range reorg attacks
	// and enable faster initial sync. Additional checkpoints should be added from
	// Namecoin Core src/chainparams.cpp (https://github.com/namecoin/namecoin-core)
	//
	// To add checkpoints:
	// 1. Find the latest checkpoint values in Namecoin Core's chainparams.cpp
	// 2. Convert block hashes from Namecoin Core's format to chainhash.Hash format
	// 3. Add entries to the slice below, ordered by ascending height
	//
	// Important checkpoints to consider adding:
	// - Block 19200: AuxPow (merged mining) activation
	// - Block 24000: Name expiration rule change
	// - Regular intervals (e.g., every 50,000-100,000 blocks for recent history)
	Checkpoints: []chaincfg.Checkpoint{
		{Height: 0, Hash: &MainNetGenesisHash},
		// TODO: Add additional checkpoints from Namecoin Core
		// The official checkpoint list can be found at:
		// https://github.com/namecoin/namecoin-core/blob/master/src/chainparams.cpp
	},

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

	// Genesis block
	GenesisBlock: &testNetGenesisBlock,
	GenesisHash:  &testNetGenesisHash,

	// Proof of work parameters
	PowLimit:                 new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 224), big.NewInt(1)),
	PowLimitBits:             0x1d00ffff,
	ReduceMinDifficulty:      true,
	MinDiffReductionTime:     time.Minute * 20,
	GenerateSupported:        false,
	CoinbaseMaturity:         100,
	SubsidyReductionInterval: 210000,

	// Checkpoints ordered by height for testnet
	// See mainnet checkpoints for documentation on how to add more
	Checkpoints: []chaincfg.Checkpoint{
		{Height: 0, Hash: &testNetGenesisHash},
		// TODO: Add additional testnet checkpoints from Namecoin Core
	},

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

	// Genesis block
	GenesisBlock: &regTestGenesisBlock,
	GenesisHash:  &regTestGenesisHash,

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

	// Checkpoints for regtest (typically minimal or none for local testing)
	// Regtest is used for local development and testing, so checkpoints are
	// not typically needed. However, the genesis block is included for consistency.
	Checkpoints: []chaincfg.Checkpoint{
		{Height: 0, Hash: &regTestGenesisHash},
	},

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
