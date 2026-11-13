package audit_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/propagation"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
)

func TestBaggage(t *testing.T) {
	t.Parallel()
	prop := propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)

	expected := audit.WorkspaceBuildBaggage{
		IP: "127.0.0.1",
	}

	ctx, err := audit.BaggageToContext(context.Background(), expected)
	require.NoError(t, err)

	carrier := propagation.MapCarrier{}
	prop.Inject(ctx, carrier)
	bCtx := prop.Extract(ctx, carrier)
	got := audit.BaggageFromContext(bCtx)

	require.Equal(t, expected, got)
}

func TestResourceTarget_ChatTitleNotLeaked(t *testing.T) {
	t.Parallel()

	chat := database.Chat{
		ID:    uuid.UUID{1},
		Title: "sensitive-project-name",
	}
	target := audit.ResourceTarget(chat)
	require.NotContains(t, target, chat.Title,
		"ResourceTarget for Chat must not contain the title; it should use a UUID prefix")
}

func TestChatAutomationResource(t *testing.T) {
	t.Parallel()

	automation := database.ChatAutomation{
		ID:             uuid.UUID{1},
		OrganizationID: uuid.UUID{2},
		Name:           "ci-failures",
		Prompt:         "prompt-must-not-reach-the-target",
		WebhookSecret:  sql.NullString{String: "secret-must-not-reach-the-target", Valid: true},
	}
	require.Equal(t, "ci-failures", audit.ResourceTarget(automation))
	require.Equal(t, automation.ID, audit.ResourceID(automation))
	require.Equal(t, database.ResourceTypeChatAutomation, audit.ResourceType(automation))
	require.True(t, audit.ResourceRequiresOrgID[database.ChatAutomation]())
}

func TestResourceTarget_MCPServerConfigSlugFallback(t *testing.T) {
	t.Parallel()

	config := database.MCPServerConfig{
		ID:          uuid.UUID{1},
		DisplayName: "GitHub MCP",
		Slug:        "github",
	}
	require.Equal(t, "GitHub MCP", audit.ResourceTarget(config))

	config.DisplayName = ""
	require.Equal(t, "github", audit.ResourceTarget(config))

	config.Slug = ""
	require.Equal(t, config.ID.String(), audit.ResourceTarget(config))
}
