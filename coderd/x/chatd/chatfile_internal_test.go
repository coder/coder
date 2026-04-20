package chatd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
)

// TestChatFileResolver_RejectsOversizedAnthropicImages verifies the
// server-side safety net: even if an oversized image slips past the
// browser's resize step, chatFileResolver refuses to forward it to
// Anthropic. This prevents sending a request that Anthropic will
// reject for exceeding its 5 MiB inline-image cap.
func TestChatFileResolver_RejectsOversizedAnthropicImages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		provider     string
		mimetype     string
		size         int
		expectReject bool
	}{
		{
			name:         "OversizedAnthropicPNG_Rejected",
			provider:     "anthropic",
			mimetype:     "image/png",
			size:         anthropicMaxInlineImageBytes + 1,
			expectReject: true,
		},
		{
			name:         "OversizedAnthropicJPEG_Rejected",
			provider:     "anthropic",
			mimetype:     "image/jpeg",
			size:         anthropicMaxInlineImageBytes + 1024,
			expectReject: true,
		},
		{
			name:         "AtLimitAnthropicImage_Accepted",
			provider:     "anthropic",
			mimetype:     "image/png",
			size:         anthropicMaxInlineImageBytes,
			expectReject: false,
		},
		{
			name:         "UndersizedAnthropicImage_Accepted",
			provider:     "anthropic",
			mimetype:     "image/png",
			size:         1024,
			expectReject: false,
		},
		{
			name:         "OversizedOpenAIImage_Accepted",
			provider:     "openai",
			mimetype:     "image/png",
			size:         anthropicMaxInlineImageBytes + 1,
			expectReject: false,
		},
		{
			name:         "OversizedAnthropicText_Accepted",
			provider:     "anthropic",
			mimetype:     "text/plain",
			size:         anthropicMaxInlineImageBytes + 1,
			expectReject: false,
		},
		{
			name:         "ProviderCaseInsensitive_Rejected",
			provider:     "Anthropic",
			mimetype:     "image/png",
			size:         anthropicMaxInlineImageBytes + 1,
			expectReject: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()

			ctrl := gomock.NewController(t)
			db := dbmock.NewMockStore(ctrl)
			server := &Server{db: db}

			fileID := uuid.New()
			row := database.ChatFile{
				ID:       fileID,
				Name:     "attachment.png",
				Mimetype: tc.mimetype,
				Data:     bytes.Repeat([]byte{0x00}, tc.size),
			}
			db.EXPECT().
				GetChatFilesByIDs(gomock.Any(), []uuid.UUID{fileID}).
				Return([]database.ChatFile{row}, nil).
				Times(1)

			resolver := server.chatFileResolver(tc.provider)
			got, err := resolver(ctx, []uuid.UUID{fileID})

			if tc.expectReject {
				require.Error(t, err)
				require.Nil(t, got)
				// The error must carry a pre-built classification so
				// the user sees an actionable message instead of a
				// generic provider failure.
				classified := chaterror.Classify(err)
				require.Equal(t, chaterror.KindConfig, classified.Kind)
				require.Equal(t, "anthropic", classified.Provider)
				require.False(t, classified.Retryable)
				require.Contains(t, strings.ToLower(classified.Message), "anthropic")
				require.Contains(t, classified.Message, "5242880")
				return
			}
			require.NoError(t, err)
			require.Contains(t, got, fileID)
			require.Equal(t, row.Data, got[fileID].Data)
			require.Equal(t, tc.mimetype, got[fileID].MediaType)
		})
	}
}

// TestChatFileResolver_PropagatesDBError confirms unrelated database
// failures pass through unchanged (not masked by the Anthropic check).
func TestChatFileResolver_PropagatesDBError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	server := &Server{db: db}

	sentinel := xerrors.New("boom")
	fileID := uuid.New()
	db.EXPECT().
		GetChatFilesByIDs(gomock.Any(), []uuid.UUID{fileID}).
		Return(nil, sentinel).
		Times(1)

	resolver := server.chatFileResolver("anthropic")
	got, err := resolver(ctx, []uuid.UUID{fileID})
	require.ErrorIs(t, err, sentinel)
	require.Nil(t, got)
}
