package systemaccounttokenprovider

import (
	"errors"
	"testing"
	"time"
)

const (
	testSecretKey      = "test_secret_key"
	testExpirationTime = 3600
)

func TestCreateSystemAccountJWTToken(t *testing.T) {
	clock := func() time.Time { return time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC) }
	provider := NewSystemAccountTokenProvider(testSecretKey, testExpirationTime, clock)

	token, err := provider.CreateSystemAccountJWTToken("test_sysacct_id")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if len(token) == 0 {
		t.Errorf("expected non-empty token, but got empty")
	}

	// Ensure that the token has the correct prefix
	if prefix := token[:len(SystemAccountTokenPrefix)]; prefix != SystemAccountTokenPrefix {
		t.Errorf("expected token prefix %q, but got %q", SystemAccountTokenPrefix, prefix)
	}
}

func TestValidateSystemAccountJWTToken(t *testing.T) {
	clock := func() time.Time { return time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC) }
	provider := NewSystemAccountTokenProvider(testSecretKey, testExpirationTime, clock)

	token, _ := provider.CreateSystemAccountJWTToken("test_sysacct_id")

	sysacctID, err := provider.ValidateSystemAccountJWTToken(token)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if sysacctID != "test_sysacct_id" {
		t.Errorf("expected system account ID %q, but got %q", "test_sysacct_id", sysacctID)
	}

	// Test invalid token
	_, err = provider.ValidateSystemAccountJWTToken("invalid_token")
	if !errors.Is(err, ErrInvalidSystemAccountToken) {
		t.Errorf("expected ErrInvalidSystemAccountToken, but got %v", err)
	}

	// Test expired token
	expiredToken, _ := provider.CreateSystemAccountJWTToken("test_sysacct_id")
	clock = func() time.Time { return time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC) }
	provider = NewSystemAccountTokenProvider(testSecretKey, -1, clock)
	_, err = provider.ValidateSystemAccountJWTToken(expiredToken)
	if !errors.Is(err, ErrExpiredSystemAccountToken) {
		t.Errorf("expected ErrExpiredSystemAccountToken, but got %v", err)
	}
}
