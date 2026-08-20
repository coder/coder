package agentssh

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
)

func TestParseSCPCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		cmd    []string
		wantOK bool
		want   FileTransferOperation
	}{
		{
			name:   "Upload",
			cmd:    []string{"scp", "-t", "/tmp"},
			wantOK: true,
			want: FileTransferOperation{
				Protocol: FileTransferProtocolSCP,
				Action:   FileTransferActionUpload,
				Path:     "/tmp",
			},
		},
		{
			name:   "Download",
			cmd:    []string{"scp", "-f", "secrets.txt"},
			wantOK: true,
			want: FileTransferOperation{
				Protocol: FileTransferProtocolSCP,
				Action:   FileTransferActionDownload,
				Path:     "secrets.txt",
			},
		},
		{
			name:   "CombinedFlags",
			cmd:    []string{"scp", "-qrt", "/home/coder"},
			wantOK: true,
			want: FileTransferOperation{
				Protocol: FileTransferProtocolSCP,
				Action:   FileTransferActionUpload,
				Path:     "/home/coder",
			},
		},
		{
			name:   "SeparateFlags",
			cmd:    []string{"scp", "-v", "-f", "-p", "file with spaces"},
			wantOK: true,
			want: FileTransferOperation{
				Protocol: FileTransferProtocolSCP,
				Action:   FileTransferActionDownload,
				Path:     "file with spaces",
			},
		},
		{
			name:   "DoubleDash",
			cmd:    []string{"scp", "-t", "--", "-weird-path"},
			wantOK: true,
			want: FileTransferOperation{
				Protocol: FileTransferProtocolSCP,
				Action:   FileTransferActionUpload,
				Path:     "-weird-path",
			},
		},
		{
			name:   "NoDirection",
			cmd:    []string{"scp", "/tmp"},
			wantOK: false,
		},
		{
			name:   "NoPath",
			cmd:    []string{"scp", "-t"},
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			op, ok := parseSCPCommand(tt.cmd)
			require.Equal(t, tt.wantOK, ok)
			if ok {
				assert.Equal(t, tt.want, op)
			}
		})
	}
}

func TestParseRsyncCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		cmd    []string
		wantOK bool
		want   FileTransferOperation
	}{
		{
			name:   "Download",
			cmd:    []string{"rsync", "--server", "--sender", "-vlogDtpre.iLsfxCIvu", ".", "/home/coder/project"},
			wantOK: true,
			want: FileTransferOperation{
				Protocol: FileTransferProtocolRsync,
				Action:   FileTransferActionDownload,
				Path:     "/home/coder/project",
			},
		},
		{
			name:   "Upload",
			cmd:    []string{"rsync", "--server", "-vlogDtpre.iLsfxCIvu", ".", "/tmp/dest"},
			wantOK: true,
			want: FileTransferOperation{
				Protocol: FileTransferProtocolRsync,
				Action:   FileTransferActionUpload,
				Path:     "/tmp/dest",
			},
		},
		{
			name:   "NotServer",
			cmd:    []string{"rsync", "-av", "src", "dst"},
			wantOK: false,
		},
		{
			name:   "NoPath",
			cmd:    []string{"rsync", "--server", "."},
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			op, ok := parseRsyncCommand(tt.cmd)
			require.Equal(t, tt.wantOK, ok)
			if ok {
				assert.Equal(t, tt.want, op)
			}
		})
	}
}

func TestFileTransferOpEmitter(t *testing.T) {
	t.Parallel()

	t.Run("Dedupe", func(t *testing.T) {
		t.Parallel()
		var got []FileTransferOperation
		emitter := newFileTransferOpEmitter(slog.Logger{}, uuid.New(), func(_ uuid.UUID, op FileTransferOperation) {
			got = append(got, op)
		})
		op := FileTransferOperation{
			Protocol: FileTransferProtocolSFTP,
			Action:   FileTransferActionDownload,
			Path:     "/tmp/file",
		}
		emitter.Emit(op)
		emitter.Emit(op)
		emitter.Emit(op)
		require.Len(t, got, 1)

		// A different action on the same path is a distinct operation.
		op.Action = FileTransferActionUpload
		emitter.Emit(op)
		require.Len(t, got, 2)
	})

	t.Run("Cap", func(t *testing.T) {
		t.Parallel()
		count := 0
		emitter := newFileTransferOpEmitter(slog.Logger{}, uuid.New(), func(uuid.UUID, FileTransferOperation) {
			count++
		})
		for i := range maxFileTransferOpsPerSession + 100 {
			emitter.Emit(FileTransferOperation{
				Protocol: FileTransferProtocolSFTP,
				Action:   FileTransferActionDownload,
				Path:     "/tmp/file" + string(rune('a'+i%26)) + string(rune('a'+(i/26)%26)) + string(rune('a'+(i/676)%26)),
			})
		}
		require.Equal(t, maxFileTransferOpsPerSession, count)
	})
}
