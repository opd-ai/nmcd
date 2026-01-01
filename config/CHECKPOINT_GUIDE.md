# Checkpoint Management Guide

## Overview

Checkpoints are hardcoded block heights and their corresponding block hashes that serve several important purposes:

1. **Security**: Protect against long-range reorganization attacks
2. **Performance**: Enable faster initial blockchain synchronization
3. **Validation**: Provide known-good points in the blockchain history

## Current Status

The checkpoint infrastructure is now implemented with genesis blocks for all three networks:
- **Mainnet**: Genesis block (height 0)
- **Testnet**: Genesis block (height 0)
- **Regtest**: Genesis block (height 0)

**Additional checkpoints should be added** from the official Namecoin Core source code to improve security and sync performance.

## How to Add Checkpoints

### Step 1: Find Namecoin Core Checkpoints

The authoritative source for checkpoint values is the Namecoin Core repository:

```
https://github.com/namecoin/namecoin-core/blob/master/src/chainparams.cpp
```

Look for the `CCheckpointData` structure which contains a map of block heights to block hashes.

### Step 2: Convert Hash Format

Namecoin Core uses hexadecimal string format for block hashes. You need to convert them to `chainhash.Hash` format used by this codebase.

**Example conversion:**

Namecoin Core format:
```cpp
{ 19200, uint256S("0x000000000000152bb2c1b95c5e5f9b4bfa9a95b2c1e5f9b4bfa9a95b2c1e5f9b") }
```

nmcd format (in `config/namecoin_params.go`):
```go
var block19200Hash = chainhash.Hash([chainhash.HashSize]byte{
    // Note: byte order is reversed (little-endian)
    0x9b, 0x5f, 0x1e, 0x2c, 0x5b, 0xa9, 0x9a, 0xfa,
    0xb4, 0xf9, 0xe5, 0x1e, 0x2c, 0x5b, 0xa9, 0x9a,
    // ... (32 bytes total)
})
```

**Important**: Block hashes in Bitcoin/Namecoin are stored in little-endian byte order, so you must reverse the bytes when converting from hexadecimal string format.

### Step 3: Add Hash Variables

Add hash variables near the top of `config/namecoin_params.go`, after the genesis hash definitions:

```go
// Checkpoint block hashes
var (
    // Block 19200: AuxPow (merged mining) activation
    mainnetBlock19200Hash = chainhash.Hash([chainhash.HashSize]byte{
        // ... bytes here
    })

    // Block 24000: Name expiration rule change
    mainnetBlock24000Hash = chainhash.Hash([chainhash.HashSize]byte{
        // ... bytes here
    })

    // Add more as needed
)
```

### Step 4: Update Checkpoint Slice

Update the `Checkpoints` field in the appropriate network params struct:

```go
Checkpoints: []chaincfg.Checkpoint{
    {Height: 0, Hash: &MainNetGenesisHash},
    {Height: 19200, Hash: &mainnetBlock19200Hash},
    {Height: 24000, Hash: &mainnetBlock24000Hash},
    // Add more checkpoints here, ordered by ascending height
},
```

### Step 5: Run Tests

After adding checkpoints, run the test suite to verify:

```bash
# Test checkpoints specifically
go test ./config -v -run Checkpoint

# Test the entire config package
go test ./config -v

# Run full test suite
make test
```

## Important Checkpoints to Consider

Based on Namecoin's history, the following blocks are significant candidates for checkpoints:

### Mainnet
- **Block 19200**: AuxPow (merged mining) activation - **CRITICAL CONSENSUS CHANGE**
- **Block 24000**: Name expiration rule change (12,000 → 36,000 blocks)
- **Block 100000**: Regular checkpoint for early chain history
- **Block 200000**: Regular checkpoint
- **Block 300000**: Regular checkpoint
- **Later blocks**: Add recent checkpoints at regular intervals (e.g., every 50,000-100,000 blocks)

### Testnet
- Similar pattern to mainnet but with testnet-specific block hashes
- Check Namecoin Core's testnet checkpoint data

### Regtest
- Typically only the genesis block is needed
- Regtest is for local development/testing where checkpoints are less critical

## Checkpoint Selection Criteria

When selecting which checkpoints to add, consider:

1. **Age**: Focus on blocks old enough to be considered immutable
2. **Consensus changes**: Include blocks with important protocol upgrades
3. **Regular intervals**: Add checkpoints at consistent height intervals for recent history
4. **Community consensus**: Use checkpoints that are widely accepted by the Namecoin community
5. **Verification**: Only add checkpoints that you can verify against multiple sources

## Security Considerations

- **Only add checkpoints from trusted sources** (official Namecoin Core repository)
- **Verify hashes** using multiple independent sources (block explorers, your own synced node)
- **Never modify existing checkpoints** unless there's a critical error (very rare)
- **Test thoroughly** after adding new checkpoints

## Example: Adding a Checkpoint

Here's a complete example of adding the block 19200 checkpoint:

```go
// In config/namecoin_params.go

// After genesis hash definitions, add:
var mainnetBlock19200Hash = chainhash.Hash([chainhash.HashSize]byte{
    // Block 19200 hash (reversed from hex string)
    // TODO: Replace with actual hash from Namecoin Core
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
})

// In NamecoinMainNetParams, update Checkpoints:
Checkpoints: []chaincfg.Checkpoint{
    {Height: 0, Hash: &MainNetGenesisHash},
    {Height: 19200, Hash: &mainnetBlock19200Hash},  // Added
},
```

## Automated Tools

While manual conversion is straightforward, you could create a small tool to automate hash conversion:

```go
// Example helper to convert hex string to byte array
func hexHashToBytes(hexStr string) [32]byte {
    // Remove 0x prefix if present
    hexStr = strings.TrimPrefix(hexStr, "0x")

    // Decode hex to bytes
    bytes, _ := hex.DecodeString(hexStr)

    // Reverse for little-endian
    for i, j := 0, len(bytes)-1; i < j; i, j = i+1, j-1 {
        bytes[i], bytes[j] = bytes[j], bytes[i]
    }

    var result [32]byte
    copy(result[:], bytes)
    return result
}
```

## References

- [Namecoin Core Source Code](https://github.com/namecoin/namecoin-core)
- [Bitcoin Checkpoints Documentation](https://en.bitcoin.it/wiki/Checkpoint)
- [btcd chaincfg Documentation](https://pkg.go.dev/github.com/btcsuite/btcd/chaincfg#Checkpoint)

## Questions or Issues?

If you encounter issues adding checkpoints:
1. Verify the hash format (little-endian byte order)
2. Check that heights are in ascending order
3. Run the test suite to catch errors early
4. Consult the Namecoin Core source code for reference
