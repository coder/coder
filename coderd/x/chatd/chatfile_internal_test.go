package chatd

import (
	"bytes"
	"context"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
)

// inlineImageCapFor panics if the provider has no documented cap;
// tests that call this must use a capped provider.
func inlineImageCapFor(t *testing.T, provider string) int {
	t.Helper()
	imageCap, ok := chatprovider.InlineImageByteCap(provider)
	require.Truef(t, ok, "expected provider %q to have an inline image cap", provider)
	return imageCap
}

// TestChatFileResolver_RejectsOversizedImages verifies the server-side
// safety net: even if an oversized image slips past the browser's
// resize step, chatFileResolver refuses to forward it upstream. This
// prevents sending a request that the provider's upstream API would
// reject for exceeding its inline-image cap.
func TestChatFileResolver_RejectsOversizedImages(t *testing.T) {
	t.Parallel()

	// Computed rather than hardcoded so the table doesn't silently
	// rot if chatprovider ever retunes the cap.
	anthropicCap := inlineImageCapFor(t, "anthropic")

	tests := []struct {
		name             string
		provider         string
		mimetype         string
		size             int
		expectReject     bool
		expectProviderID string // classified.Provider after normalization
	}{
		{
			name:             "OversizedAnthropicPNG_Rejected",
			provider:         "anthropic",
			mimetype:         "image/png",
			size:             anthropicCap + 1,
			expectReject:     true,
			expectProviderID: "anthropic",
		},
		{
			name:             "OversizedAnthropicJPEG_Rejected",
			provider:         "anthropic",
			mimetype:         "image/jpeg",
			size:             anthropicCap + 1024,
			expectReject:     true,
			expectProviderID: "anthropic",
		},
		{
			// Server uses >= for the boundary, so exactly-at-limit
			// is also rejected. Anthropic's public docs phrase the
			// limit as "5 MB maximum" without specifying inclusivity;
			// rejecting strictly safer than letting the exact-limit
			// file fail upstream with a generic error.
			name:             "AtLimitAnthropicImage_Rejected",
			provider:         "anthropic",
			mimetype:         "image/png",
			size:             anthropicCap,
			expectReject:     true,
			expectProviderID: "anthropic",
		},
		{
			name:             "JustUnderLimitAnthropicImage_Accepted",
			provider:         "anthropic",
			mimetype:         "image/png",
			size:             anthropicCap - 1,
			expectReject:     false,
			expectProviderID: "anthropic",
		},
		{
			name:             "UndersizedAnthropicImage_Accepted",
			provider:         "anthropic",
			mimetype:         "image/png",
			size:             1024,
			expectReject:     false,
			expectProviderID: "anthropic",
		},
		{
			// Bedrock reuses Anthropic's wire format and cap (see
			// chatprovider.InlineImageByteCap); the same oversize
			// image bound for Bedrock must be rejected.
			name:             "OversizedBedrockPNG_Rejected",
			provider:         "bedrock",
			mimetype:         "image/png",
			size:             anthropicCap + 1,
			expectReject:     true,
			expectProviderID: "bedrock",
		},
		{
			name:             "OversizedOpenAIImage_Accepted",
			provider:         "openai",
			mimetype:         "image/png",
			size:             anthropicCap + 1,
			expectReject:     false,
			expectProviderID: "openai",
		},
		{
			name:             "OversizedAnthropicText_Accepted",
			provider:         "anthropic",
			mimetype:         "text/plain",
			size:             anthropicCap + 1,
			expectReject:     false,
			expectProviderID: "anthropic",
		},
		{
			name:             "ProviderMixedCase_Rejected",
			provider:         "Anthropic",
			mimetype:         "image/png",
			size:             anthropicCap + 1,
			expectReject:     true,
			expectProviderID: "anthropic",
		},
		{
			name:             "ProviderAllCaps_Rejected",
			provider:         "ANTHROPIC",
			mimetype:         "image/png",
			size:             anthropicCap + 1,
			expectReject:     true,
			expectProviderID: "anthropic",
		},
		{
			name:             "ProviderPaddedWhitespace_Rejected",
			provider:         "  anthropic  ",
			mimetype:         "image/png",
			size:             anthropicCap + 1,
			expectReject:     true,
			expectProviderID: "anthropic",
		},
	}

	for _, tc := range tests {
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
				require.Equal(t, tc.expectProviderID, classified.Provider)
				require.False(t, classified.Retryable)
				// Surface should include the provider display name
				// and the byte cap derived from the constant.
				displayName := chatprovider.ProviderDisplayName(tc.expectProviderID)
				require.Contains(t, classified.Message, displayName)
				require.Contains(
					t,
					classified.Message,
					strconv.Itoa(inlineImageCapFor(t, tc.expectProviderID)),
				)
				return
			}
			require.NoError(t, err)
			require.Contains(t, got, fileID)
			require.Equal(t, row.Data, got[fileID].Data)
			require.Equal(t, tc.mimetype, got[fileID].MediaType)
		})
	}
}

// TestChatFileResolver_MultiFileFailsFastOnFirstOversized pins the
// current "first bad file aborts the batch" contract. If this is
// ever changed to collect all violations, this test should be
// updated rather than silently dropped.
func TestChatFileResolver_MultiFileFailsFastOnFirstOversized(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	server := &Server{db: db}

	anthropicCap := inlineImageCapFor(t, "anthropic")
	okFileA := database.ChatFile{
		ID:       uuid.New(),
		Name:     "ok-a.png",
		Mimetype: "image/png",
		Data:     bytes.Repeat([]byte{0x00}, 1024),
	}
	oversized := database.ChatFile{
		ID:       uuid.New(),
		Name:     "too-big.png",
		Mimetype: "image/png",
		Data:     bytes.Repeat([]byte{0x00}, anthropicCap+1),
	}
	okFileB := database.ChatFile{
		ID:       uuid.New(),
		Name:     "ok-b.png",
		Mimetype: "image/png",
		Data:     bytes.Repeat([]byte{0x00}, 1024),
	}
	ids := []uuid.UUID{okFileA.ID, oversized.ID, okFileB.ID}

	db.EXPECT().
		GetChatFilesByIDs(gomock.Any(), ids).
		Return([]database.ChatFile{okFileA, oversized, okFileB}, nil).
		Times(1)

	resolver := server.chatFileResolver("anthropic")
	got, err := resolver(ctx, ids)
	require.Error(t, err)
	require.Nil(t, got)
	classified := chaterror.Classify(err)
	require.Equal(t, chaterror.KindConfig, classified.Kind)
	// The error must identify the specific offending file so a user
	// with several attachments knows which one to replace.
	require.Contains(t, err.Error(), oversized.Name)
}

// TestChatFileResolver_PropagatesDBError confirms unrelated database
// failures pass through unchanged (not masked by the size check).
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

// TestChatFileResolver_UnknownProviderSkipsCapCheck confirms providers
// without a documented inline cap are never rejected by the backstop.
func TestChatFileResolver_UnknownProviderSkipsCapCheck(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	server := &Server{db: db}

	fileID := uuid.New()
	row := database.ChatFile{
		ID:       fileID,
		Name:     "huge.png",
		Mimetype: "image/png",
		// Well above every documented cap; but without a matching
		// provider the backstop must not fire.
		Data: bytes.Repeat([]byte{0x00}, 50*1024*1024),
	}
	db.EXPECT().
		GetChatFilesByIDs(gomock.Any(), []uuid.UUID{fileID}).
		Return([]database.ChatFile{row}, nil).
		Times(1)

	resolver := server.chatFileResolver("openrouter")
	got, err := resolver(ctx, []uuid.UUID{fileID})
	require.NoError(t, err)
	require.Contains(t, got, fileID)
}
