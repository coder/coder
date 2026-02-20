package ctap2

import (
	"testing"
)

func TestParseGetAssertionAuthDataTooShort(t *testing.T) {
	if _, err := ParseGetAssertionAuthData([]byte{0x00, 0x01}); err == nil {
		t.Fatalf("expected error for short authData")
	}
}

func TestParseMakeCredentialAuthDataTooShort(t *testing.T) {
	if _, err := ParseMakeCredentialAuthData([]byte{0x00, 0x01, 0x02}); err == nil {
		t.Fatalf("expected error for short authData")
	}
}
