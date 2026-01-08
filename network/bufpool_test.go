package network

import (
	"bytes"
	"testing"
)

func TestBufferPool_GetPut(t *testing.T) {
	// Get a buffer from pool
	buf := GetBuffer()
	if buf == nil {
		t.Fatal("GetBuffer() returned nil")
	}

	// Verify buffer is empty
	if buf.Len() != 0 {
		t.Errorf("GetBuffer() returned non-empty buffer: len=%d", buf.Len())
	}

	// Verify buffer has capacity
	if buf.Cap() == 0 {
		t.Error("GetBuffer() returned buffer with zero capacity")
	}

	// Write some data
	testData := []byte("test data for buffer pool")
	buf.Write(testData)

	if buf.Len() != len(testData) {
		t.Errorf("buffer.Write() failed: expected len=%d, got len=%d", len(testData), buf.Len())
	}

	// Return buffer to pool
	PutBuffer(buf)
}

func TestBufferPool_Reset(t *testing.T) {
	// Get buffer and write data
	buf := GetBuffer()
	buf.Write([]byte("some data"))

	// Return to pool
	PutBuffer(buf)

	// Get again - should be reset
	buf2 := GetBuffer()
	defer PutBuffer(buf2)

	if buf2.Len() != 0 {
		t.Errorf("Buffer from pool not reset: len=%d, expected 0", buf2.Len())
	}
}

func TestBufferPool_LargeBuffer(t *testing.T) {
	// Get buffer
	buf := GetBuffer()

	// Write large amount of data (>64KB to exceed pool threshold)
	largeData := make([]byte, 128*1024) // 128KB
	buf.Write(largeData)

	if buf.Cap() < len(largeData) {
		t.Errorf("buffer capacity too small: cap=%d, data=%d", buf.Cap(), len(largeData))
	}

	// Return to pool - should not actually pool due to size
	PutBuffer(buf)

	// Get new buffer - should be default size, not the large one
	buf2 := GetBuffer()
	defer PutBuffer(buf2)

	// New buffer should have default capacity (4KB), not large capacity
	if buf2.Cap() > 10*1024 {
		t.Errorf("Pool returned large buffer: cap=%d, expected ~4KB", buf2.Cap())
	}
}

func TestBufferPool_NilBuffer(t *testing.T) {
	// PutBuffer(nil) should not panic
	PutBuffer(nil)
}

func TestBufferPool_Concurrent(t *testing.T) {
	// Test concurrent access to buffer pool
	const numGoroutines = 100
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			buf := GetBuffer()
			buf.Write([]byte("test"))
			PutBuffer(buf)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines; i++ {
		<-done
	}
}

// BenchmarkBufferPool_GetPut measures buffer pool performance
func BenchmarkBufferPool_GetPut(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf := GetBuffer()
		buf.Write([]byte("test data"))
		PutBuffer(buf)
	}
}

// BenchmarkBufferPool_NoPool measures allocation without pool
func BenchmarkBufferPool_NoPool(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf := bytes.NewBuffer(make([]byte, 0, 4096))
		buf.Write([]byte("test data"))
		// No PutBuffer - just let GC handle it
	}
}

// BenchmarkBufferPool_LargeWrite measures performance with larger writes
func BenchmarkBufferPool_LargeWrite(b *testing.B) {
	data := make([]byte, 1024) // 1KB
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		buf := GetBuffer()
		buf.Write(data)
		PutBuffer(buf)
	}
}
