package namedb

import (
	"testing"
)

// TestBufferPool_GetPut verifies basic buffer pool operations
func TestBufferPool_GetPut(t *testing.T) {
	// Get a buffer
	buf := getBuffer()
	if buf == nil {
		t.Fatal("getBuffer() returned nil")
	}
	if buf.Len() != 0 {
		t.Errorf("Expected empty buffer, got len=%d", buf.Len())
	}

	// Write some data
	testData := []byte("test data")
	buf.Write(testData)
	if buf.Len() != len(testData) {
		t.Errorf("Expected len=%d, got %d", len(testData), buf.Len())
	}

	// Return to pool
	putBuffer(buf)

	// Get another buffer - should be reset
	buf2 := getBuffer()
	if buf2.Len() != 0 {
		t.Errorf("Expected reset buffer, got len=%d", buf2.Len())
	}
	putBuffer(buf2)
}

// TestBufferPool_LargeBuffer verifies that oversized buffers are not pooled
func TestBufferPool_LargeBuffer(t *testing.T) {
	buf := getBuffer()

	// Grow buffer beyond threshold
	largeData := make([]byte, 100000) // 100KB, above 64KB threshold
	buf.Write(largeData)

	if buf.Cap() <= 65536 {
		t.Fatalf("Test setup error: buffer didn't grow as expected, cap=%d", buf.Cap())
	}

	// Return to pool - should not be pooled
	putBuffer(buf)

	// Get a new buffer - should be a fresh one, not the large one
	buf2 := getBuffer()
	if buf2.Cap() > 65536 {
		t.Errorf("Large buffer was pooled incorrectly, cap=%d", buf2.Cap())
	}
	putBuffer(buf2)
}

// TestBufferPool_Reset verifies buffers are reset when retrieved
func TestBufferPool_Reset(t *testing.T) {
	buf := getBuffer()
	buf.WriteString("previous data")
	putBuffer(buf)

	// Should get a reset buffer
	buf2 := getBuffer()
	if buf2.Len() != 0 {
		t.Errorf("Buffer not reset: len=%d", buf2.Len())
	}
	if string(buf2.Bytes()) != "" {
		t.Errorf("Buffer contains old data: %q", buf2.Bytes())
	}
	putBuffer(buf2)
}

// BenchmarkBufferPool_GetPut measures pool overhead
func BenchmarkBufferPool_GetPut(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf := getBuffer()
		putBuffer(buf)
	}
}

// BenchmarkBufferPool_Write measures write performance with pooling
func BenchmarkBufferPool_Write(b *testing.B) {
	b.ReportAllocs()
	data := []byte("test data for benchmarking")

	for i := 0; i < b.N; i++ {
		buf := getBuffer()
		buf.Write(data)
		putBuffer(buf)
	}
}

// BenchmarkBufferPool_NoPool compares against non-pooled allocation
func BenchmarkBufferPool_NoPool(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf := make([]byte, 0, 2048)
		_ = buf
	}
}
