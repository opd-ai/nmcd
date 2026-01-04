package mail

import (
	"testing"
)

// TestParseBitAddress_Success tests successful .bit address parsing
func TestParseBitAddress_Success(t *testing.T) {
	tests := []struct {
		name      string
		addr      string
		wantLocal string
	}{
		{
			name:      "simple_address",
			addr:      "alice@mail.bit",
			wantLocal: "alice",
		},
		{
			name:      "numeric_localpart",
			addr:      "user123@inbox.bit",
			wantLocal: "user123",
		},
		{
			name:      "dots_in_localpart",
			addr:      "first.last@company.bit",
			wantLocal: "first.last",
		},
		{
			name:      "subdomain",
			addr:      "bob@mail.example.bit",
			wantLocal: "bob",
		},
		{
			name:      "complex_localpart",
			addr:      "user+tag@service.bit",
			wantLocal: "user+tag",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseBitAddress(tt.addr)
			if err != nil {
				t.Errorf("parseBitAddress(%q) unexpected error: %v", tt.addr, err)
				return
			}
			if got != tt.wantLocal {
				t.Errorf("parseBitAddress(%q) = %q, want %q", tt.addr, got, tt.wantLocal)
			}
		})
	}
}

// TestParseBitAddress_Errors tests error cases in address parsing
func TestParseBitAddress_Errors(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		wantErr string
	}{
		{
			name:    "no_at_symbol",
			addr:    "alice.mail.bit",
			wantErr: "must contain exactly one @ symbol",
		},
		{
			name:    "multiple_at_symbols",
			addr:    "alice@mail@example.bit",
			wantErr: "must contain exactly one @ symbol",
		},
		{
			name:    "empty_localpart",
			addr:    "@mail.bit",
			wantErr: "localpart cannot be empty",
		},
		{
			name:    "not_bit_domain",
			addr:    "alice@gmail.com",
			wantErr: "domain must end with .bit",
		},
		{
			name:    "missing_domain",
			addr:    "alice@.bit",
			wantErr: "domain cannot be just .bit",
		},
		{
			name:    "only_at",
			addr:    "@",
			wantErr: "localpart cannot be empty",
		},
		{
			name:    "bit_suffix_not_at_end",
			addr:    "alice@bit.com",
			wantErr: "domain must end with .bit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseBitAddress(tt.addr)
			if err == nil {
				t.Errorf("parseBitAddress(%q) expected error containing %q, got nil (result: %q)", tt.addr, tt.wantErr, got)
				return
			}
			if got != "" {
				t.Errorf("parseBitAddress(%q) error case should return empty string, got %q", tt.addr, got)
			}
			// Check error message contains expected string
			errMsg := err.Error()
			if !contains(errMsg, tt.wantErr) {
				t.Errorf("parseBitAddress(%q) error = %q, want error containing %q", tt.addr, errMsg, tt.wantErr)
			}
		})
	}
}

// TestParseBitAddress_EmptyString tests edge case of empty address
func TestParseBitAddress_EmptyString(t *testing.T) {
	got, err := parseBitAddress("")
	if err == nil {
		t.Errorf("parseBitAddress(\"\") expected error, got nil (result: %q)", got)
		return
	}
	if got != "" {
		t.Errorf("parseBitAddress(\"\") error case should return empty string, got %q", got)
	}
}

// contains checks if s contains substr (helper for error message validation)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && (s[0:len(substr)] == substr || contains(s[1:], substr))))
}
