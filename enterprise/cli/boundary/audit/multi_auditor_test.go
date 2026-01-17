//nolint:paralleltest,testpackage,revive,gocritic
package audit

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

type mockAuditor struct {
	onAudit func(req Request)
}

func (m *mockAuditor) AuditRequest(req Request) {
	if m.onAudit != nil {
		m.onAudit(req)
	}
}

func TestSetupAuditor_DisabledAuditLogs(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := context.Background()

	auditor, err := SetupAuditor(ctx, logger, true, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	multi, ok := auditor.(*MultiAuditor)
	if !ok {
		t.Fatalf("expected *MultiAuditor, got %T", auditor)
	}

	if len(multi.auditors) != 1 {
		t.Errorf("expected 1 auditor, got %d", len(multi.auditors))
	}

	if _, ok := multi.auditors[0].(*LogAuditor); !ok {
		t.Errorf("expected *LogAuditor, got %T", multi.auditors[0])
	}
}

func TestSetupAuditor_EmptySocketPath(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := context.Background()

	_, err := SetupAuditor(ctx, logger, false, "")
	if err == nil {
		t.Fatal("expected error for empty socket path, got nil")
	}
}

func TestSetupAuditor_SocketDoesNotExist(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := context.Background()

	auditor, err := SetupAuditor(ctx, logger, false, "/nonexistent/socket/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	multi, ok := auditor.(*MultiAuditor)
	if !ok {
		t.Fatalf("expected *MultiAuditor, got %T", auditor)
	}

	if len(multi.auditors) != 1 {
		t.Errorf("expected 1 auditor, got %d", len(multi.auditors))
	}

	if _, ok := multi.auditors[0].(*LogAuditor); !ok {
		t.Errorf("expected *LogAuditor, got %T", multi.auditors[0])
	}
}

func TestSetupAuditor_SocketExists(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a temporary file to simulate the socket existing
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")
	f, err := os.Create(socketPath)
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	err = f.Close()
	if err != nil {
		t.Fatalf("failed to close temp file: %v", err)
	}

	auditor, err := SetupAuditor(ctx, logger, false, socketPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	multi, ok := auditor.(*MultiAuditor)
	if !ok {
		t.Fatalf("expected *MultiAuditor, got %T", auditor)
	}

	if len(multi.auditors) != 2 {
		t.Errorf("expected 2 auditors, got %d", len(multi.auditors))
	}

	if _, ok := multi.auditors[0].(*LogAuditor); !ok {
		t.Errorf("expected first auditor to be *LogAuditor, got %T", multi.auditors[0])
	}

	if _, ok := multi.auditors[1].(*SocketAuditor); !ok {
		t.Errorf("expected second auditor to be *SocketAuditor, got %T", multi.auditors[1])
	}
}

func TestMultiAuditor_AuditRequest(t *testing.T) {
	t.Parallel()

	var called1, called2 bool
	auditor1 := &mockAuditor{onAudit: func(req Request) { called1 = true }}
	auditor2 := &mockAuditor{onAudit: func(req Request) { called2 = true }}

	multi := NewMultiAuditor(auditor1, auditor2)
	multi.AuditRequest(Request{Method: "GET", URL: "https://example.com"})

	if !called1 {
		t.Error("expected first auditor to be called")
	}
	if !called2 {
		t.Error("expected second auditor to be called")
	}
}
