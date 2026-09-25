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

func headersFromActor(actor *context.Actor, names map[string]string) map[string]string {
	if actor == nil {
		return nil
	}

	headers := make(map[string]string, len(names))
	if name := names["id"]; name != "" {
		headers[name] = actor.ID
	}
	if name := names["username"]; name != "" {
		if value, ok := actor.Metadata["Username"]; ok {
			if value := fmt.Sprint(value); value != "" {
				headers[name] = value
			}
		}
	}
	return headers
}
