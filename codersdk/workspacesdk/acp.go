package workspacesdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
)

// ACPPath identifies a parent chat's ephemeral sessions on a workspace agent.
func ACPPath(organization, parent uuid.UUID) string {
	return "/api/v0/acp/" + organization.String() + "/" + parent.String() + "/"
}

// ACPRequest makes an agent-local request without tying adapter lifetime to it.
func ACPRequest(ctx context.Context, conn AgentConn, method, path string, body any) (*http.Response, error) {
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, fmt.Sprintf("http://agent:%d%s", AgentHTTPAPIServerPort, path), bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	transport := &http.Transport{DialContext: conn.DialContext, DisableKeepAlives: true}
	return (&http.Client{Transport: transport}).Do(req)
}
