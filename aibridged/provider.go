package aibridged

import (
	"net/http"
)

type Provider interface {
	CreateSession(w http.ResponseWriter, r *http.Request, tools ToolRegistry) (Session, error)
	Identifier() string
	BaseURL() string
	Key() string
}

type ProviderRegistry map[string]Provider
