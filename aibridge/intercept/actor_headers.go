package intercept

import (
	"fmt"

	ant_option "github.com/anthropics/anthropic-sdk-go/option"
	oai_option "github.com/openai/openai-go/v3/option"

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

// ActorHeadersAsOpenAIOpts produces a slice of headers using OpenAI's RequestOption type.
func ActorHeadersAsOpenAIOpts(actor *context.Actor) []oai_option.RequestOption {
	var opts []oai_option.RequestOption

	headers := headersFromActor(actor)
	if len(headers) == 0 {
		return nil
	}

	for k, v := range headers {
		// [k] will be canonicalized, see [http.Header]'s [Add] method.
		opts = append(opts, oai_option.WithHeaderAdd(k, v))
	}

	return opts
}

// ActorHeadersAsAnthropicOpts produces a slice of headers using Anthropic's RequestOption type.
func ActorHeadersAsAnthropicOpts(actor *context.Actor) []ant_option.RequestOption {
	var opts []ant_option.RequestOption

	headers := headersFromActor(actor)
	if len(headers) == 0 {
		return nil
	}

	for k, v := range headers {
		// [k] will be canonicalized, see [http.Header]'s [Add] method.
		opts = append(opts, ant_option.WithHeaderAdd(k, v))
	}

	return opts
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
