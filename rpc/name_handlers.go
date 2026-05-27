package rpc

import (
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/opd-ai/nmcd/chain"
	"github.com/opd-ai/nmcd/namedb"
	"github.com/opd-ai/nmcd/wallet"
)

// nameShow returns information about a name
func (s *Server) nameShow(req *Request) *Response {
	if errResp := s.requireBlockchain(req.ID); errResp != nil {
		return errResp
	}

	var params []string
	if err := json.Unmarshal(req.Params, &params); err != nil || len(params) == 0 {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -32602,
				Message: "Invalid params",
			},
			ID: req.ID,
		}
	}

	name := params[0]
	if errResp := validateNameLength(name, req.ID); errResp != nil {
		return errResp
	}

	record, err := s.blockchain.GetName(name)
	if err != nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -5,
				Message: fmt.Sprintf("Name not found: %s", name),
			},
			ID: req.ID,
		}
	}

	bestHeight := s.blockchain.BestSnapshot().Height
	expiresIn := record.ExpiresAt - bestHeight
	expired := expiresIn < 0

	result := map[string]interface{}{
		"name":       record.Name,
		"value":      record.Value,
		"txid":       record.TxHash.String(),
		"height":     record.Height,
		"expires_in": expiresIn,
		"expired":    expired,
		"address":    record.Address,
	}

	return &Response{
		Jsonrpc: "2.0",
		Result:  result,
		ID:      req.ID,
	}
}

// nameUpdate updates a name's value. This creates a NAME_UPDATE transaction.
// Parameters: ["name", "value"] or ["name", "value", "address"]
// The name must exist and not be expired. If no address is specified, the
// name will be updated to remain at its current address.
//
// Returns the transaction hex that can be broadcast to the network.
// Note: This creates an unsigned transaction template. For full transaction
// signing and broadcasting, the wallet must have the private key for the
// address that owns the name.
func (s *Server) nameUpdate(req *Request) *Response {
	if errResp := s.requireWallet(req.ID); errResp != nil {
		return errResp
	}

	params, errResp := parseStringParams(req.Params, req.ID, 2, "[\"name\", \"value\"] or [\"name\", \"value\", \"address\"]")
	if errResp != nil {
		return errResp
	}

	name := params[0]
	newValue := params[1]

	destAddress, errResp := s.parseOptionalDestAddress(params, req.ID)
	if errResp != nil {
		return errResp
	}

	if errResp := validateNameLength(name, req.ID); errResp != nil {
		return errResp
	}
	if errResp := validateValueSize(newValue, req.ID); errResp != nil {
		return errResp
	}

	record, errResp := s.lookupActiveNameRecord(name, req.ID)
	if errResp != nil {
		return errResp
	}

	if errResp := s.verifyNameOwnership(record, req.ID); errResp != nil {
		return errResp
	}

	utxos, nameUtxoIndex, errResp := s.collectNameUpdateUTXOs(name, record, req.ID)
	if errResp != nil {
		return errResp
	}

	feeRate := int64(1)
	tx, err := s.wallet.CreateNameUpdateTx(name, newValue, utxos, nameUtxoIndex, feeRate, destAddress)
	if err != nil {
		return errorResponse(req.ID, -1, fmt.Sprintf("Failed to create transaction: %v", err))
	}

	result := map[string]interface{}{
		"txid":   tx.TxHash().String(),
		"name":   name,
		"value":  newValue,
		"status": "broadcasted",
	}
	if destAddress != nil {
		result["address"] = destAddress.EncodeAddress()
	}

	return s.broadcastAndRespond(tx, req.ID, result)
}

// parseOptionalDestAddress parses an optional P2PKH destination address from the third parameter.
func (s *Server) parseOptionalDestAddress(params []string, reqID interface{}) (btcutil.Address, *Response) {
	if len(params) < 3 || params[2] == "" {
		return nil, nil
	}
	addr, err := btcutil.DecodeAddress(params[2], s.blockchain.ChainParams())
	if err != nil {
		return nil, errorResponse(reqID, -5, fmt.Sprintf("Invalid destination address: %v", err))
	}
	if _, ok := addr.(*btcutil.AddressPubKeyHash); !ok {
		return nil, errorResponse(reqID, -5, fmt.Sprintf("Destination address must be P2PKH, got: %T", addr))
	}
	return addr, nil
}

// lookupActiveNameRecord retrieves a name record and verifies it is not expired.
func (s *Server) lookupActiveNameRecord(name string, reqID interface{}) (*namedb.NameRecord, *Response) {
	record, err := s.blockchain.GetName(name)
	if err != nil {
		return nil, errorResponse(reqID, -4, fmt.Sprintf("Name not found: %s", name))
	}
	bestHeight := s.blockchain.BestSnapshot().Height
	// Names are valid through ExpiresAt and expire after.
	// A name with ExpiresAt=100 is valid at height 100 but expired at 101.
	// Use strict less-than to match project convention: ExpiresAt < currentHeight means expired.
	if record.ExpiresAt < bestHeight {
		return nil, errorResponse(reqID, -4, fmt.Sprintf("Name expired at block %d (current: %d)", record.ExpiresAt, bestHeight))
	}
	return record, nil
}

// verifyNameOwnership checks that the wallet has the private key for the name's current owner.
func (s *Server) verifyNameOwnership(record *namedb.NameRecord, reqID interface{}) *Response {
	if !s.wallet.HasKey(record.Address) {
		return errorResponse(reqID, -13, fmt.Sprintf("Wallet does not have the private key for address: %s", record.Address))
	}
	return nil
}

// collectNameUpdateUTXOs retrieves and converts the UTXOs needed for a NAME_UPDATE transaction.
func (s *Server) collectNameUpdateUTXOs(name string, record *namedb.NameRecord, reqID interface{}) ([]wallet.UTXO, int, *Response) {
	nameUTXO, err := s.blockchain.GetNameUTXO(name)
	if err != nil {
		return nil, 0, errorResponse(reqID, -1, fmt.Sprintf("Failed to get name UTXO: %v", err))
	}

	walletUTXOs, err := s.blockchain.GetUTXOsForAddress(record.Address)
	if err != nil {
		return nil, 0, errorResponse(reqID, -1, fmt.Sprintf("Failed to get wallet UTXOs: %v", err))
	}

	utxos, nameUtxoIndex := convertAndFindNameUTXO(walletUTXOs, &nameUTXO.TxHash, nameUTXO.OutIndex)
	if nameUtxoIndex == -1 {
		return nil, 0, errorResponse(reqID, -1, "Name UTXO not found in wallet UTXOs")
	}
	return utxos, nameUtxoIndex, nil
}

// convertAndFindNameUTXO converts namedb UTXOs to wallet UTXOs and locates the name UTXO index.
func convertAndFindNameUTXO(dbUTXOs []*namedb.UTXO, nameHash *chainhash.Hash, nameOutIndex uint32) ([]wallet.UTXO, int) {
	var utxos []wallet.UTXO
	nameUtxoIndex := -1
	for _, dbUTXO := range dbUTXOs {
		wUtxo := wallet.UTXO{
			TxHash:   dbUTXO.TxHash,
			Vout:     dbUTXO.OutIndex,
			Value:    dbUTXO.Value,
			PkScript: dbUTXO.PkScript,
			Address:  dbUTXO.Address,
		}
		if dbUTXO.TxHash.IsEqual(nameHash) && dbUTXO.OutIndex == nameOutIndex {
			nameUtxoIndex = len(utxos)
		}
		utxos = append(utxos, wUtxo)
	}
	return utxos, nameUtxoIndex
}

// getWalletAddressAndUTXOs is a helper function that retrieves the wallet address and UTXOs
// for funding name operation transactions. This reduces code duplication between nameNew
// and nameFirstUpdate methods.
//
// Returns:
//   - btcutil.Address: The decoded wallet address
//   - []wallet.UTXO: The converted wallet UTXOs ready for transaction creation
//   - *Response: Error response if any step fails, nil on success
func (s *Server) getWalletAddressAndUTXOs(reqID interface{}) (btcutil.Address, []wallet.UTXO, *Response) {
	// Get a wallet address to own the name
	addresses := s.wallet.GetAddresses()
	if len(addresses) == 0 {
		return nil, nil, &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -1,
				Message: "No addresses in wallet. Create an address first using getnewaddress.",
			},
			ID: reqID,
		}
	}
	ownerAddress := addresses[0] // Use the first address

	// Decode the address
	addr, err := btcutil.DecodeAddress(ownerAddress, s.blockchain.ChainParams())
	if err != nil {
		return nil, nil, &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -5,
				Message: fmt.Sprintf("Invalid address: %v", err),
			},
			ID: reqID,
		}
	}

	// Get wallet UTXOs for funding
	walletUTXOs, err := s.blockchain.GetUTXOsForAddress(ownerAddress)
	if err != nil {
		return nil, nil, &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -1,
				Message: fmt.Sprintf("Failed to get wallet UTXOs: %v", err),
			},
			ID: reqID,
		}
	}

	if len(walletUTXOs) == 0 {
		return nil, nil, &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -6,
				Message: "Insufficient funds. No UTXOs available in wallet.",
			},
			ID: reqID,
		}
	}

	// Convert namedb UTXOs to wallet UTXOs
	var utxos []wallet.UTXO
	for _, dbUTXO := range walletUTXOs {
		wUtxo := wallet.UTXO{
			TxHash:   dbUTXO.TxHash,
			Vout:     dbUTXO.OutIndex,
			Value:    dbUTXO.Value,
			PkScript: dbUTXO.PkScript,
			Address:  dbUTXO.Address,
		}
		utxos = append(utxos, wUtxo)
	}

	return addr, utxos, nil
}

// nameNew creates a NAME_NEW transaction for pre-registering a name commitment.
// This is the first step in the two-phase name registration process to prevent front-running.
//
// Parameters:
//   - name: Name to be registered (e.g., "d/example")
//
// Returns a JSON object with:
//   - txid: Transaction ID of the NAME_NEW transaction
//   - name: The name being registered
//   - rand: Hex-encoded random salt (MUST be saved for NAME_FIRSTUPDATE)
//   - status: "broadcasted" indicating transaction is in mempool
func (s *Server) nameNew(req *Request) *Response {
	if errResp := s.requireWallet(req.ID); errResp != nil {
		return errResp
	}

	params, errResp := parseStringParams(req.Params, req.ID, 1, "[\"name\"]")
	if errResp != nil {
		return errResp
	}

	name := params[0]
	if errResp := validateNameLength(name, req.ID); errResp != nil {
		return errResp
	}

	if errResp := s.checkNameNotActive(name, req.ID); errResp != nil {
		return errResp
	}

	addr, utxos, errResp := s.getWalletAddressAndUTXOs(req.ID)
	if errResp != nil {
		return errResp
	}

	randBytes, err := wallet.GenerateRand()
	if err != nil {
		return errorResponse(req.ID, -1, fmt.Sprintf("Failed to generate random salt for NAME_NEW: %v", err))
	}

	feeRate := int64(1)
	tx, randBytesReturned, err := s.wallet.CreateNameNewTx(randBytes, name, utxos, feeRate, addr)
	if err != nil {
		return errorResponse(req.ID, -1, fmt.Sprintf("Failed to create NAME_NEW transaction: %v", err))
	}

	result := map[string]interface{}{
		"txid":   tx.TxHash().String(),
		"name":   name,
		"rand":   fmt.Sprintf("%x", randBytesReturned),
		"status": "broadcasted",
	}
	return s.broadcastAndRespond(tx, req.ID, result)
}

// checkNameNotActive verifies a name doesn't already exist as an unexpired registration.
func (s *Server) checkNameNotActive(name string, reqID interface{}) *Response {
	existingRecord, err := s.blockchain.GetName(name)
	if err != nil {
		return nil // Name doesn't exist - that's what we want
	}
	bestHeight := s.blockchain.BestSnapshot().Height
	if existingRecord.ExpiresAt >= bestHeight {
		return errorResponse(reqID, -25, fmt.Sprintf("Name already exists and is not expired (expires at block %d, current: %d)", existingRecord.ExpiresAt, bestHeight))
	}
	return nil
}

// nameFirstUpdate creates a NAME_FIRSTUPDATE transaction to complete name registration.
// This is the second step in the two-phase registration process. Must be called at least
// 12 blocks after the NAME_NEW transaction.
//
// Parameters:
//   - name: Name being registered (must match the NAME_NEW commitment)
//   - rand: Hex-encoded random bytes from the NAME_NEW transaction
//   - value: Initial value for the name (max 1023 bytes)
//
// Returns a JSON object with:
//   - txid: Transaction ID of the NAME_FIRSTUPDATE transaction
//   - name: The name being registered
//   - value: The initial value
//   - status: "broadcasted" indicating transaction is in mempool
func (s *Server) nameFirstUpdate(req *Request) *Response {
	if errResp := s.requireWallet(req.ID); errResp != nil {
		return errResp
	}

	params, errResp := parseStringParams(req.Params, req.ID, 3, "[\"name\", \"rand\", \"value\"]")
	if errResp != nil {
		return errResp
	}

	name, randHex, value := params[0], params[1], params[2]

	if errResp := validateNameLength(name, req.ID); errResp != nil {
		return errResp
	}
	if errResp := validateValueSize(value, req.ID); errResp != nil {
		return errResp
	}

	randBytes, err := hex.DecodeString(randHex)
	if err != nil {
		return errorResponse(req.ID, -5, fmt.Sprintf("Invalid rand hex: %v", err))
	}

	if errResp := s.validateNameNewCommitment(randBytes, name, req.ID); errResp != nil {
		return errResp
	}

	addr, utxos, errResp := s.getWalletAddressAndUTXOs(req.ID)
	if errResp != nil {
		return errResp
	}

	nameNewUtxoIndex := findNameNewUTXOIndex(utxos)
	if nameNewUtxoIndex < 0 {
		return errorResponse(req.ID, -1, "No NAME_NEW UTXO found in wallet. Did you run name_new first?")
	}

	feeRate := int64(1)
	tx, err := s.wallet.CreateNameFirstUpdateTx(name, randHex, value, utxos, nameNewUtxoIndex, feeRate, addr)
	if err != nil {
		return errorResponse(req.ID, -1, fmt.Sprintf("Failed to create NAME_FIRSTUPDATE transaction: %v", err))
	}

	result := map[string]interface{}{
		"txid":   tx.TxHash().String(),
		"name":   name,
		"value":  value,
		"status": "broadcasted",
	}
	return s.broadcastAndRespond(tx, req.ID, result)
}

// validateNameNewCommitment validates that a NAME_NEW commitment exists and is within the valid window.
func (s *Server) validateNameNewCommitment(randBytes []byte, name string, reqID interface{}) *Response {
	commitHash := wallet.ComputeNameNewHash(randBytes, name)
	nameNewRecord, err := s.blockchain.GetNameDB().GetNameNew(commitHash)
	if err != nil {
		return errorResponse(reqID, -25, "NAME_NEW commitment not found. You must call name_new first and wait for confirmation.")
	}

	bestHeight := s.blockchain.BestSnapshot().Height
	blocksSinceNameNew := bestHeight - nameNewRecord.Height
	if blocksSinceNameNew < 12 {
		return errorResponse(reqID, -25, fmt.Sprintf("NAME_NEW not confirmed enough. Need 12 blocks, only %d blocks have passed.", blocksSinceNameNew))
	}
	if blocksSinceNameNew > 36000 {
		return errorResponse(reqID, -25, fmt.Sprintf("NAME_NEW commitment expired. Maximum window is 36,000 blocks, but %d blocks have passed.", blocksSinceNameNew))
	}
	return nil
}

// findNameNewUTXOIndex locates the NAME_NEW UTXO index by checking for OP_NAME_NEW opcode in scripts.
// Returns -1 if no NAME_NEW UTXO is found.
func findNameNewUTXOIndex(utxos []wallet.UTXO) int {
	for i, utxo := range utxos {
		if len(utxo.PkScript) > 22 && utxo.PkScript[0] == opNameNew {
			return i
		}
	}
	return -1
}

// nameList returns all names in the database
func (s *Server) nameList(req *Request) *Response {
	if errResp := s.requireBlockchain(req.ID); errResp != nil {
		return errResp
	}

	names, err := s.blockchain.ListNames()
	if err != nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -1,
				Message: fmt.Sprintf("Failed to list names: %v", err),
			},
			ID: req.ID,
		}
	}

	// Format names for response
	result := make([]map[string]interface{}, len(names))
	bestHeight := s.blockchain.BestSnapshot().Height
	for i, record := range names {
		expiresIn := record.ExpiresAt - bestHeight
		expired := expiresIn < 0
		if expiresIn < 0 {
			expiresIn = 0
		}
		result[i] = map[string]interface{}{
			"name":       record.Name,
			"value":      record.Value,
			"txid":       record.TxHash.String(),
			"height":     record.Height,
			"expires_in": expiresIn,
			"expired":    expired,
			"address":    record.Address,
		}
	}

	return &Response{
		Jsonrpc: "2.0",
		Result:  result,
		ID:      req.ID,
	}
}

// nameHistory returns the history of a name, including all past operations.
// Each entry in the history represents a NAME_FIRSTUPDATE or NAME_UPDATE operation.
func (s *Server) nameHistory(req *Request) *Response {
	if errResp := s.requireBlockchain(req.ID); errResp != nil {
		return errResp
	}

	var params []string
	if err := json.Unmarshal(req.Params, &params); err != nil || len(params) == 0 {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -32602,
				Message: "Invalid params: expected ['name']",
			},
			ID: req.ID,
		}
	}

	name := params[0]
	history, err := s.blockchain.GetNameHistory(name)
	if err != nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -1,
				Message: fmt.Sprintf("Failed to get name history: %v", err),
			},
			ID: req.ID,
		}
	}

	// Format history for response.
	// Historical records use 'expires_at' (absolute block height) instead of 'expires_in'
	// because these are past snapshots where calculating blocks remaining would be misleading.
	result := make([]map[string]interface{}, len(history))
	for i, record := range history {
		result[i] = map[string]interface{}{
			"name":       record.Name,
			"value":      record.Value,
			"txid":       record.TxHash.String(),
			"height":     record.Height,
			"expires_at": record.ExpiresAt,
			"address":    record.Address,
		}
	}

	return &Response{
		Jsonrpc: "2.0",
		Result:  result,
		ID:      req.ID,
	}
}

// nameScan scans names with prefix matching and pagination.
// Matches Namecoin Core's name_scan RPC.
// Parameters: [start] [count] where start is the prefix and count is max results (default 500)
func (s *Server) nameScan(req *Request) *Response {
	if errResp := s.requireBlockchain(req.ID); errResp != nil {
		return errResp
	}

	start, count, errResp := parseNameScanParams(req.Params, req.ID)
	if errResp != nil {
		return errResp
	}

	names, err := s.blockchain.ScanNames(start, count)
	if err != nil {
		return errorResponse(req.ID, -32603, fmt.Sprintf("Failed to scan names: %v", err))
	}

	return successResponse(req.ID, formatNameRecords(names, s.blockchain.BestSnapshot().Height))
}

// parseNameScanParams extracts start prefix and count from name_scan parameters.
func parseNameScanParams(rawParams json.RawMessage, reqID interface{}) (string, int, *Response) {
	start := ""
	count := 500

	var params []interface{}
	if err := json.Unmarshal(rawParams, &params); err != nil || len(params) == 0 {
		return start, count, nil
	}

	if len(params) > 0 {
		startStr, ok := params[0].(string)
		if !ok {
			return "", 0, errorResponse(reqID, -32602, "start must be a string")
		}
		start = startStr
	}

	if len(params) > 1 {
		countFloat, ok := params[1].(float64)
		if !ok {
			return "", 0, errorResponse(reqID, -32602, "count must be a number")
		}
		count = int(countFloat)
		if count <= 0 || count > 10000 {
			return "", 0, errorResponse(reqID, -32602, "count must be between 1 and 10000")
		}
	}

	return start, count, nil
}

// formatNameRecords converts name records to JSON-RPC result format.
func formatNameRecords(names []*namedb.NameRecord, currentHeight int32) []map[string]interface{} {
	result := make([]map[string]interface{}, len(names))
	for i, record := range names {
		expiresIn := record.ExpiresAt - currentHeight
		expired := expiresIn < 0
		if expiresIn < 0 {
			expiresIn = 0
		}
		result[i] = map[string]interface{}{
			"name":       record.Name,
			"value":      record.Value,
			"txid":       record.TxHash.String(),
			"height":     record.Height,
			"expires_in": expiresIn,
			"expired":    expired,
			"address":    record.Address,
		}
	}
	return result
}

// namePending returns pending name operations from the mempool.
// Matches Namecoin Core's name_pending RPC.
// Parameters: [] or ["name"] where name is an optional filter
func (s *Server) namePending(req *Request) *Response {
	result := []map[string]interface{}{}

	// Get mempool from peer manager
	if s.peerMgr == nil {
		// No peer manager means no mempool - return empty list
		return &Response{
			Jsonrpc: "2.0",
			Result:  result,
			ID:      req.ID,
		}
	}

	mempool := s.peerMgr.GetMempool()
	if mempool == nil {
		return &Response{
			Jsonrpc: "2.0",
			Result:  result,
			ID:      req.ID,
		}
	}

	// Parse optional name filter from params
	var nameFilter string
	var params []interface{}
	if err := json.Unmarshal(req.Params, &params); err == nil && len(params) > 0 {
		if name, ok := params[0].(string); ok {
			nameFilter = name
		}
	}

	// Get all transactions from mempool and parse name operations
	mempoolTxs := mempool.GetAll()
	for _, tx := range mempoolTxs {
		nameOps := chain.ParseNameOperationsFromTx(tx)
		for _, op := range nameOps {
			// Apply name filter if specified
			if nameFilter != "" && op.Name != nameFilter {
				continue
			}

			// Build result object matching Namecoin Core format
			opResult := map[string]interface{}{
				"name":   op.Name,
				"txid":   op.TxHash.String(),
				"vout":   op.OutputIndex,
				"op":     op.OpType.String(),
				"ismine": false, // Would require wallet lookup
			}

			// Add value for NAME_FIRSTUPDATE and NAME_UPDATE
			// NAME_NEW operations only contain a hash commitment, not a value
			if op.OpType != namedb.NameNew {
				opResult["value"] = op.Value
			}

			result = append(result, opResult)
		}
	}

	return &Response{
		Jsonrpc: "2.0",
		Result:  result,
		ID:      req.ID,
	}
}
