package rpc

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/opd-ai/nmcd/namedb"
)

// getNewAddress generates a new address in the wallet and returns it.
// This method creates a new key pair and persists it to the wallet file.
func (s *Server) getNewAddress(req *Request) *Response {
	if s.wallet == nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -1,
				Message: "Wallet not initialized. Start the node with wallet enabled.",
			},
			ID: req.ID,
		}
	}

	address, err := s.wallet.GenerateKey()
	if err != nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -1,
				Message: fmt.Sprintf("Failed to generate address: %v", err),
			},
			ID: req.ID,
		}
	}

	return &Response{
		Jsonrpc: "2.0",
		Result:  address,
		ID:      req.ID,
	}
}

// listAddresses returns all addresses in the wallet.
// Returns an array of address strings currently stored in the wallet.
func (s *Server) listAddresses(req *Request) *Response {
	if s.wallet == nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -1,
				Message: "Wallet not initialized. Start the node with wallet enabled.",
			},
			ID: req.ID,
		}
	}

	addresses := s.wallet.GetAddresses()

	return &Response{
		Jsonrpc: "2.0",
		Result:  addresses,
		ID:      req.ID,
	}
}

// walletPassphrase unlocks the wallet with a password for a specified time.
// Parameters: [password, timeout]
//   - password (string, required): The wallet password
//   - timeout (int, optional): Time in seconds to keep wallet unlocked (default: 60)
//
// Returns: null on success
// Errors:
//   - -1: Wallet not initialized
//   - -13: Wallet is not encrypted
//   - -14: Incorrect password
func (s *Server) walletPassphrase(req *Request) *Response {
	if errResp := s.requireWallet(req.ID); errResp != nil {
		return errResp
	}

	params, errResp := parseInterfaceParams(req.Params, req.ID, 1, "[password] or [password, timeout]")
	if errResp != nil {
		return errResp
	}

	password, ok := params[0].(string)
	if !ok {
		return errorResponse(req.ID, -32602, "Invalid password parameter: expected string")
	}

	timeout, errResp := parsePassphraseTimeout(params, req.ID)
	if errResp != nil {
		return errResp
	}

	if !s.wallet.IsEncrypted() {
		return errorResponse(req.ID, -13, "Wallet is not encrypted")
	}

	if err := s.wallet.Unlock(password); err != nil {
		s.logWarn("wallet unlock attempt failed", "error", err)
		return errorResponse(req.ID, -14, "authentication failed")
	}

	// Cancel any existing auto-lock timer and create a new one.
	// Lock ordering: always acquire autoLockMu before the wallet lock to
	// prevent deadlock between the callback and walletLock.
	// A generation counter ensures a superseded callback is a no-op even
	// when Stop() returns false (the callback has already started running).
	s.autoLockMu.Lock()
	if s.autoLockTimer != nil {
		s.autoLockTimer.Stop()
		s.autoLockTimer = nil
	}
	s.autoLockGen++
	gen := s.autoLockGen
	s.autoLockTimer = time.AfterFunc(time.Duration(timeout)*time.Second, func() {
		// Take autoLockMu first (consistent lock order: autoLockMu then wallet).
		s.autoLockMu.Lock()
		if s.autoLockGen != gen {
			// This timer was superseded by a newer walletpassphrase or walletlock call.
			s.autoLockMu.Unlock()
			return
		}
		s.autoLockTimer = nil
		s.autoLockMu.Unlock()
		// Lock the wallet outside autoLockMu to avoid lock-order inversion.
		if err := s.wallet.Lock(); err != nil {
			s.logError("auto-lock: failed to lock wallet", "error", err)
		}
	})
	s.autoLockMu.Unlock()

	return successResponse(req.ID, nil)
}

// parsePassphraseTimeout extracts the optional timeout parameter (default 60 seconds).
func parsePassphraseTimeout(params []interface{}, reqID interface{}) (int, *Response) {
	if len(params) <= 1 {
		return 60, nil
	}
	timeoutFloat, ok := params[1].(float64)
	if !ok {
		return 0, errorResponse(reqID, -32602, "Invalid timeout parameter: expected integer")
	}
	timeout := int(timeoutFloat)
	if timeout <= 0 {
		return 0, errorResponse(reqID, -32602, "Invalid timeout: must be positive")
	}
	return timeout, nil
}

// walletLock locks the wallet, removing keys from memory.
// Parameters: none
// Returns: null on success
// Errors:
//   - -1: Wallet not initialized
//   - -13: Wallet is not encrypted
func (s *Server) walletLock(req *Request) *Response {
	if s.wallet == nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -1,
				Message: "Wallet not initialized. Start the node with wallet enabled.",
			},
			ID: req.ID,
		}
	}

	if !s.wallet.IsEncrypted() {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -13,
				Message: "Wallet is not encrypted",
			},
			ID: req.ID,
		}
	}

	// Cancel any active auto-lock timer when manually locking.
	// Incrementing the generation invalidates any in-flight callback.
	s.autoLockMu.Lock()
	if s.autoLockTimer != nil {
		s.autoLockTimer.Stop()
		s.autoLockTimer = nil
	}
	s.autoLockGen++
	s.autoLockMu.Unlock()

	if err := s.wallet.Lock(); err != nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -1,
				Message: fmt.Sprintf("Failed to lock wallet: %v", err),
			},
			ID: req.ID,
		}
	}

	return &Response{
		Jsonrpc: "2.0",
		Result:  nil,
		ID:      req.ID,
	}
}

// encryptWallet encrypts the wallet with a password.
// Parameters: [password]
//   - password (string, required): The password to encrypt the wallet with
//
// Returns: null on success
// Errors:
//   - -1: Wallet not initialized or already encrypted
//   - -8: Invalid password (too weak)
func (s *Server) encryptWallet(req *Request) *Response {
	if s.wallet == nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -1,
				Message: "Wallet not initialized. Start the node with wallet enabled.",
			},
			ID: req.ID,
		}
	}

	var params []interface{}
	if err := json.Unmarshal(req.Params, &params); err != nil || len(params) == 0 {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -32602,
				Message: "Invalid params: expected [password]",
			},
			ID: req.ID,
		}
	}

	password, ok := params[0].(string)
	if !ok {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -32602,
				Message: "Invalid password parameter: expected string",
			},
			ID: req.ID,
		}
	}

	// Encrypt wallet
	if err := s.wallet.EncryptWallet(password); err != nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -8,
				Message: fmt.Sprintf("Failed to encrypt wallet: %v", err),
			},
			ID: req.ID,
		}
	}

	return &Response{
		Jsonrpc: "2.0",
		Result:  "Wallet encrypted successfully. Please backup your wallet and remember your password.",
		ID:      req.ID,
	}
}

// getBalance returns the total balance for all wallet addresses.
// Parameters: [] (no parameters required)
// Returns: balance in NMC as a float
func (s *Server) getBalance(req *Request) *Response {
	if s.wallet == nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -1,
				Message: "Wallet not initialized. Start the node with wallet enabled.",
			},
			ID: req.ID,
		}
	}

	if s.blockchain == nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -32603,
				Message: "Blockchain not initialized",
			},
			ID: req.ID,
		}
	}

	// Get all wallet addresses
	addresses := s.wallet.GetAddresses()
	if len(addresses) == 0 {
		return &Response{
			Jsonrpc: "2.0",
			Result:  0.0,
			ID:      req.ID,
		}
	}

	// Sum up UTXOs for all addresses
	var totalSatoshis int64
	var errorCount int
	for _, addr := range addresses {
		utxos, err := s.blockchain.GetUTXOsForAddress(addr)
		if err != nil {
			errorCount++
			s.logWarn("failed to get UTXOs for address", "address", addr, "error", err)
			continue // Skip addresses with errors
		}
		for _, utxo := range utxos {
			totalSatoshis += utxo.Value
		}
	}

	if errorCount > 0 {
		s.logWarn("getbalance returned incomplete results", "skipped_addresses", errorCount)
	}

	// Convert satoshis to NMC (1 NMC = 100,000,000 satoshis)
	balance := float64(totalSatoshis) / 1e8

	return &Response{
		Jsonrpc: "2.0",
		Result:  balance,
		ID:      req.ID,
	}
}

// listUnspent returns all unspent transaction outputs for wallet addresses.
// Parameters: [] or [minconf] or [minconf, maxconf] or [minconf, maxconf, [addresses]]
//   - minconf (int, optional): Minimum confirmations (default: 1)
//   - maxconf (int, optional): Maximum confirmations (default: 9999999)
//   - addresses (array, optional): Filter by addresses (default: all wallet addresses)
//
// Returns array of UTXO objects with txid, vout, address, amount, confirmations, etc.
func (s *Server) listUnspent(req *Request) *Response {
	if err := s.validateListUnspentState(); err != nil {
		return err
	}

	minConf, maxConf, filterAddrs := s.parseListUnspentParams(req.Params)
	addresses := s.resolveTargetAddresses(filterAddrs)
	utxos := s.collectFilteredUTXOs(addresses, minConf, maxConf)

	return &Response{
		Jsonrpc: "2.0",
		Result:  utxos,
		ID:      req.ID,
	}
}

// validateListUnspentState checks wallet and blockchain availability.
func (s *Server) validateListUnspentState() *Response {
	if s.wallet == nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -1,
				Message: "Wallet not initialized. Start the node with wallet enabled.",
			},
		}
	}
	if s.blockchain == nil {
		return &Response{
			Jsonrpc: "2.0",
			Error: &Error{
				Code:    -32603,
				Message: "Blockchain not initialized",
			},
		}
	}
	return nil
}

// parseListUnspentParams extracts minconf, maxconf, and address filters from request params.
func (s *Server) parseListUnspentParams(rawParams json.RawMessage) (minConf, maxConf int, filterAddresses []string) {
	minConf = 1
	maxConf = 9999999

	var params []interface{}
	if err := json.Unmarshal(rawParams, &params); err != nil || len(params) == 0 {
		return minConf, maxConf, filterAddresses
	}

	if minC, ok := params[0].(float64); ok {
		minConf = int(minC)
	}
	if len(params) > 1 {
		if maxC, ok := params[1].(float64); ok {
			maxConf = int(maxC)
		}
	}
	if len(params) > 2 {
		filterAddresses = s.extractAddressFilter(params[2])
	}
	return minConf, maxConf, filterAddresses
}

// extractAddressFilter parses the address filter parameter.
func (s *Server) extractAddressFilter(param interface{}) []string {
	var addresses []string
	if addrs, ok := param.([]interface{}); ok {
		for _, a := range addrs {
			if addr, ok := a.(string); ok {
				addresses = append(addresses, addr)
			}
		}
	}
	return addresses
}

// resolveTargetAddresses returns the addresses to query (filter or all wallet addresses).
func (s *Server) resolveTargetAddresses(filterAddresses []string) []string {
	if len(filterAddresses) > 0 {
		return filterAddresses
	}
	return s.wallet.GetAddresses()
}

// collectFilteredUTXOs gathers UTXOs for addresses, applying confirmation filters.
func (s *Server) collectFilteredUTXOs(addresses []string, minConf, maxConf int) []map[string]interface{} {
	bestHeight := s.blockchain.BestSnapshot().Height
	var result []map[string]interface{}
	var errorCount int

	for _, addr := range addresses {
		utxos, err := s.blockchain.GetUTXOsForAddress(addr)
		if err != nil {
			errorCount++
			s.logWarn("failed to get UTXOs for address", "address", addr, "error", err)
			continue
		}

		for _, utxo := range utxos {
			if utxoObj := s.buildUTXOResult(utxo, bestHeight, minConf, maxConf); utxoObj != nil {
				result = append(result, utxoObj)
			}
		}
	}

	if errorCount > 0 {
		s.logWarn("listunspent returned incomplete results", "skipped_addresses", errorCount)
	}

	if result == nil {
		result = []map[string]interface{}{}
	}
	return result
}

// buildUTXOResult creates a UTXO result object if it passes confirmation filters.
func (s *Server) buildUTXOResult(utxo *namedb.UTXO, bestHeight int32, minConf, maxConf int) map[string]interface{} {
	confirmations := s.calculateConfirmations(utxo.Height, bestHeight)
	if confirmations < minConf || confirmations > maxConf {
		return nil
	}

	result := map[string]interface{}{
		"txid":          utxo.TxHash.String(),
		"vout":          utxo.OutIndex,
		"address":       utxo.Address,
		"amount":        float64(utxo.Value) / 1e8,
		"confirmations": confirmations,
		"spendable":     true,
		"solvable":      true,
		"safe":          true,
	}

	if len(utxo.PkScript) > 0 {
		result["scriptPubKey"] = hex.EncodeToString(utxo.PkScript)
	}

	return result
}

// calculateConfirmations computes confirmations for a UTXO at a given height.
func (s *Server) calculateConfirmations(utxoHeight, bestHeight int32) int {
	if utxoHeight > 0 {
		return int(bestHeight - utxoHeight + 1)
	}
	return 0
}
