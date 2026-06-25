package chatprovider_test

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"charm.land/fantasy"
	fantasybedrock "charm.land/fantasy/providers/bedrock"
)

type bedrockContextProbeCase struct {
	Endpoint string
	Region   string
	ModelID  string
}

func TestBedrockContextWindowProbe(t *testing.T) {
	if os.Getenv("BEDROCK_CONTEXT_WINDOW_TEST") != "1" {
		t.Skip("set BEDROCK_CONTEXT_WINDOW_TEST=1 to run Bedrock context probes")
	}

	maxUnits := envInt(t, "BEDROCK_CONTEXT_WINDOW_MAX", 1_000_000)
	minUnits := envInt(t, "BEDROCK_CONTEXT_WINDOW_MIN", 1)
	if minUnits < 1 {
		t.Fatalf("BEDROCK_CONTEXT_WINDOW_MIN must be positive, got %d", minUnits)
	}
	if maxUnits < minUnits {
		t.Fatalf("BEDROCK_CONTEXT_WINDOW_MAX must be >= min, got %d < %d", maxUnits, minUnits)
	}

	probeTimeout := envDuration(t, "BEDROCK_CONTEXT_WINDOW_TIMEOUT", 2*time.Minute)
	cases := []bedrockContextProbeCase{
		{
			Endpoint: "https://bedrock-runtime.eu-north-1.amazonaws.com",
			Region:   "eu-north-1",
			ModelID:  "eu.anthropic.claude-opus-4-8",
		},
		{
			Endpoint: "https://bedrock-runtime.eu-north-1.amazonaws.com",
			Region:   "eu-north-1",
			ModelID:  "us.anthropic.claude-opus-4-8",
		},
		{
			Endpoint: "https://bedrock-runtime.us-east-2.amazonaws.com",
			Region:   "us-east-2",
			ModelID:  "eu.anthropic.claude-opus-4-8",
		},
		{
			Endpoint: "https://bedrock-runtime.us-east-2.amazonaws.com",
			Region:   "us-east-2",
			ModelID:  "us.anthropic.claude-opus-4-8",
		},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("%s/%s", tc.Region, tc.ModelID), func(t *testing.T) {
			probeBedrockContextWindow(t, tc, minUnits, maxUnits, probeTimeout)
		})
	}
}

func probeBedrockContextWindow(
	t *testing.T,
	tc bedrockContextProbeCase,
	minUnits int,
	maxUnits int,
	probeTimeout time.Duration,
) {
	t.Helper()

	provider, err := fantasybedrock.New(
		fantasybedrock.WithRegion(tc.Region),
		fantasybedrock.WithBaseURL(tc.Endpoint),
	)
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}

	model, err := provider.LanguageModel(t.Context(), tc.ModelID)
	if err != nil {
		t.Fatalf("create model: %v", err)
	}

	maxResult := probeBedrockPrompt(t, model, maxUnits, probeTimeout)
	t.Logf(
		"probe endpoint=%s region=%s model=%s units=%d passed=%t duration=%s err=%v",
		tc.Endpoint,
		tc.Region,
		tc.ModelID,
		maxUnits,
		maxResult.Passed,
		maxResult.Duration,
		maxResult.Err,
	)
	if maxResult.Passed {
		t.Logf("lower bound: at least %d prompt units", maxUnits)
		return
	}
	if !maxResult.ContextTooLong {
		t.Fatalf("max probe failed with non-context error: %v", maxResult.Err)
	}

	low := minUnits - 1
	high := maxUnits
	for low+1 < high {
		mid := low + (high-low)/2
		result := probeBedrockPrompt(t, model, mid, probeTimeout)
		t.Logf(
			"probe endpoint=%s region=%s model=%s units=%d passed=%t duration=%s err=%v",
			tc.Endpoint,
			tc.Region,
			tc.ModelID,
			mid,
			result.Passed,
			result.Duration,
			result.Err,
		)
		if result.Passed {
			low = mid
			continue
		}
		if result.ContextTooLong {
			high = mid
			continue
		}
		t.Fatalf("probe failed with non-context error at %d units: %v", mid, result.Err)
	}

	t.Logf("largest passing prompt units: %d", low)
	t.Logf("smallest failing prompt units: %d", high)
}

type bedrockPromptProbeResult struct {
	Passed         bool
	ContextTooLong bool
	Duration       time.Duration
	Err            error
}

func probeBedrockPrompt(
	t *testing.T,
	model fantasy.LanguageModel,
	units int,
	timeout time.Duration,
) bedrockPromptProbeResult {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), timeout)
	defer cancel()

	maxOutputTokens := int64(1)
	prompt := buildBedrockProbePrompt(units)
	started := time.Now()
	_, err := model.Generate(ctx, fantasy.Call{
		Prompt: fantasy.Prompt{
			{
				Role: fantasy.MessageRoleUser,
				Content: []fantasy.MessagePart{
					fantasy.TextPart{Text: prompt},
				},
			},
		},
		MaxOutputTokens: &maxOutputTokens,
	})
	duration := time.Since(started)
	if err == nil {
		return bedrockPromptProbeResult{Passed: true, Duration: duration}
	}
	return bedrockPromptProbeResult{
		ContextTooLong: isBedrockContextTooLong(err),
		Duration:       duration,
		Err:            err,
	}
}

func buildBedrockProbePrompt(units int) string {
	return strings.Repeat(" ping", units)
}

func isBedrockContextTooLong(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "input is too long") ||
		strings.Contains(message, "too long for requested model") ||
		strings.Contains(message, "prompt is too long") ||
		strings.Contains(message, "context length")
}

func envInt(t *testing.T, name string, fallback int) int {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		t.Fatalf("parse %s=%q: %v", name, value, err)
	}
	return parsed
}

func envDuration(t *testing.T, name string, fallback time.Duration) time.Duration {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		t.Fatalf("parse %s=%q: %v", name, value, err)
	}
	return parsed
}
