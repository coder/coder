package mcpclient

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
)

func TestSummaryErrorTruncation(t *testing.T) {
	t.Parallel()

	t.Run("short errors pass through", func(t *testing.T) {
		t.Parallel()
		got := summaryError(connectionKindOrg, secretRedactor{}, xerrors.New("connect refused"))
		require.Equal(t, "connect refused", got)
	})

	t.Run("large remote-controlled errors are bounded", func(t *testing.T) {
		t.Parallel()
		huge := strings.Repeat("x", 1<<20)
		got := summaryError(connectionKindOrg, secretRedactor{}, xerrors.New(huge))
		require.Len(t, got, maxSummaryErrorLen+len("... (truncated)"))
		require.True(t, strings.HasSuffix(got, "... (truncated)"))
		require.True(t, strings.HasPrefix(got, "xxx"))
	})

	t.Run("truncation lands on a rune boundary", func(t *testing.T) {
		t.Parallel()
		// A two-byte rune straddles the cap boundary, so the cut
		// must back up instead of splitting it.
		msg := strings.Repeat("a", maxSummaryErrorLen-1) +
			strings.Repeat("é", 20)
		got := summaryError(connectionKindOrg, secretRedactor{}, xerrors.New(msg))
		require.True(t, utf8.ValidString(got))
		require.True(t, strings.HasSuffix(got, "... (truncated)"))
		require.Len(t, got, maxSummaryErrorLen-1+len("... (truncated)"))
	})
}

func TestBuildAuthHeadersSkipsFailedToken(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	cfg := Server{
		ID:       uuid.New(),
		Slug:     "revoked",
		UserAuth: UserAuthOAuth2,
	}

	t.Run("FailureReasonSet", func(t *testing.T) {
		t.Parallel()
		headers := buildAuthHeaders(
			context.Background(), logger, cfg,
			map[uuid.UUID]database.MCPServerUserToken{
				cfg.ID: {
					MCPServerConfigID:         cfg.ID,
					AccessToken:               "leftover",
					OauthRefreshFailureReason: "invalid_grant",
				},
			},
			uuid.New(), nil,
		)
		require.NotContains(t, headers, "Authorization")
	})

	t.Run("HealthyToken", func(t *testing.T) {
		t.Parallel()
		headers := buildAuthHeaders(
			context.Background(), logger, cfg,
			map[uuid.UUID]database.MCPServerUserToken{
				cfg.ID: {
					MCPServerConfigID: cfg.ID,
					AccessToken:       "valid",
					TokenType:         "Bearer",
				},
			},
			uuid.New(), nil,
		)
		require.Equal(t, "Bearer valid", headers["Authorization"])
	})
}
