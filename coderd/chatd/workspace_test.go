package chatd

import (
	"context"
	"fmt"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

type templateSelectionModel struct {
	generateCall   *fantasy.Call
	generateBlocks []fantasy.Content
}

func (*templateSelectionModel) Provider() string {
	return "fake"
}

func (*templateSelectionModel) Model() string {
	return "fake"
}

func (m *templateSelectionModel) Generate(_ context.Context, call fantasy.Call) (*fantasy.Response, error) {
	captured := call
	m.generateCall = &captured
	return &fantasy.Response{Content: m.generateBlocks}, nil
}

func (*templateSelectionModel) Stream(context.Context, fantasy.Call) (fantasy.StreamResponse, error) {
	return nil, xerrors.New("not implemented")
}

func (*templateSelectionModel) GenerateObject(context.Context, fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
	return nil, xerrors.New("not implemented")
}

func (*templateSelectionModel) StreamObject(context.Context, fantasy.ObjectCall) (fantasy.ObjectStreamResponse, error) {
	return nil, xerrors.New("not implemented")
}

func TestSelectTemplateWithModel_SetsToolChoiceNone(t *testing.T) {
	t.Parallel()

	candidateID := uuid.New()
	candidates := []database.Template{
		{
			ID:          candidateID,
			Name:        "starter",
			DisplayName: "Starter",
			Description: "Starter template",
		},
	}

	model := &templateSelectionModel{
		generateBlocks: []fantasy.Content{
			fantasy.TextContent{
				Text: fmt.Sprintf(`{"template_id":"%s","reason":"best match"}`, candidateID),
			},
		},
	}

	selection, err := selectTemplateWithModel(context.Background(), model, "create a workspace", candidates)
	require.NoError(t, err)
	require.Equal(t, candidateID, selection.ID)
	require.NotNil(t, model.generateCall)
	require.NotNil(t, model.generateCall.ToolChoice)
	require.Equal(t, fantasy.ToolChoiceNone, *model.generateCall.ToolChoice)
}
