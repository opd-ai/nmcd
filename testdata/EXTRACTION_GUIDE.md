# Test Vector Extraction Guide

This guide explains how to extract real Namecoin mainnet blocks for test vectors.

## Prerequisites

1. **Namecoin Core** installed and synced to at least block 25,000
2. **namecoin-cli** command-line tool available
3. **jq** for JSON processing (optional but helpful)

## Extracting Block Data

### Method 1: Using Namecoin Core RPC

```bash
# Extract a block at specific height
HEIGHT=19200
BLOCKHASH=$(namecoin-cli getblockhash $HEIGHT)
BLOCKDATA=$(namecoin-cli getblock $BLOCKHASH 0)

# Save to test vector format
cat > testdata/blocks/block_${HEIGHT}.json <<EOF
{
  "description": "Block ${HEIGHT} - Description here",
  "network": "mainnet",
  "type": "block",
  "height": ${HEIGHT},
  "hash": "${BLOCKHASH}",
  "hex": "${BLOCKDATA}",
  "valid": true,
  "notes": "Additional notes here"
}
EOF
```

### Method 2: Using Block Explorer API

Some Namecoin block explorers provide REST APIs to fetch block data in hexadecimal format:

```bash
# Example using a hypothetical API endpoint
HEIGHT=19200
curl -s "https://api.namecoin.explorer/block/height/${HEIGHT}?format=hex" | \
  jq -r '{
    description: "Block \(.height)",
    network: "mainnet",
    type: "block",
    height: .height,
    hash: .hash,
    hex: .hex,
    valid: true,
    notes: ""
  }' > testdata/blocks/block_${HEIGHT}.json
```

## Priority Blocks to Extract

### Critical Consensus Blocks (P0 - Highest Priority)

1. **Block 0** (Genesis Block)
   ```bash
   namecoin-cli getblockhash 0
   ```
   - Validates Namecoin-specific genesis block parsing
   - Critical: Different from Bitcoin genesis block

2. **Block 19,199** (Last Pre-AuxPoW Block)
   ```bash
   namecoin-cli getblockhash 19199
   ```
   - Validates pre-AuxPoW block processing
   - Should NOT have AuxPoW bit set in version

3. **Block 19,200** (AuxPoW Activation)
   ```bash
   namecoin-cli getblockhash 19200
   ```
   - **MOST CRITICAL**: First AuxPoW block
   - Validates AuxPoW activation logic
   - Must have AuxPoW bit set in version
   - Contains first merged mining proof

4. **Block 19,201** (Second AuxPoW Block)
   ```bash
   namecoin-cli getblockhash 19201
   ```
   - Validates AuxPoW consistency
   - Ensures first block wasn't a special case

### Important Blocks (P1 - High Priority)

5. **Blocks 19,202-19,210** (First 10 AuxPoW Blocks)
   ```bash
   for height in {19202..19210}; do
     HASH=$(namecoin-cli getblockhash $height)
     DATA=$(namecoin-cli getblock $HASH 0)
     echo "$height: $HASH"
   done
   ```
   - Validates AuxPoW consistency across multiple blocks
   - Catches edge cases in merkle branch validation

6. **Block 210,000** (First Subsidy Halving)
   ```bash
   namecoin-cli getblockhash 210000
   ```
   - Validates subsidy calculation at halving
   - Expected: 25 NMC block reward (down from 50 NMC)

7. **Block 420,000** (Second Subsidy Halving)
   ```bash
   namecoin-cli getblockhash 420000
   ```
   - Validates second halving
   - Expected: 12.5 NMC block reward

### Representative Blocks (P2 - Medium Priority)

8. **Block 50,000** (Random AuxPoW Block)
   - Validates typical merged mining operations

9. **Block 100,000** (Random AuxPoW Block)
   - Additional validation point

10. **Recent Block** (e.g., Block 500,000+)
    - Validates modern chain state
    - Includes contemporary difficulty adjustments

## Extraction Script

Create `scripts/extract_test_vectors.sh`:

```bash
#!/bin/bash
set -e

# Configuration
NAMECOIN_CLI="${NAMECOIN_CLI:-namecoin-cli}"
OUTPUT_DIR="testdata/blocks"
mkdir -p "$OUTPUT_DIR"

# Function to extract a block
extract_block() {
    local height=$1
    local description=$2
    local notes=$3
    
    echo "Extracting block $height..."
    
    local hash=$($NAMECOIN_CLI getblockhash $height 2>/dev/null || echo "")
    if [ -z "$hash" ]; then
        echo "Error: Could not get block hash for height $height"
        return 1
    fi
    
    local hex_data=$($NAMECOIN_CLI getblock $hash 0 2>/dev/null || echo "")
    if [ -z "$hex_data" ]; then
        echo "Error: Could not get block data for hash $hash"
        return 1
    fi
    
    # Create test vector JSON
    cat > "$OUTPUT_DIR/block_${height}.json" <<EOF
{
  "description": "$description",
  "network": "mainnet",
  "type": "block",
  "height": $height,
  "hash": "$hash",
  "hex": "$hex_data",
  "valid": true,
  "notes": "$notes"
}
EOF
    
    echo "✓ Saved block $height to $OUTPUT_DIR/block_${height}.json"
}

# Extract priority blocks
echo "Extracting Namecoin mainnet test vectors..."
echo "==========================================="

# P0 - Critical
extract_block 0 "Genesis Block" "Namecoin genesis block - validates Namecoin-specific genesis"
extract_block 19199 "Last Pre-AuxPoW Block" "Final block before AuxPoW activation - must NOT have AuxPoW"
extract_block 19200 "AuxPoW Activation Block" "First AuxPoW block - CRITICAL for validation testing"
extract_block 19201 "Second AuxPoW Block" "Validates AuxPoW consistency after activation"

# P1 - High Priority
for height in {19202..19210}; do
    extract_block $height "AuxPoW Block #$(($height - 19199))" "Early AuxPoW block for consistency testing"
done

extract_block 210000 "First Subsidy Halving" "Block reward halves to 25 NMC"
extract_block 420000 "Second Subsidy Halving" "Block reward halves to 12.5 NMC"

# P2 - Medium Priority
extract_block 50000 "Representative AuxPoW Block" "Random block for typical AuxPoW validation"
extract_block 100000 "Representative AuxPoW Block" "Random block for typical AuxPoW validation"

echo "==========================================="
echo "✓ Test vector extraction complete!"
echo "Extracted $(ls -1 $OUTPUT_DIR/*.json 2>/dev/null | wc -l) blocks"
```

Make executable:
```bash
chmod +x scripts/extract_test_vectors.sh
```

Run:
```bash
./scripts/extract_test_vectors.sh
```

## Verifying Extracted Blocks

After extraction, verify the blocks are valid:

```bash
# Check block exists and is valid JSON
jq '.' testdata/blocks/block_19200.json > /dev/null && echo "✓ Valid JSON"

# Verify hex data length (should be non-empty)
jq -r '.hex' testdata/blocks/block_19200.json | wc -c

# Verify all required fields present
jq -e '.description, .network, .type, .height, .hash, .hex, .valid' testdata/blocks/block_19200.json > /dev/null && echo "✓ All fields present"
```

## Test Vector Format Specification

Each test vector file follows this schema:

```json
{
  "description": "Human-readable description of test case",
  "network": "mainnet|testnet|regtest",
  "type": "block",
  "height": 19200,
  "hash": "blockhash in hex (64 characters)",
  "hex": "full block data in hex (variable length)",
  "valid": true,
  "notes": "Additional context about this test case"
}
```

**Fields:**
- `description`: Brief description of what this block tests
- `network`: Which network this block is from
- `type`: Always "block" for block vectors
- `height`: Block height (integer)
- `hash`: Block hash in hexadecimal (64 hex chars = 32 bytes)
- `hex`: Complete serialized block in hexadecimal (includes header, transactions, and AuxPoW if present)
- `valid`: Whether this block should pass validation (true for mainnet blocks)
- `notes`: Additional details about consensus rules being tested

## Automation with GitHub Actions (Future)

Consider adding a GitHub Action to periodically update test vectors:

```yaml
name: Update Test Vectors
on:
  schedule:
    - cron: '0 0 1 * *'  # Monthly
  workflow_dispatch:

jobs:
  update-vectors:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Install Namecoin Core
        run: |
          # Install Namecoin Core from binaries
          # Or use Docker: docker run namecoin/namecoin-core
      - name: Sync blockchain
        run: |
          # Sync to required height
      - name: Extract vectors
        run: ./scripts/extract_test_vectors.sh
      - name: Create PR
        uses: peter-evans/create-pull-request@v4
        with:
          commit-message: Update test vectors from mainnet
          title: 'chore: Update mainnet test vectors'
```

## Alternative: Public Blockchain Data Sources

If you don't have a Namecoin Core node, you can use public sources:

### 1. Namecoin Block Explorers
- https://namecha.in/ - Namecoin block explorer
- https://explorer.namecoin.org/ - Alternative explorer

### 2. Namecoin Core Test Data
Check Namecoin Core repository for existing test data:
```bash
git clone https://github.com/namecoin/namecoin-core.git
cd namecoin-core/src/test/data
# Look for test block data files
```

### 3. Community Resources
- Ask on Namecoin forums/IRC for pre-extracted test blocks
- Check if Namecoin project maintains a test vector repository

## Troubleshooting

### Block Not Found
If `getblockhash` returns an error:
- Ensure Namecoin Core is fully synced to the requested height
- Check that the blockchain index is complete (`-reindex` if needed)

### RPC Connection Issues
If `namecoin-cli` doesn't connect:
- Check `~/.namecoin/namecoin.conf` has `rpcuser` and `rpcpassword` set
- Verify `namecoind` is running: `namecoin-cli getinfo`
- Check RPC port (default: 8336 for mainnet)

### Invalid Hex Data
If hex data seems corrupted:
- Re-extract the block
- Verify with: `echo "$hex" | xxd -r -p | xxd | head`
- Check block size matches expected (header=80 bytes + transactions + AuxPoW)

## Next Steps

After extracting test vectors:
1. Run tests: `go test -v ./chain -run TestMainnetVectors`
2. Verify all blocks parse correctly
3. Compare validation results with Namecoin Core behavior
4. Document any discrepancies found
5. Update PROTOCOL_COMPLIANCE_AUDIT.md with results
