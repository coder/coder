package chatcontrol

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"strings"

	"golang.org/x/xerrors"
)

const (
	// SentinelStart marks the beginning of a scaletest control block.
	SentinelStart = "<coder-scaletest-control>"
	// SentinelEnd marks the end of a scaletest control block.
	SentinelEnd = "</coder-scaletest-control>"

	// SchemaVersion is the current control schema version.
	SchemaVersion = 1

	// DefaultToolName is the first real-workspace tool used by the scaletest.
	DefaultToolName = "execute"
	// DefaultToolCommand is the harmless execute command used by default.
	DefaultToolCommand = "echo scaletest"
)

// Control configures deterministic tool-call behavior for a single turn.
type Control struct {
	Version           int    `json:"v"`
	ToolCallsThisTurn int    `json:"tool_calls_this_turn"`
	Tool              string `json:"tool,omitempty"`
	Command           string `json:"command,omitempty"`
}

// WithDefaults applies the v1 default tool settings.
func (c Control) WithDefaults() Control {
	if c.Version == 0 {
		c.Version = SchemaVersion
	}
	if c.Tool == "" {
		c.Tool = DefaultToolName
	}
	if c.Command == "" {
		c.Command = DefaultToolCommand
	}
	return c
}

// Validate checks that the control block is internally consistent.
func (c Control) Validate() error {
	if c.Version != SchemaVersion {
		return xerrors.Errorf("validate version: must be %d", SchemaVersion)
	}
	if c.ToolCallsThisTurn < 0 {
		return xerrors.Errorf("validate tool_calls_this_turn: must not be negative")
	}
	if c.Tool == "" {
		return xerrors.Errorf("validate tool: must not be empty")
	}
	if c.Command == "" {
		return xerrors.Errorf("validate command: must not be empty")
	}
	return nil
}

// PrefixPrompt prepends the structured control block to a user prompt.
func PrefixPrompt(prompt string, control Control) (string, error) {
	control = control.WithDefaults()
	if err := control.Validate(); err != nil {
		return "", xerrors.Errorf("validate control: %w", err)
	}
	payload, err := json.Marshal(control)
	if err != nil {
		return "", xerrors.Errorf("marshal control: %w", err)
	}
	return SentinelStart + string(payload) + SentinelEnd + "\n" + prompt, nil
}

// ParsePrompt extracts a leading control block from prompt.
func ParsePrompt(prompt string) (Control, string, bool, error) {
	remainder, ok := strings.CutPrefix(prompt, SentinelStart)
	if !ok {
		return Control{}, prompt, false, nil
	}

	end := strings.Index(remainder, SentinelEnd)
	if end < 0 {
		return Control{}, "", false, xerrors.Errorf("parse control: missing closing sentinel")
	}

	var control Control
	if err := json.Unmarshal([]byte(remainder[:end]), &control); err != nil {
		return Control{}, "", false, xerrors.Errorf("parse control JSON: %w", err)
	}
	control = control.WithDefaults()
	if err := control.Validate(); err != nil {
		return Control{}, "", false, xerrors.Errorf("validate parsed control: %w", err)
	}

	stripped := remainder[end+len(SentinelEnd):]
	stripped = strings.TrimPrefix(stripped, "\n")
	return control, stripped, true, nil
}

// ToolCallsByTurn distributes totalToolCalls across totalTurns deterministically.
func ToolCallsByTurn(totalTurns, totalToolCalls int, chatSeed int64) []int {
	counts := make([]int, max(totalTurns, 0))
	if totalTurns <= 0 || totalToolCalls <= 0 {
		return counts
	}

	base := totalToolCalls / totalTurns
	remainder := totalToolCalls % totalTurns
	for i := range counts {
		counts[i] = base
	}
	if remainder == 0 {
		return counts
	}

	rng := newDeterministicRand(chatSeed)
	perm := rng.Perm(totalTurns)
	for _, turnIndex := range perm[:remainder] {
		counts[turnIndex]++
	}
	return counts
}

// DeriveChatSeed deterministically derives a per-chat seed from a top-level seed
// and stable runner identity values.
func DeriveChatSeed(seed int64, values ...string) int64 {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%d", seed)
	for _, value := range values {
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(value))
	}
	sum := h.Sum(nil)
	return int64(binary.LittleEndian.Uint64(sum[:8]))
}

func newDeterministicRand(seed int64) *rand.Rand {
	var payload [8]byte
	binary.LittleEndian.PutUint64(payload[:], uint64(seed))
	sum := sha256.Sum256(payload[:])
	return rand.New(rand.NewPCG(
		binary.LittleEndian.Uint64(sum[:8]),
		binary.LittleEndian.Uint64(sum[8:16]),
	))
}
