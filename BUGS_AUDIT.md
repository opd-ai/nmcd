# Go Codebase Audit Report

## Executive Summary
- Total issues found: 4
- Critical: 2, High: 1, Medium: 1, Low: 0
- Files analyzed: 44

## Critical Issues
### [CRITICAL-001] RPC authentication silently disabled with partial credentials
**File:** internal/server/server.go:85-88  
**Severity:** Critical  
**Issue:** If only `-rpcuser` or `-rpcpassword` is provided, the server logs a warning but starts with authentication fully disabled, leaving administrative RPCs unauthenticated.  
**Impact:** Operators can believe RPC is protected while it is open to any local/remote caller (depending on bind address), enabling full node control without credentials.  
**Fix:** Fail startup when only one credential is provided and keep RPC disabled until both are set; alternatively default to rejecting unauthenticated requests.  
```go
	if (cfg.RPCUser != "" && cfg.RPCPassword == "") || (cfg.RPCUser == "" && cfg.RPCPassword != "") {
		log.Printf("Warning: Both -rpcuser and -rpcpassword must be set for RPC authentication. Authentication is disabled.")
	}
	// Recommended:
	// return fmt.Errorf("RPC authentication requires both -rpcuser and -rpcpassword")
```

### [CRITICAL-002] Wallet private keys persisted unencrypted on disk
**File:** wallet/wallet.go:119-132  
**Severity:** Critical  
**Issue:** Wallet keys are written directly to `wallet.json` in hex without any encryption or key stretching.  
**Impact:** Compromise of the data directory or backups immediately exposes all private keys, enabling theft of name ownership and funds.  
**Fix:** Encrypt wallet storage with a passphrase (e.g., scrypt + AEAD), or integrate OS keyring/HD seed protection; at minimum gate access with a required passphrase.  
```go
	data, err := json.MarshalIndent(wd, "", "  ")
	// ...
	if err := os.WriteFile(w.walletPath(), data, 0600); err != nil {
		return fmt.Errorf("failed to write wallet: %w", err)
	}
```

## High-Priority Bugs
### [HIGH-001] RPC transport lacks TLS, exposing credentials and traffic
**File:** rpc/server.go:83-87  
**Severity:** High  
**Issue:** The RPC server uses plain HTTP without any TLS configuration while supporting Basic Auth.  
**Impact:** When bound beyond localhost, credentials and RPC payloads travel in cleartext and can be intercepted or modified.  
**Fix:** Add TLS support (cert/key config) or refuse to start unless bound to loopback when TLS is disabled; document requirement for TLS termination.  
```go
	s.server = &http.Server{
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
```

## Medium Issues
### [MEDIUM-001] Unbounded RPC request body allows memory exhaustion
**File:** rpc/server.go:143-146  
**Severity:** Medium  
**Issue:** Requests are decoded directly from `r.Body` without size limits. A client can stream an arbitrarily large payload, tying up memory/CPU before decode fails.  
**Impact:** Remote clients can exhaust memory and degrade availability (DoS) without authentication when RPC is open.  
**Fix:** Wrap the body with `http.MaxBytesReader` (e.g., 1–2MB cap) before decoding and return `-32600` on limit violations.  
```go
	var req Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, &req, -32700, "Parse error")
		return
	}
```

## Recommendations
1. Enforce RPC authentication strictly: fail startup on partial credentials and default-deny unauthenticated requests.
2. Add TLS termination (or loopback-only restriction) for RPC traffic to prevent credential interception.
3. Introduce size limits for RPC request bodies and validate content types to harden against DoS.
4. Encrypt wallet storage with a user-supplied passphrase and document backup/rotation procedures.
