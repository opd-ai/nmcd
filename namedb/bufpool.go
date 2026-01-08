package namedb

import (
	"bytes"
	"sync"
)

// bufferPool is a sync.Pool for reusing byte buffers during serialization operations.
// This reduces memory allocations by reusing buffers instead of creating new ones
// for each encoding/decoding operation.
var bufferPool = sync.Pool{
	New: func() interface{} {
		// Pre-allocate buffers with a reasonable size for typical name records
		// (name + value + metadata typically < 2KB)
		return bytes.NewBuffer(make([]byte, 0, 2048))
	},
}

// getBuffer retrieves a buffer from the pool
func getBuffer() *bytes.Buffer {
	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset() // Clear any previous content
	return buf
}

// putBuffer returns a buffer to the pool for reuse
func putBuffer(buf *bytes.Buffer) {
	// Don't pool buffers that have grown too large
	// This prevents memory bloat from outlier large values
	if buf.Cap() > 65536 { // 64KB threshold
		return
	}
	bufferPool.Put(buf)
}
