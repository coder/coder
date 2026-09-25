// Package headers defines HTTP header names and helpers shared by AI Gateway
// request handlers.
package headers

import (
	"fmt"
	"strings"

	aibcontext "github.com/coder/coder/v2/aibridge/context"
)

const (
	// ActorHeaderPrefix prefixes every AI Bridge actor header.
	ActorHeaderPrefix      = "X-AI-Bridge-Actor"
	actorHeaderPrefixLower = "x-ai-bridge-actor"
)

// ActorIDHeader returns the name of the header that carries the actor ID.
func ActorIDHeader() string {
	return fmt.Sprintf("%s-ID", ActorHeaderPrefix)
}

// ActorMetadataHeader returns the name of the header that carries the actor
// metadata value for name.
func ActorMetadataHeader(name string) string {
	return fmt.Sprintf("%s-Metadata-%s", ActorHeaderPrefix, name)
}

// IsActorHeader reports whether name is an AI Bridge actor header.
func IsActorHeader(name string) bool {
	return strings.HasPrefix(strings.ToLower(name), actorHeaderPrefixLower)
}

// headersFromActor produces a map of headers from a given [aibcontext.Actor].
func headersFromActor(actor *aibcontext.Actor) map[string]string {
	if actor == nil {
		return nil
	}

	headers := make(map[string]string, len(actor.Metadata)+1)

	// Add actor ID.
	headers[ActorIDHeader()] = actor.ID

	// Add headers for provided metadata.
	for k, v := range actor.Metadata {
		headers[ActorMetadataHeader(k)] = fmt.Sprintf("%v", v)
	}

	return headers
}
