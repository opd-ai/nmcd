package network

import (
	"bytes"
	"sync"
)

// bufferPool is a pool of reusable byte buffers for message serialization.
// Using sync.Pool reduces GC pressure by reusing temporary buffers instead of
// allocating new ones for each message. This is particularly effective for
// wire protocol messages which are frequently serialized/deserialized.
//
// The pool automatically grows and shrinks based on demand, and the garbage
// collector can reclaim unused buffers during GC cycles.
var bufferPool = sync.Pool{
	New: func() interface{} {
		// Default buffer size of 4KB is chosen based on typical wire message sizes:
		// - MsgVersion: ~100 bytes
		// - MsgTx: 200-500 bytes (typical)
		// - MsgBlock header: 80 bytes
		// - MsgInv: varies, but often < 1KB
		// 4KB provides headroom without excessive memory waste
		return bytes.NewBuffer(make([]byte, 0, 4096))
	},
}

// GetBuffer retrieves a buffer from the pool.
// The buffer is reset to empty but retains its underlying capacity.
//
// Usage:
//
//	buf := GetBuffer()
//	defer PutBuffer(buf)
//	// Use buf for serialization...
func GetBuffer() *bytes.Buffer {
	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset() // Clear any existing data while preserving capacity
	return buf
}

// PutBuffer returns a buffer to the pool for reuse.
// The buffer should not be used after calling PutBuffer.
//
// Note: Extremely large buffers (>64KB) are not returned to the pool
// to prevent memory bloat. This threshold is chosen because:
// - Most wire messages are < 4KB (headers, invs, version, etc.)
// - Even large transactions are typically < 10KB
// - Block messages use specialized handling
// - 64KB is a reasonable upper bound for pool reuse
func PutBuffer(buf *bytes.Buffer) {
	if buf == nil {
		return
	}

	// Don't pool extremely large buffers to prevent memory bloat
	const maxPooledSize = 64 * 1024 // 64KB
	if buf.Cap() > maxPooledSize {
		return
	}

	bufferPool.Put(buf)
}
