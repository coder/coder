package intercept

import (
	"fmt"

	"github.com/coder/coder/v2/aibridge/context"
	"github.com/coder/coder/v2/aibridge/utils"
)

func ActorIDHeader() string {
	return fmt.Sprintf("%s-ID", utils.ActorHeaderPrefix)
}

func ActorMetadataHeader(name string) string {
	return fmt.Sprintf("%s-Metadata-%s", utils.ActorHeaderPrefix, name)
}

func IsActorHeader(name string) bool {
	return utils.IsActorHeader(name)
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
