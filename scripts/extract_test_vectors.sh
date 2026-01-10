#!/bin/bash
#
# Extract Namecoin mainnet test vectors for AuxPoW validation testing
#
# This script extracts real Namecoin blockchain blocks and saves them as test vectors
# for validating nmcd's AuxPoW implementation against actual mainnet data.
#
# Prerequisites:
#   - Namecoin Core installed and synced to at least block 210,000
#   - namecoin-cli command available in PATH
#   - jq for JSON processing (optional but recommended)
#
# Usage:
#   ./scripts/extract_test_vectors.sh
#
# Output:
#   Test vector JSON files in testdata/blocks/
#

set -e  # Exit on error

# Configuration
NAMECOIN_CLI="${NAMECOIN_CLI:-namecoin-cli}"
OUTPUT_DIR="testdata/blocks"
VERBOSE="${VERBOSE:-1}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Logging functions
log() {
    if [ "$VERBOSE" = "1" ]; then
        echo -e "${GREEN}[INFO]${NC} $1"
    fi
}

warn() {
    echo -e "${YELLOW}[WARN]${NC} $1" >&2
}

error() {
    echo -e "${RED}[ERROR]${NC} $1" >&2
}

# Check prerequisites
check_prerequisites() {
    log "Checking prerequisites..."
    
    # Check if namecoin-cli is available
    if ! command -v "$NAMECOIN_CLI" &> /dev/null; then
        error "namecoin-cli not found in PATH"
        error "Please install Namecoin Core or set NAMECOIN_CLI environment variable"
        exit 1
    fi
    
    # Check if Namecoin Core is running
    if ! "$NAMECOIN_CLI" listwallets &> /dev/null; then
        error "Cannot connect to Namecoin Core"
        error "Please ensure namecoind is running and RPC credentials are configured"
        error "Check ~/.namecoin/namecoin.conf for rpcuser and rpcpassword"
        exit 1
    fi
    
    # Check blockchain sync status
    local blockcount
    blockcount=$("$NAMECOIN_CLI" getblockcount 2>/dev/null || echo "0")
    log "Connected to Namecoin Core (block height: $blockcount)"
    
    if [ "$blockcount" -lt 19200 ]; then
        warn "Blockchain not synced to AuxPoW activation height (19,200)"
        warn "Current height: $blockcount"
        warn "Some test vectors cannot be extracted"
    fi
    
    # Check if jq is available (optional)
    if command -v jq &> /dev/null; then
        log "jq available for JSON formatting"
        HAS_JQ=1
    else
        warn "jq not found - JSON will not be pretty-printed"
        HAS_JQ=0
    fi
}

# Create output directory
create_output_dir() {
    mkdir -p "$OUTPUT_DIR"
    log "Output directory: $OUTPUT_DIR"
}

# Extract a single block and save as test vector
# Arguments: height, description, notes
extract_block() {
    local height=$1
    local description=$2
    local notes=$3
    local filename="block_${height}"
    
    # Add suffix for special blocks
    case $height in
        0)      filename="block_0_genesis" ;;
        19199)  filename="block_19199_pre_auxpow" ;;
        19200)  filename="block_19200_auxpow_activation" ;;
        19201)  filename="block_19201_second_auxpow" ;;
        50000)  filename="block_50000_representative" ;;
        210000) filename="block_210000_halving1" ;;
    esac
    
    local output_file="$OUTPUT_DIR/${filename}.json"
    
    log "Extracting block $height..."
    
    # Get block hash
    local hash
    if ! hash=$("$NAMECOIN_CLI" getblockhash "$height" 2>/dev/null); then
        error "Failed to get block hash for height $height"
        error "Blockchain may not be synced to this height yet"
        return 1
    fi
    
    log "  Hash: $hash"
    
    # Get block hex data
    local hex_data
    if ! hex_data=$("$NAMECOIN_CLI" getblock "$hash" 0 2>/dev/null); then
        error "Failed to get block data for hash $hash"
        return 1
    fi
    
    local hex_length=${#hex_data}
    log "  Size: $hex_length bytes (hex)"
    
    # Create JSON test vector
    # Note: We use printf to avoid issues with special characters in notes
    local json_content
    json_content=$(cat <<EOF
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
)
    
    # Pretty-print with jq if available
    if [ "$HAS_JQ" = "1" ]; then
        echo "$json_content" | jq '.' > "$output_file"
    else
        echo "$json_content" > "$output_file"
    fi
    
    log "  ✓ Saved to $output_file"
    return 0
}

# Main extraction logic
main() {
    echo "Namecoin Test Vector Extraction"
    echo "================================"
    echo ""
    
    check_prerequisites
    create_output_dir
    
    echo ""
    log "Extracting priority test vectors..."
    echo ""
    
    # Track success/failure
    local total=0
    local success=0
    local failed=0
    
    # P0 - Critical blocks
    echo "Priority 0: Critical Consensus Blocks"
    echo "--------------------------------------"
    
    if extract_block 0 \
        "Namecoin Genesis Block (Block 0)" \
        "Genesis block for Namecoin. Different from Bitcoin genesis block. Critical for validating Namecoin-specific genesis block parsing."; then
        ((success++))
    else
        ((failed++))
    fi
    ((total++))
    
    if extract_block 19199 \
        "Last Pre-AuxPoW Block (Block 19,199)" \
        "Final block before AuxPoW activation. Block version should NOT have AuxPoW bit (0x100) set. This validates pre-AuxPoW block processing still works correctly."; then
        ((success++))
    else
        ((failed++))
    fi
    ((total++))
    
    if extract_block 19200 \
        "AuxPoW Activation Block (Block 19,200) - CRITICAL TEST VECTOR" \
        "**MOST CRITICAL TEST VECTOR** - First block with AuxPoW on Namecoin mainnet. Block version MUST have AuxPoW bit (0x100) set. Contains first merged mining proof with parent Bitcoin block. This block validates: (1) AuxPoW deserialization from network format, (2) Chain ID extraction (should be 1 for Namecoin), (3) Parent block PoW validation, (4) Merkle branch verification for both coinbase and chain merkle trees, (5) AuxPoW version bit enforcement."; then
        ((success++))
    else
        ((failed++))
    fi
    ((total++))
    
    if extract_block 19201 \
        "Second AuxPoW Block (Block 19,201)" \
        "Second AuxPoW block validates that activation wasn't a special case. Ensures AuxPoW validation consistency across multiple blocks."; then
        ((success++))
    else
        ((failed++))
    fi
    ((total++))
    
    echo ""
    echo "Priority 1: Representative Blocks"
    echo "----------------------------------"
    
    if extract_block 50000 \
        "Block 50,000 - Representative AuxPoW Block" \
        "Random AuxPoW block from well after activation. Validates typical merged mining operations with mature difficulty adjustments."; then
        ((success++))
    else
        ((failed++))
    fi
    ((total++))
    
    if extract_block 210000 \
        "Block 210,000 - First Subsidy Halving" \
        "First subsidy halving block. Block reward halves from 50 NMC to 25 NMC. Validates subsidy calculation at halving boundary. Includes AuxPoW validation."; then
        ((success++))
    else
        ((failed++))
    fi
    ((total++))
    
    # Summary
    echo ""
    echo "Extraction Summary"
    echo "=================="
    echo "Total blocks:     $total"
    echo "Successfully extracted: $success"
    echo "Failed:           $failed"
    echo ""
    
    if [ "$success" -eq "$total" ]; then
        log "✓ All test vectors extracted successfully!"
        log ""
        log "Next steps:"
        log "1. Run tests: go test -v ./chain -run TestMainnet"
        log "2. Review test output for validation results"
        log "3. Update PROTOCOL_COMPLIANCE_AUDIT.md if all tests pass"
        exit 0
    elif [ "$success" -gt 0 ]; then
        warn "Partially complete: $success of $total blocks extracted"
        warn "Failed blocks may require blockchain sync to higher height"
        exit 1
    else
        error "Failed to extract any test vectors"
        error "Check Namecoin Core connection and blockchain sync status"
        exit 1
    fi
}

# Run main function
main
