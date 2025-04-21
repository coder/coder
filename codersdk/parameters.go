package codersdk

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/codersdk/wsjson"
	previewtypes "github.com/coder/preview/types"
	"github.com/coder/websocket"
)

// FriendlyDiagnostic is included to guarantee it is generated in the output
// types. This is used as the type override for `previewtypes.Diagnostic`.
type FriendlyDiagnostic = previewtypes.FriendlyDiagnostic

// NullHCLString is included to guarantee it is generated in the output
// types. This is used as the type override for `previewtypes.HCLString`.
type NullHCLString = previewtypes.NullHCLString

func (c *Client) TemplateVersionDynamicParameters(ctx context.Context, userID, version uuid.UUID) (*wsjson.Stream[DynamicParametersResponse, DynamicParametersRequest], error) {
	conn, err := c.Dial(ctx, fmt.Sprintf("/api/v2/users/%s/templateversions/%s/parameters", userID, version), nil)
	if err != nil {
		return nil, err
	}
	return wsjson.NewStream[DynamicParametersResponse, DynamicParametersRequest](conn, websocket.MessageText, websocket.MessageText, c.Logger()), nil
}
