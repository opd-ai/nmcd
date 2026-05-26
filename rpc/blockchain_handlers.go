package rpc

import (
	"bytes"
	"encoding/hex"
	"fmt"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

// getBlock returns a block by hash with optional verbose mode.
// Parameters: [blockhash] or [blockhash, verbose]
//   - blockhash (string, required): The block hash as hex string
//   - verbose (bool, optional): If false (default), returns hex-encoded block data.
//     If true, returns JSON object with block details.
func (s *Server) getBlock(req *Request) *Response {
	params, errResp := parseInterfaceParams(req.Params, req.ID, 1, "[blockhash] or [blockhash, verbose]")
	if errResp != nil {
		return errResp
	}

	hash, errResp := parseHashParam(params, 0, req.ID, "block hash")
	if errResp != nil {
		return errResp
	}

	verbose, errResp := parseVerboseParam(params, 1, req.ID)
	if errResp != nil {
		return errResp
	}

	if errResp := s.requireBlockchain(req.ID); errResp != nil {
		return errResp
	}

	block, err := s.blockchain.GetBlockByHash(hash)
	if err != nil {
		return errorResponse(req.ID, -5, fmt.Sprintf("Block not found: %v", err))
	}

	if !verbose {
		return s.serializeBlockHex(block, req.ID)
	}

	return successResponse(req.ID, s.buildVerboseBlockResult(block, hash))
}

// serializeBlockHex returns a hex-encoded block data response.
func (s *Server) serializeBlockHex(block *btcutil.Block, reqID interface{}) *Response {
	blockBytes, err := block.Bytes()
	if err != nil {
		return errorResponse(reqID, -1, fmt.Sprintf("Failed to serialize block: %v", err))
	}
	return successResponse(reqID, fmt.Sprintf("%x", blockBytes))
}

// buildVerboseBlockResult builds a verbose JSON result for a block.
func (s *Server) buildVerboseBlockResult(block *btcutil.Block, hash *chainhash.Hash) map[string]interface{} {
	msgBlock := block.MsgBlock()
	header := msgBlock.Header
	bestSnapshot := s.blockchain.BestSnapshot()

	height, err := s.blockchain.BlockHeightByHash(hash)
	if err != nil {
		height = -1
	}

	var confirmations int32
	if height >= 0 {
		confirmations = bestSnapshot.Height - height + 1
	}

	txs := make([]string, len(msgBlock.Transactions))
	for i, tx := range msgBlock.Transactions {
		txs[i] = tx.TxHash().String()
	}

	result := map[string]interface{}{
		"hash":              hash.String(),
		"confirmations":     confirmations,
		"height":            height,
		"version":           header.Version,
		"merkleroot":        header.MerkleRoot.String(),
		"time":              header.Timestamp.Unix(),
		"nonce":             header.Nonce,
		"bits":              fmt.Sprintf("%08x", header.Bits),
		"difficulty":        getDifficultyRatio(header.Bits, s.blockchain.ChainParams()),
		"previousblockhash": header.PrevBlock.String(),
		"tx":                txs,
	}

	if height >= 0 && height < bestSnapshot.Height {
		nextHash, err := s.blockchain.BlockHashByHeight(height + 1)
		if err == nil {
			result["nextblockhash"] = nextHash.String()
		}
	}

	return result
}

// getBlockHash returns the block hash for a given height.
// Parameters: [height]
// - height (int): The block height
func (s *Server) getBlockHash(req *Request) *Response {
	params, errResp := parseInterfaceParams(req.Params, req.ID, 1, "[height]")
	if errResp != nil {
		return errResp
	}

	height, errResp := parseBlockHeightParam(params[0], req.ID)
	if errResp != nil {
		return errResp
	}

	if errResp := s.requireBlockchain(req.ID); errResp != nil {
		return errResp
	}

	hash, err := s.blockchain.BlockHashByHeight(height)
	if err != nil {
		return errorResponse(req.ID, -8, fmt.Sprintf("Block height out of range: %v", err))
	}

	return successResponse(req.ID, hash.String())
}

// parseBlockHeightParam parses and validates a block height from a JSON parameter.
func parseBlockHeightParam(param, reqID interface{}) (int32, *Response) {
	var height int32
	switch v := param.(type) {
	case float64:
		if v > 2147483647 || v < -2147483648 {
			return 0, errorResponse(reqID, -32602, fmt.Sprintf("Invalid params: height out of int32 range: %v", v))
		}
		height = int32(v)
	default:
		return 0, errorResponse(reqID, -32602, fmt.Sprintf("Invalid params: height must be a number, got %T", param))
	}
	if height < 0 {
		return 0, errorResponse(reqID, -8, "Block height out of range")
	}
	return height, nil
}

// getRawTransaction returns the raw transaction data.
// Parameters: [txid] or [txid, verbose]
//   - txid (string, required): The transaction ID
//   - verbose (bool, optional): If false (default), returns hex-encoded transaction.
//     If true, returns JSON object with transaction details.
//
// Note: This implementation searches through recent blocks to find transactions.
// It does not currently support mempool transactions or a full transaction index.
func (s *Server) getRawTransaction(req *Request) *Response {
	params, errResp := parseInterfaceParams(req.Params, req.ID, 1, "[txid] or [txid, verbose]")
	if errResp != nil {
		return errResp
	}

	txid, errResp := parseHashParam(params, 0, req.ID, "transaction ID")
	if errResp != nil {
		return errResp
	}

	verbose, errResp := parseVerboseParam(params, 1, req.ID)
	if errResp != nil {
		return errResp
	}

	if errResp := s.requireBlockchain(req.ID); errResp != nil {
		return errResp
	}

	foundTx, foundBlockHash, foundHeight, bestHeight, errResp := s.searchTransaction(txid, req.ID)
	if errResp != nil {
		return errResp
	}

	if !verbose {
		return s.serializeTransactionHex(foundTx, req.ID)
	}

	return successResponse(req.ID, buildVerboseTransactionResult(foundTx, foundBlockHash, foundHeight, bestHeight))
}

// searchTransaction searches recent blocks for a transaction by its hash.
// Returns the transaction, block hash, block height, best height, and an error response if not found.
func (s *Server) searchTransaction(txid *chainhash.Hash, reqID interface{}) (*wire.MsgTx, *chainhash.Hash, int32, int32, *Response) {
	bestHeight := s.blockchain.BestSnapshot().Height

	startHeight := bestHeight - 1000
	if startHeight < 0 {
		startHeight = 0
	}

	for height := bestHeight; height >= startHeight; height-- {
		hash, err := s.blockchain.BlockHashByHeight(height)
		if err != nil {
			continue
		}
		block, err := s.blockchain.GetBlockByHash(hash)
		if err != nil {
			continue
		}
		for _, tx := range block.MsgBlock().Transactions {
			txHash := tx.TxHash()
			if txHash.IsEqual(txid) {
				return tx, hash, height, bestHeight, nil
			}
		}
	}

	return nil, nil, 0, 0, errorResponse(reqID, -5, fmt.Sprintf("Transaction not found: %s", txid.String()))
}

// serializeTransactionHex serializes a transaction to hex-encoded string response.
func (s *Server) serializeTransactionHex(tx *wire.MsgTx, reqID interface{}) *Response {
	var buf bytes.Buffer
	if err := tx.Serialize(&buf); err != nil {
		return errorResponse(reqID, -1, fmt.Sprintf("Failed to serialize transaction: %v", err))
	}
	return successResponse(reqID, fmt.Sprintf("%x", buf.Bytes()))
}

// buildVerboseTransactionResult builds a verbose JSON result for a transaction.
func buildVerboseTransactionResult(tx *wire.MsgTx, blockHash *chainhash.Hash, height, bestHeight int32) map[string]interface{} {
	result := map[string]interface{}{
		"txid":          tx.TxHash().String(),
		"version":       tx.Version,
		"locktime":      tx.LockTime,
		"blockhash":     blockHash.String(),
		"blockheight":   height,
		"confirmations": bestHeight - height + 1,
	}

	vin := make([]map[string]interface{}, len(tx.TxIn))
	for i, txIn := range tx.TxIn {
		vin[i] = map[string]interface{}{
			"txid":     txIn.PreviousOutPoint.Hash.String(),
			"vout":     txIn.PreviousOutPoint.Index,
			"sequence": txIn.Sequence,
		}
	}
	result["vin"] = vin

	vout := make([]map[string]interface{}, len(tx.TxOut))
	for i, txOut := range tx.TxOut {
		vout[i] = map[string]interface{}{
			"value": float64(txOut.Value) / 1e8,
			"n":     i,
			"scriptPubKey": map[string]interface{}{
				"hex": fmt.Sprintf("%x", txOut.PkScript),
			},
		}
	}
	result["vout"] = vout

	return result
}

// sendRawTransaction broadcasts a raw transaction to the network.
// Parameters: [hexstring]
//   - hexstring (string, required): The hex-encoded raw transaction
//
// Returns: transaction hash (txid) if broadcast was successful
func (s *Server) sendRawTransaction(req *Request) *Response {
	params, errResp := parseInterfaceParams(req.Params, req.ID, 1, "[hexstring]")
	if errResp != nil {
		return errResp
	}

	hexStr, ok := params[0].(string)
	if !ok {
		return errorResponse(req.ID, -32602, "Invalid params: hexstring must be a string")
	}

	txBytes, err := hex.DecodeString(hexStr)
	if err != nil {
		return errorResponse(req.ID, -22, fmt.Sprintf("TX decode failed: %v", err))
	}

	var tx wire.MsgTx
	if err := tx.Deserialize(bytes.NewReader(txBytes)); err != nil {
		return errorResponse(req.ID, -22, fmt.Sprintf("TX decode failed: %v", err))
	}

	if s.peerMgr == nil {
		return errorResponse(req.ID, -1, "Network not available: peer manager not initialized")
	}

	if err := s.peerMgr.BroadcastTx(&tx); err != nil {
		return errorResponse(req.ID, -25, fmt.Sprintf("Transaction rejected: %v", err))
	}

	txHash := tx.TxHash()
	return successResponse(req.ID, txHash.String())
}
