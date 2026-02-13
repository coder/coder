package fido2

import (
	"errors"
	"fmt"
	"testing"

	gofido2 "github.com/coder/coder/v2/cli/fido2/internal/fido2"
)

func TestClassifyError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		err    error
		expect error
	}{
		{"nil", nil, nil},

		// Sentinel from vendored library.
		{"ErrPinUvAuthTokenRequired direct", gofido2.ErrPinUvAuthTokenRequired, ErrPinRequired},
		{"ErrPinUvAuthTokenRequired wrapped", fmt.Errorf("device: %w", gofido2.ErrPinUvAuthTokenRequired), ErrPinRequired},

		// String-matched PIN patterns.
		{"PIN_REQUIRED string", errors.New("ctap2: PIN_REQUIRED"), ErrPinRequired},
		{"PIN_INVALID string", errors.New("ctap2: PIN_INVALID"), ErrPinRequired},
		{"pin required string", errors.New("pin required"), ErrPinRequired},
		{"pinUvAuthToken required string", errors.New("pinUvAuthToken required"), ErrPinRequired},

		// String-matched timeout patterns.
		{"timed out", errors.New("operation timed out"), ErrTouchTimeout},
		{"OPERATION_DENIED", errors.New("ctap2: OPERATION_DENIED"), ErrTouchTimeout},
		{"KEEPALIVE_CANCEL", errors.New("KEEPALIVE_CANCEL"), ErrTouchTimeout},
		{"ACTION_TIMEOUT", errors.New("ACTION_TIMEOUT"), ErrTouchTimeout},
		{"USER_ACTION_TIMEOUT", errors.New("USER_ACTION_TIMEOUT"), ErrTouchTimeout},
		{"operation denied", errors.New("operation denied by user"), ErrTouchTimeout},

		// Unknown errors pass through.
		{"unknown error", errors.New("something else"), nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := classifyError(tt.err)
			if tt.expect == nil {
				// For nil input, expect nil output.
				// For unknown errors, expect the original error back.
				if tt.err == nil && got != nil {
					t.Fatalf("expected nil, got %v", got)
				}
				if tt.err != nil && got != tt.err {
					t.Fatalf("expected original error, got %v", got)
				}
				return
			}
			if !errors.Is(got, tt.expect) {
				t.Fatalf("expected %v, got %v", tt.expect, got)
			}
		})
	}
}
