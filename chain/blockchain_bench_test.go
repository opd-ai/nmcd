package chain

import (
	"crypto/sha256"
	"fmt"
	"testing"
)

// BenchmarkParseNameScript measures script parsing performance.
// Target: < 100μs per script
func BenchmarkParseNameScript(b *testing.B) {
	scripts := []struct {
		name   string
		script []byte
	}{
		{
			name:   "NAME_NEW",
			script: buildNameNewScript(make([]byte, 20)),
		},
		{
			name:   "NAME_FIRSTUPDATE",
			script: buildNameFirstUpdateScript([]byte("d/test"), make([]byte, 20), []byte(`{"ip":"1.2.3.4"}`)),
		},
		{
			name:   "NAME_UPDATE",
			script: buildNameUpdateScript([]byte("d/test"), []byte(`{"ip":"1.2.3.4"}`)),
		},
	}

	for _, tc := range scripts {
		b.Run(tc.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _, _, err := parseNameScript(tc.script)
				if err != nil {
					b.Fatalf("parseNameScript failed: %v", err)
				}
			}
		})
	}
}

// BenchmarkComputeCommitmentHash measures commitment hash computation.
// Target: < 50μs per hash (double SHA256 + RIPEMD160)
func BenchmarkComputeCommitmentHash(b *testing.B) {
	name := []byte("d/testname")
	rand := make([]byte, 20)
	chainID := []byte{0x01, 0x00} // Namecoin chain ID

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Compute: RIPEMD160(SHA256(rand + name + chainID))
		data := append(rand, name...)
		data = append(data, chainID...)

		hash1 := sha256.Sum256(data)
		_ = hash1 // In real code, this would go to RIPEMD160
	}
}

// BenchmarkValidateNameFormat measures name format validation performance.
// Target: < 10μs per validation
func BenchmarkValidateNameFormat(b *testing.B) {
	names := []string{
		"d/example",
		"id/johndoe",
		"p/mydata",
		"d/very-long-name-with-many-characters-for-testing",
	}

	values := []string{
		`{"ip":"1.2.3.4"}`,
		`{"name":"John"}`,
		`personal data`,
		`{"dns":{"ns":["ns1.example.com"]}}`,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		name := names[i%len(names)]
		value := values[i%len(values)]
		if err := validateNameFormat(name, value); err != nil {
			b.Fatalf("validateNameFormat failed for benchmark input (name=%q, value=%q): %v", name, value, err)
		}
	}
}

// BenchmarkBuildNameScript measures script building performance.
// Target: < 50μs per script
func BenchmarkBuildNameScript(b *testing.B) {
	name := []byte("d/test")
	value := []byte(`{"ip":"1.2.3.4"}`)
	rand := make([]byte, 20)

	b.Run("NAME_NEW", func(b *testing.B) {
		b.ReportAllocs()
		hash := make([]byte, 20)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = buildNameNewScript(hash)
		}
	})

	b.Run("NAME_FIRSTUPDATE", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = buildNameFirstUpdateScript(name, rand, value)
		}
	})

	b.Run("NAME_UPDATE", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = buildNameUpdateScript(name, value)
		}
	})
}

// BenchmarkPushData measures push data encoding performance.
// Target: < 5μs per encode
func BenchmarkPushData(b *testing.B) {
	data := [][]byte{
		[]byte("short"),   // <= 75 bytes
		make([]byte, 100), // 76-255 bytes
		make([]byte, 300), // 256+ bytes
		[]byte(`{"ip":"1.2.3.4","ns":["ns1","ns2"]}`), // Typical value
	}

	for i, d := range data {
		b.Run(fmt.Sprintf("Size_%d", len(d)), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for j := 0; j < b.N; j++ {
				_ = pushData(d)
			}
		})
		_ = i
	}
}

// BenchmarkMemoryUsage measures memory allocation patterns for script operations.
func BenchmarkMemoryUsage(b *testing.B) {
	b.ReportAllocs()

	script := buildNameUpdateScript([]byte("d/test"), []byte(`{"ip":"1.2.3.4"}`))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _, _ = parseNameScript(script)
	}
}
