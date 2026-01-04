package bridge

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/opd-ai/nmcd/client"
)

// mockNameClient implements client.NameClient for testing
type mockNameClient struct {
	names map[string]*client.NameRecord
}

func (m *mockNameClient) ResolveName(ctx context.Context, name string) (*client.NameRecord, error) {
	if record, ok := m.names[name]; ok {
		return record, nil
	}
	return nil, client.ErrNameNotFound
}

func (m *mockNameClient) RegisterName(ctx context.Context, name, value string, opts *client.RegisterOpts) (*client.TxResult, error) {
	return nil, errors.New("not implemented")
}

func (m *mockNameClient) UpdateName(ctx context.Context, name, value string, opts *client.UpdateOpts) (*client.TxResult, error) {
	return nil, errors.New("not implemented")
}

func (m *mockNameClient) ListNames(ctx context.Context, filter *client.ListFilter) ([]*client.NameRecord, error) {
	return nil, errors.New("not implemented")
}

func (m *mockNameClient) GetNameHistory(ctx context.Context, name string) ([]*client.NameRecord, error) {
	return nil, errors.New("not implemented")
}

func (m *mockNameClient) WaitForConfirmation(ctx context.Context, txHash string, confirmations int) error {
	return errors.New("not implemented")
}

func (m *mockNameClient) GetInfo(ctx context.Context) (*client.NodeInfo, error) {
	return nil, errors.New("not implemented")
}

func (m *mockNameClient) Close() error {
	return nil
}

// TestNewNamecoinBridge tests bridge construction
func TestNewNamecoinBridge(t *testing.T) {
	mock := &mockNameClient{names: make(map[string]*client.NameRecord)}
	bridge := NewNamecoinBridge(mock)

	if bridge == nil {
		t.Fatal("NewNamecoinBridge returned nil")
	}

	if bridge.nc != mock {
		t.Error("Bridge client not set correctly")
	}
}

// TestLookupMail_Success tests successful mail config lookup
func TestLookupMail_Success(t *testing.T) {
	tests := []struct {
		name      string
		nameValue string
		want      MailConfig
	}{
		{
			name: "basic_email_only",
			nameValue: `{
				"email": "user@gmail.com"
			}`,
			want: MailConfig{
				ForwardTo:   "user@gmail.com",
				BackupAddrs: nil,
				PublicKey:   nil,
			},
		},
		{
			name: "email_with_backup",
			nameValue: `{
				"email": "user@gmail.com",
				"backup": ["backup1@proton.me", "backup2@example.com"]
			}`,
			want: MailConfig{
				ForwardTo:   "user@gmail.com",
				BackupAddrs: []string{"backup1@proton.me", "backup2@example.com"},
				PublicKey:   nil,
			},
		},
		{
			name: "email_with_single_backup",
			nameValue: `{
				"email": "user@gmail.com",
				"backup": ["backup@proton.me"]
			}`,
			want: MailConfig{
				ForwardTo:   "user@gmail.com",
				BackupAddrs: []string{"backup@proton.me"},
				PublicKey:   nil,
			},
		},
		{
			name: "email_with_pubkey",
			nameValue: `{
				"email": "user@gmail.com",
				"pubkey": "dGVzdGtleQ=="
			}`,
			want: MailConfig{
				ForwardTo:   "user@gmail.com",
				BackupAddrs: nil,
				PublicKey:   []byte("testkey"),
			},
		},
		{
			name: "full_config",
			nameValue: `{
				"email": "user@gmail.com",
				"backup": ["backup1@proton.me", "backup2@example.com"],
				"pubkey": "dGVzdGtleQ=="
			}`,
			want: MailConfig{
				ForwardTo:   "user@gmail.com",
				BackupAddrs: []string{"backup1@proton.me", "backup2@example.com"},
				PublicKey:   []byte("testkey"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockNameClient{
				names: map[string]*client.NameRecord{
					"alice": {
						Name:  "alice",
						Value: tt.nameValue,
					},
				},
			}

			bridge := NewNamecoinBridge(mock)
			got, err := bridge.LookupMail("alice")

			if err != nil {
				t.Fatalf("LookupMail() error = %v, want nil", err)
			}

			if got.ForwardTo != tt.want.ForwardTo {
				t.Errorf("ForwardTo = %q, want %q", got.ForwardTo, tt.want.ForwardTo)
			}

			if len(got.BackupAddrs) != len(tt.want.BackupAddrs) {
				t.Errorf("BackupAddrs length = %d, want %d", len(got.BackupAddrs), len(tt.want.BackupAddrs))
			} else {
				for i := range got.BackupAddrs {
					if got.BackupAddrs[i] != tt.want.BackupAddrs[i] {
						t.Errorf("BackupAddrs[%d] = %q, want %q", i, got.BackupAddrs[i], tt.want.BackupAddrs[i])
					}
				}
			}

			if len(got.PublicKey) != len(tt.want.PublicKey) {
				t.Errorf("PublicKey length = %d, want %d", len(got.PublicKey), len(tt.want.PublicKey))
			} else if len(got.PublicKey) > 0 {
				for i := range got.PublicKey {
					if got.PublicKey[i] != tt.want.PublicKey[i] {
						t.Errorf("PublicKey[%d] = %v, want %v", i, got.PublicKey[i], tt.want.PublicKey[i])
					}
				}
			}
		})
	}
}

// TestLookupMail_NameNotFound tests handling of non-existent names
func TestLookupMail_NameNotFound(t *testing.T) {
	mock := &mockNameClient{names: make(map[string]*client.NameRecord)}
	bridge := NewNamecoinBridge(mock)

	_, err := bridge.LookupMail("nonexistent")

	if !errors.Is(err, client.ErrNameNotFound) {
		t.Errorf("LookupMail() error = %v, want ErrNameNotFound", err)
	}
}

// TestLookupMail_InvalidJSON tests handling of invalid JSON in name values
func TestLookupMail_InvalidJSON(t *testing.T) {
	tests := []struct {
		name      string
		nameValue string
		wantErr   error
	}{
		{
			name:      "not_json",
			nameValue: "not json at all",
			wantErr:   ErrInvalidMailConfig,
		},
		{
			name:      "missing_email_field",
			nameValue: `{"backup": ["test@example.com"]}`,
			wantErr:   ErrInvalidMailConfig,
		},
		{
			name:      "empty_email",
			nameValue: `{"email": ""}`,
			wantErr:   ErrInvalidMailConfig,
		},
		{
			name:      "invalid_base64_pubkey",
			nameValue: `{"email": "user@gmail.com", "pubkey": "not-valid-base64!!!"}`,
			wantErr:   ErrInvalidMailConfig,
		},
		{
			name:      "malformed_json",
			nameValue: `{"email": "user@gmail.com"`,
			wantErr:   ErrInvalidMailConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockNameClient{
				names: map[string]*client.NameRecord{
					"test": {
						Name:  "test",
						Value: tt.nameValue,
					},
				},
			}

			bridge := NewNamecoinBridge(mock)
			_, err := bridge.LookupMail("test")

			if err == nil {
				t.Fatal("LookupMail() error = nil, want error")
			}

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("LookupMail() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

// TestParseMailConfig tests the parseMailConfig helper function
func TestParseMailConfig(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    MailConfig
		wantErr bool
	}{
		{
			name:  "minimal_valid_config",
			value: `{"email": "user@example.com"}`,
			want: MailConfig{
				ForwardTo:   "user@example.com",
				BackupAddrs: nil,
				PublicKey:   nil,
			},
			wantErr: false,
		},
		{
			name:  "config_with_empty_backup_array",
			value: `{"email": "user@example.com", "backup": []}`,
			want: MailConfig{
				ForwardTo:   "user@example.com",
				BackupAddrs: []string{},
				PublicKey:   nil,
			},
			wantErr: false,
		},
		{
			name:  "config_with_multiple_backups",
			value: `{"email": "user@example.com", "backup": ["b1@example.com", "b2@example.com", "b3@example.com"]}`,
			want: MailConfig{
				ForwardTo:   "user@example.com",
				BackupAddrs: []string{"b1@example.com", "b2@example.com", "b3@example.com"},
				PublicKey:   nil,
			},
			wantErr: false,
		},
		{
			name:    "invalid_json",
			value:   `{invalid}`,
			want:    MailConfig{},
			wantErr: true,
		},
		{
			name:    "missing_email",
			value:   `{"backup": ["test@example.com"]}`,
			want:    MailConfig{},
			wantErr: true,
		},
		{
			name:    "empty_string",
			value:   ``,
			want:    MailConfig{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseMailConfig(tt.value)

			if tt.wantErr {
				if err == nil {
					t.Error("parseMailConfig() error = nil, want error")
				}
				return
			}

			if err != nil {
				t.Fatalf("parseMailConfig() error = %v, want nil", err)
			}

			if got.ForwardTo != tt.want.ForwardTo {
				t.Errorf("ForwardTo = %q, want %q", got.ForwardTo, tt.want.ForwardTo)
			}

			if len(got.BackupAddrs) != len(tt.want.BackupAddrs) {
				t.Errorf("BackupAddrs length = %d, want %d", len(got.BackupAddrs), len(tt.want.BackupAddrs))
			}

			if len(got.PublicKey) != len(tt.want.PublicKey) {
				t.Errorf("PublicKey length = %d, want %d", len(got.PublicKey), len(tt.want.PublicKey))
			}
		})
	}
}

// TestParseMailConfig_PublicKeyDecoding tests base64 decoding of public keys
func TestParseMailConfig_PublicKeyDecoding(t *testing.T) {
	testKey := []byte("test public key 123")
	encodedKey := base64.StdEncoding.EncodeToString(testKey)

	value := `{"email": "user@example.com", "pubkey": "` + encodedKey + `"}`

	config, err := parseMailConfig(value)
	if err != nil {
		t.Fatalf("parseMailConfig() error = %v, want nil", err)
	}

	if len(config.PublicKey) != len(testKey) {
		t.Fatalf("PublicKey length = %d, want %d", len(config.PublicKey), len(testKey))
	}

	for i := range testKey {
		if config.PublicKey[i] != testKey[i] {
			t.Errorf("PublicKey[%d] = %v, want %v", i, config.PublicKey[i], testKey[i])
		}
	}
}

// TestLookupMail_ThreadSafety tests concurrent access to bridge
func TestLookupMail_ThreadSafety(t *testing.T) {
	mock := &mockNameClient{
		names: map[string]*client.NameRecord{
			"alice": {
				Name:  "alice",
				Value: `{"email": "alice@example.com"}`,
			},
			"bob": {
				Name:  "bob",
				Value: `{"email": "bob@example.com"}`,
			},
		},
	}

	bridge := NewNamecoinBridge(mock)

	// Run 100 concurrent lookups
	done := make(chan bool, 100)
	for i := 0; i < 100; i++ {
		go func(id int) {
			name := "alice"
			if id%2 == 0 {
				name = "bob"
			}

			_, err := bridge.LookupMail(name)
			if err != nil {
				t.Errorf("Concurrent lookup %d failed: %v", id, err)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 100; i++ {
		<-done
	}
}
