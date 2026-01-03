# Namecoin Core Test Vectors

This directory contains test vectors from Namecoin Core to ensure validation compatibility.

## Purpose

Test vectors provide real-world blockchain data that can be used to verify that nmcd's
validation logic matches Namecoin Core exactly. This includes:

- Block headers and full blocks (with and without AuxPow)
- Name operation transactions (NAME_NEW, NAME_FIRSTUPDATE, NAME_UPDATE)
- Edge cases and consensus rules
- Known invalid transactions/blocks that should be rejected

## Current Status

**AuxPoW Mainnet Testing Infrastructure:** ✅ **IMPLEMENTED**

The infrastructure for testing AuxPoW validation against real Namecoin mainnet blocks is now complete:

- ✅ Test vector JSON format specification
- ✅ Test vector loader (`chain/testvector.go`)
- ✅ Comprehensive test suite (`chain/mainnet_vectors_test.go`)
- ✅ Extraction script (`scripts/extract_test_vectors.sh`)
- ✅ Placeholder test vectors for critical blocks
- ✅ Detailed extraction guide (`testdata/EXTRACTION_GUIDE.md`)

**Next Steps:**
1. Extract real mainnet blocks using `scripts/extract_test_vectors.sh` (requires Namecoin Core)
2. Re-run tests with `go test -v ./chain -run TestMainnet`
3. Mark PROTOCOL_COMPLIANCE_AUDIT.md Issue #1 as complete after successful validation

## Sources

Test vectors should be extracted from:

1. **Namecoin Core Repository**: https://github.com/namecoin/namecoin-core
   - src/test/data/ directory contains Bitcoin Core test vectors
   - Namecoin-specific vectors need to be extracted from mainnet/testnet

2. **Namecoin Mainnet Blockchain**:
   - Use Namecoin Core's RPC to extract real blocks/transactions
   - Focus on consensus-critical blocks (e.g., AuxPow activation at block 19,200)
   - Include name operations with various edge cases

3. **Namecoin Testnet Blockchain**:
   - Similar to mainnet but easier to generate test cases
   - Can create specific scenarios for testing

## File Format

Test vectors are stored as JSON files with the following structure:

```json
{
  "description": "Block 19200 - AuxPow activation",
  "network": "mainnet",
  "type": "block",
  "height": 19200,
  "hash": "d8a7c3e01e1e95bcee015e6fcc7583a2ca60b79e5a3aa0a171eddd344ada903d",
  "data": "...hex-encoded block data...",
  "valid": true,
  "notes": "First block with AuxPow on mainnet"
}
```

## Test Vector Types

### 1. Block Vectors (`blocks/`)

**Valid blocks:**
- `block_genesis.json` - Genesis block (block 0)
- `block_pre_auxpow.json` - Block before AuxPow activation (< 19,200)
- `block_auxpow_activation.json` - Block 19,200 (AuxPow activation)
- `block_auxpow_merged.json` - Block with AuxPow (>= 19,200)

**Invalid blocks:**
- `block_invalid_pow.json` - Block with insufficient proof of work
- `block_invalid_version.json` - Block at height >= 19,200 without AuxPow bit
- `block_invalid_subsidy.json` - Block with excessive coinbase reward
- `block_invalid_auxpow.json` - Block with malformed AuxPow structure

### 2. Transaction Vectors (`transactions/`)

**Valid name operations:**
- `tx_name_new.json` - NAME_NEW operation
- `tx_name_firstupdate.json` - NAME_FIRSTUPDATE operation
- `tx_name_update.json` - NAME_UPDATE operation

**Invalid name operations:**
- `tx_name_new_duplicate.json` - Duplicate NAME_NEW commitment
- `tx_name_firstupdate_too_early.json` - Before 12-block window
- `tx_name_firstupdate_expired.json` - After 36,000-block window
- `tx_name_update_expired.json` - Update to expired name
- `tx_name_update_theft.json` - NAME_UPDATE not spending name UTXO
- `tx_name_invalid_value.json` - Name value > 1023 bytes
- `tx_name_invalid_namespace.json` - Name without valid namespace prefix
- `tx_name_dust_limit.json` - Name operation output below dust limit

### 3. Chain Vectors (`chains/`)

**Reorganization scenarios:**
- `reorg_simple.json` - Simple 2-block reorganization
- `reorg_with_names.json` - Reorganization affecting name operations

**Difficulty retargeting:**
- `retarget_normal.json` - Normal difficulty adjustment at block 2016
- `retarget_max_increase.json` - Maximum 4x difficulty increase
- `retarget_max_decrease.json` - Maximum 4x difficulty decrease

## How to Add Test Vectors

### From Namecoin Core RPC:

```bash
# Get block data
namecoin-cli getblock <blockhash> 0 > block_<height>.hex

# Get transaction data
namecoin-cli getrawtransaction <txid> > tx_<type>.hex

# Convert to test vector JSON:
{
  "description": "Description of test case",
  "network": "mainnet|testnet|regtest",
  "type": "block|transaction",
  "height": <block_height>,
  "hash": "<block_or_tx_hash>",
  "data": "<hex_data_from_rpc>",
  "valid": true|false,
  "notes": "Additional context"
}
```

### From Namecoin Core Source:

1. Check Namecoin Core's test/functional/ directory for integration tests
2. Look for test data in test/data/ directory
3. Extract relevant Bitcoin Core test vectors that also apply to Namecoin
4. Add Namecoin-specific vectors for name operations

## Running Tests with Vectors

Tests should:

1. Load test vector from JSON file
2. Decode hex data to bytes
3. Deserialize to appropriate type (block, transaction)
4. Run through validation logic
5. Assert expected result (valid/invalid) matches actual result

Example:
```go
func TestBlockVectors(t *testing.T) {
    vectors, err := loadTestVectors("testdata/blocks/*.json")
    if err != nil {
        t.Fatal(err)
    }

    for _, vec := range vectors {
        blockBytes := decodeHex(vec.Data)
        block, err := chain.NewBlockFromBytes(blockBytes)
        
        isValid := err == nil
        if isValid != vec.Valid {
            t.Errorf("%s: expected valid=%v, got valid=%v",
                vec.Description, vec.Valid, isValid)
        }
    }
}
```

## Priority Test Vectors

### High Priority (Critical Consensus Rules):

1. **Block 19,200** - AuxPow activation block from mainnet
2. **Block with AuxPow** - First few merged-mined blocks
3. **Genesis block** - Namecoin genesis block
4. **Subsidy halvings** - Blocks 0, 210000, 420000 (reward changes)

### Medium Priority (Name Operations):

1. **NAME_NEW** - Example from mainnet with valid commitment
2. **NAME_FIRSTUPDATE** - Example completing a name registration
3. **NAME_UPDATE** - Example updating an existing name
4. **Name expiration** - Name at exactly 36,000 blocks since last update

### Low Priority (Edge Cases):

1. **Maximum values** - Names/values at size limits
2. **Boundary conditions** - Operations at exact timing windows
3. **Error cases** - Known invalid operations from mainnet orphaned blocks

## Maintenance

Test vectors should be updated when:

- Namecoin Core consensus rules change
- New edge cases are discovered on mainnet
- Protocol upgrades activate (e.g., new name operation types)
- Bugs are found that aren't covered by existing vectors

## Notes

- Test vectors should be deterministic and reproducible
- Include both valid and invalid cases
- Document the consensus rule being tested
- Keep vectors minimal (don't need full block history, just consensus-critical cases)
- Prefer mainnet vectors over synthetic test data when possible
