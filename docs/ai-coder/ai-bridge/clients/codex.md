# Codex CLI

The OpenAI Codex CLI is supported with specific version requirements.

## Configuration

### Version Requirement

⚠️ **Important**: You must use version `v0.58.0` of the Codex CLI.

```bash
npm install -g @openai/codex@0.58.0
```

Newer versions have a [known bug](https://github.com/openai/codex/issues/8107) that breaks the request payload when using custom endpoints.

### Support Status

*   **OpenAI Support**: ⚠️ Limited (Version restricted)
*   **Anthropic Support**: N/A

### Known Issues

*   `gpt-5-codex` support is currently [in progress](https://github.com/coder/aibridge/issues/16).

---

**References:** [Codex CLI Configuration](https://github.com/openai/codex/blob/main/docs/config.md#model_providers)
