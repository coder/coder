package testutil

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/aibridge/recorder"
)

// MockRecorder is a test implementation of aibridge.Recorder that
// captures all recording calls for test assertions.
type MockRecorder struct {
	mu sync.Mutex

	interceptions    []*recorder.InterceptionRecord
	tokenUsages      []*recorder.TokenUsageRecord
	userPrompts      []*recorder.PromptUsageRecord
	toolUsages       []*recorder.ToolUsageRecord
	modelThoughts    []*recorder.ModelThoughtRecord
	interceptionsEnd map[string]*recorder.InterceptionRecordEnded
}

func (m *MockRecorder) RecordInterception(_ context.Context, req *recorder.InterceptionRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.interceptions = append(m.interceptions, req)
	return nil
}

func (m *MockRecorder) RecordInterceptionEnded(_ context.Context, req *recorder.InterceptionRecordEnded) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.interceptionsEnd == nil {
		m.interceptionsEnd = make(map[string]*recorder.InterceptionRecordEnded)
	}
	if !slices.ContainsFunc(m.interceptions, func(intc *recorder.InterceptionRecord) bool { return intc.ID == req.ID }) {
		return xerrors.New("id not found")
	}
	m.interceptionsEnd[req.ID] = req
	return nil
}

func (m *MockRecorder) RecordPromptUsage(_ context.Context, req *recorder.PromptUsageRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.userPrompts = append(m.userPrompts, req)
	return nil
}

func (m *MockRecorder) RecordTokenUsage(_ context.Context, req *recorder.TokenUsageRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tokenUsages = append(m.tokenUsages, req)
	return nil
}

func (m *MockRecorder) RecordToolUsage(_ context.Context, req *recorder.ToolUsageRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.toolUsages = append(m.toolUsages, req)
	return nil
}

func (m *MockRecorder) RecordModelThought(_ context.Context, req *recorder.ModelThoughtRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.modelThoughts = append(m.modelThoughts, req)
	return nil
}

// RecordedTokenUsages returns a copy of recorded token usages in a thread-safe manner.
// Note: This is a shallow clone - the slice is copied but the pointers reference the
// same underlying records. This is sufficient for our test assertions which only read
// the data and don't modify the records.
func (m *MockRecorder) RecordedTokenUsages() []*recorder.TokenUsageRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.tokenUsages)
}

// TotalInputTokens returns the sum of input tokens across all recorded token usages.
func (m *MockRecorder) TotalInputTokens() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	var total int64
	for _, el := range m.tokenUsages {
		total += el.Input
	}
	return total
}

// TotalOutputTokens returns the sum of output tokens across all recorded token usages.
func (m *MockRecorder) TotalOutputTokens() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	var total int64
	for _, el := range m.tokenUsages {
		total += el.Output
	}
	return total
}

// TotalCacheReadInputTokens returns the sum of cache read input tokens across all recorded token usages.
func (m *MockRecorder) TotalCacheReadInputTokens() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	var total int64
	for _, el := range m.tokenUsages {
		total += el.CacheReadInputTokens
	}
	return total
}

// TotalCacheWriteInputTokens returns the sum of cache write input tokens across all recorded token usages.
func (m *MockRecorder) TotalCacheWriteInputTokens() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	var total int64
	for _, el := range m.tokenUsages {
		total += el.CacheWriteInputTokens
	}
	return total
}

// RecordedPromptUsages returns a copy of recorded prompt usages in a thread-safe manner.
// Note: This is a shallow clone (see RecordedTokenUsages for details).
func (m *MockRecorder) RecordedPromptUsages() []*recorder.PromptUsageRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.userPrompts)
}

// RecordedToolUsages returns a copy of recorded tool usages in a thread-safe manner.
// Note: This is a shallow clone (see RecordedTokenUsages for details).
func (m *MockRecorder) RecordedToolUsages() []*recorder.ToolUsageRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.toolUsages)
}

// RecordedModelThoughts returns a copy of recorded model thoughts in a thread-safe manner.
// Note: This is a shallow clone (see RecordedTokenUsages for details).
func (m *MockRecorder) RecordedModelThoughts() []*recorder.ModelThoughtRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.modelThoughts)
}

// RecordedInterceptions returns a copy of recorded interceptions in a thread-safe manner.
// Note: This is a shallow clone (see RecordedTokenUsages for details).
func (m *MockRecorder) RecordedInterceptions() []*recorder.InterceptionRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.interceptions)
}

// ToolUsages returns the raw toolUsages slice for direct field access in tests.
// Use RecordedToolUsages() for thread-safe access when assertions don't need direct field access.
func (m *MockRecorder) ToolUsages() []*recorder.ToolUsageRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.toolUsages
}

// RecordedInterceptionEnd returns the stored InterceptionRecordEnded for the
// given interception ID, or nil if not found.
func (m *MockRecorder) RecordedInterceptionEnd(id string) *recorder.InterceptionRecordEnded {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.interceptionsEnd[id]
}

// VerifyAllInterceptionsEnded verifies all recorded interceptions have been marked as completed.
func (m *MockRecorder) VerifyAllInterceptionsEnded(t *testing.T) {
	t.Helper()

	m.mu.Lock()
	defer m.mu.Unlock()
	require.Equalf(t, len(m.interceptions), len(m.interceptionsEnd), "got %v interception ended calls, want: %v", len(m.interceptionsEnd), len(m.interceptions))
	for _, intc := range m.interceptions {
		require.Containsf(t, m.interceptionsEnd, intc.ID, "interception with id: %v has not been ended", intc.ID)
	}
}

func (m *MockRecorder) VerifyModelThoughtsRecorded(t *testing.T, expected []recorder.ModelThoughtRecord) {
	thoughts := m.RecordedModelThoughts()
	if expected == nil {
		require.Empty(t, thoughts)
		return
	}

	require.Len(t, thoughts, len(expected), "unexpected number of model thoughts")

	// We can't guarantee the order of model thoughts since they're recorded separately, so
	// we have to scan all thoughts for a match.

	for _, exp := range expected {
		var matched *recorder.ModelThoughtRecord
		for _, thought := range thoughts {
			if strings.Contains(thought.Content, exp.Content) {
				matched = thought
			}
		}

		require.NotNil(t, matched, "could not find thought matching %q", exp.Content)
		require.EqualValues(t, exp.Metadata, matched.Metadata)
	}
}
