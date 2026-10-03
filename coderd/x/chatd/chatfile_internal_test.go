package chatd

import (
	"context"
	"strconv"
	"testing"

	"github.com/dustin/go-humanize"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/codersdk"
)

// inlineImageCapFor returns the provider's inline image cap. Fails
// the test if the provider has no documented cap.
func inlineImageCapFor(t *testing.T, provider string) int {
	t.Helper()
	imageCap, ok := chatprovider.InlineImageCapBytes(provider)
	require.Truef(t, ok, "expected provider %q to have an inline image cap", provider)
	return imageCap
}

// TestChatFileResolver_RejectsOversizedImages is the server-side
// safety net for browser-side resize: oversize images that reach the
// resolver are rejected before any upstream request.
func TestChatFileResolver_RejectsOversizedImages(t *testing.T) {
	t.Parallel()

	// Computed so the table tracks any future cap retune.
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
			// Boundary is >=: exactly-at-limit is rejected.
			// Anthropic's docs say "5 MB maximum" without
			// specifying inclusivity, so reject strictly.
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
			// Bedrock reuses Anthropic's cap.
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

	// One shared backing buffer sliced per case. The resolver only
	// reads len(f.Data), so shared backing is safe and avoids N×max
	// allocations in parallel.
	maxSize := 0
	for _, tc := range tests {
		if tc.size > maxSize {
			maxSize = tc.size
		}
	}
	sharedData := make([]byte, maxSize)

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
				Data:     sharedData[:tc.size],
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
				// Classification turns the generic upstream error
				// into an actionable user-facing message.
				classified := chaterror.Classify(err)
				require.Equal(t, codersdk.ChatErrorKindConfig, classified.Kind)
				require.Equal(t, tc.expectProviderID, classified.Provider)
				require.False(t, classified.Retryable)
				// User-facing message names the provider and shows
				// the cap in human units; raw byte count stays in
				// the wrapped developer error.
				displayName := chatprovider.ProviderDisplayName(tc.expectProviderID)
				require.Contains(t, classified.Message, displayName)
				imageCap := inlineImageCapFor(t, tc.expectProviderID)
				//nolint:gosec // imageCap is a small positive constant defined in chatprovider.
				require.Contains(t, classified.Message, humanize.IBytes(uint64(imageCap)))
				require.NotContains(
					t,
					classified.Message,
					strconv.Itoa(imageCap),
					"user-facing message should not include raw bytes",
				)
				// Wrapped error preserves exact bytes for logs.
				require.Contains(t, err.Error(), strconv.Itoa(imageCap))
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
// "first bad file aborts the batch" contract.
func TestChatFileResolver_MultiFileFailsFastOnFirstOversized(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	server := &Server{db: db}

	anthropicCap := inlineImageCapFor(t, "anthropic")
	// Shared buffer; ok files take small prefixes.
	buf := make([]byte, anthropicCap+1)
	okFileA := database.ChatFile{
		ID:       uuid.New(),
		Name:     "ok-a.png",
		Mimetype: "image/png",
		Data:     buf[:1024],
	}
	oversized := database.ChatFile{
		ID:       uuid.New(),
		Name:     "too-big.png",
		Mimetype: "image/png",
		Data:     buf,
	}
	okFileB := database.ChatFile{
		ID:       uuid.New(),
		Name:     "ok-b.png",
		Mimetype: "image/png",
		Data:     buf[:1024],
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
	require.Equal(t, codersdk.ChatErrorKindConfig, classified.Kind)
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
	// Exactly 1 byte above the Anthropic cap is enough to prove
	// the backstop is skipped for uncapped providers; no need to
	// allocate tens of MiB in CI.
	overAnyCap := inlineImageCapFor(t, "anthropic") + 1
	row := database.ChatFile{
		ID:       fileID,
		Name:     "huge.png",
		Mimetype: "image/png",
		Data:     make([]byte, overAnyCap),
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
