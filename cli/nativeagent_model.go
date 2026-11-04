package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/xerrors"
)

var (
	nativeModelID       = regexp.MustCompile(`^[a-z0-9]+(?:[.-][a-z0-9]+)*$`)
	nativeOpenAIModelID = regexp.MustCompile(`^(?:gpt-|codex-|o[1-9][0-9]*(?:-|$))`)
)

// Validate model families, not a fixed catalog: availability depends on the
// installed harness and provider credentials. Restrict IDs to literal values,
// excluding config interpolation, variants, and TOML/JSON injection.
func nativeValidateModel(harness, model string) error {
	if model == "" {
		return nil
	}
	id, provider := model, ""
	if harness == "opencode" {
		provider, id, _ = strings.Cut(model, "/")
	}
	validID := len(model) <= 256 && nativeModelID.MatchString(id)
	switch harness {
	case "claude":
		if validID && (id == "haiku" || id == "sonnet" || id == "opus" || id == "fable" || id == "best" || strings.HasPrefix(id, "claude-")) {
			return nil
		}
		return xerrors.New("choose the haiku, sonnet, opus, fable, or best Claude model, or a full claude-* model ID")
	case "codex":
		if validID && nativeOpenAIModelID.MatchString(id) {
			return nil
		}
		return xerrors.New("choose an OpenAI model ID for Codex, without a provider prefix")
	case "opencode":
		if validID && ((provider == "anthropic" && strings.HasPrefix(id, "claude-")) || (provider == "openai" && nativeOpenAIModelID.MatchString(id))) {
			return nil
		}
		return xerrors.New("choose an anthropic/claude-* or openai/<OpenAI model ID> model for OpenCode")
	default:
		return xerrors.New("unknown native model harness")
	}
}

// Native configuration remains authoritative, including settings unrelated to
// model selection. Check every supported filename before creating a default.
func nativeSetupModelConfig(root, harness, model string) error {
	if err := nativeCheckPrivateDirectory(root); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if model == "" {
			return nil
		}
		if err := nativePrivateDirectory(root); err != nil {
			return err
		}
	}
	names := []string{"config.toml"}
	if harness == "opencode" {
		names = []string{"opencode.json", "opencode.jsonc"}
	}
	exists := false
	for _, name := range names {
		if _, err := nativeReadPrivate(filepath.Join(root, name)); err == nil {
			exists = true
		} else if !errors.Is(err, os.ErrNotExist) {
			return xerrors.Errorf("existing %s model configuration is not private; it was not replaced", harness)
		}
	}
	if exists || model == "" {
		return nil
	}
	data := []byte("model = " + strconv.Quote(model))
	if harness == "opencode" {
		var err error
		data, err = json.Marshal(map[string]string{"$schema": "https://opencode.ai/config.json", "model": model})
		if err != nil {
			return xerrors.Errorf("encode private OpenCode model configuration: %w", err)
		}
	}
	if err := nativeCreatePrivate(filepath.Join(root, names[0]), append(data, '\n')); err != nil {
		return xerrors.Errorf("save private %s model configuration: %w", harness, err)
	}
	return nil
}
