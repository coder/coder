package intercept

import (
	"fmt"
	"strings"

	"github.com/coder/coder/v2/aibridge/context"
)

const (
	prefix = "X-AI-Bridge-Actor"
)

func ActorIDHeader() string {
	return fmt.Sprintf("%s-ID", prefix)
}

func ActorMetadataHeader(name string) string {
	return fmt.Sprintf("%s-Metadata-%s", prefix, name)
}

func IsActorHeader(name string) bool {
	return strings.HasPrefix(strings.ToLower(name), strings.ToLower(prefix))
}

// headersFromActor produces a map of headers from a given [context.Actor].
func headersFromActor(actor *context.Actor) map[string]string {
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
