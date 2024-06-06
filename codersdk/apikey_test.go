package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

const (
	ScopeAll                APIKeyScope = "all"
	ScopeApplicationConnect APIKeyScope = "application_connect"
)

func TestCreateToken(t *testing.T) {

	tests := []struct {
		Name      string
		UserID    string
		TokenName string
		Key       string
	}{
		{Name: "User_1", UserID: "user123", TokenName: "test_token1", Key: "dwi042jLDX"},
		{Name: "User_2", UserID: "user234", TokenName: "test_token2", Key: "hHuBXWbYBp"},
		{Name: "User_3", UserID: "user314", TokenName: "test_token3", Key: "b922yyrtTY"},
		{Name: "User_4", UserID: "user290", TokenName: "test_token4", Key: "awRT67830p"},
		{Name: "User_5", UserID: "user465", TokenName: "test_token5", Key: "errtY73XwR"},
	}
	for _, tc := range tests {
		t.Run(tc.Name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, fmt.Sprintf("/api/v2/users/%s/keys/tokens", tc.UserID), r.URL.Path)
				assert.Equal(t, http.MethodPost, r.Method)

				var req CreateTokenRequest
				err := json.NewDecoder(r.Body).Decode(&req)
				assert.NoError(t, err)
				assert.Equal(t, 24*time.Hour, req.Lifetime)
				assert.Equal(t, ScopeApplicationConnect, req.Scope)
				assert.Equal(t, tc.TokenName, req.TokenName)
				//initailize httpheaders
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(GenerateAPIKeyResponse{Key: tc.Key})
			}))
			defer ts.Close()
			//mock server URL
			baseURL, err := url.Parse(ts.URL)
			assert.NoError(t, err)
			//initialize the client struct
			client := &Client{
				HTTPClient: &http.Client{Timeout: 10 * time.Second},
				URL:        baseURL,
			}
			req := CreateTokenRequest{
				Lifetime:  24 * time.Hour,
				Scope:     ScopeApplicationConnect,
				TokenName: tc.TokenName,
			}
			ctx := context.Background()
			resp, err := client.CreateToken(ctx, tc.UserID, req)
			assert.NoError(t, err)
			assert.Equal(t, tc.Key, resp.Key)
			assert.NotNil(t, resp.Key)
		})
	}
}

func TestCreateAPIKey(t *testing.T) {
	tests := []struct {
		Name   string
		UserID string
		Key    string
	}{
		{Name: "User_1", UserID: "user435", Key: "BWNR1ZxK9b"},
		{Name: "User_2", UserID: "user564", Key: "95W0eOAy7v"},
		{Name: "User_3", UserID: "user712", Key: "75W0ePAy9a"},
	}
	for _, tc := range tests {
		t.Run(tc.Name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, fmt.Sprintf("/api/v2/users/%s/keys", tc.UserID), r.URL.Path)
				assert.Equal(t, http.MethodPost, r.Method)

				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(GenerateAPIKeyResponse{Key: tc.Key})
			}))
			defer ts.Close()
			baseURL, err := url.Parse(ts.URL)
			assert.NoError(t, err)

			client := &Client{
				HTTPClient: &http.Client{Timeout: 10 * time.Second},
				URL:        baseURL,
			}
			ctx := context.Background()
			resp, err := client.CreateAPIKey(ctx, tc.UserID)
			assert.NoError(t, err)
			assert.Equal(t, tc.Key, resp.Key)
			assert.NotNil(t, resp.Key)
		})
	}
}
