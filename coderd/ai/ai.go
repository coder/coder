package ai

import (
	"context"

	"github.com/kylecarbs/aisdk-go"
)

type Provider func(ctx context.Context, messages []aisdk.Message) (aisdk.DataStream, error)
