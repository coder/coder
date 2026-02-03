# Coder Research

Coder maintains several open-source research projects exploring the future of AI-assisted development. These projects serve as experimental platforms for testing new approaches to agent-based coding workflows and developer productivity tools.

## Mux

[Mux](https://mux.coder.com/) is a desktop and browser application for parallel agentic development, functioning as a "coding agent multiplexer" that enables developers to run multiple AI agents simultaneously in isolated workspaces. Visit the [GitHub repository](https://github.com/coder/mux) for source code and documentation.

### Features

- **Isolated workspace management**: Run multiple agents in parallel using local execution, git worktrees, or remote SSH without interference
- **Multi-model support**: Compatible with Anthropic (Claude), xAI (Grok), OpenAI (GPT), Ollama for local LLMs, and OpenRouter
- **Central git divergence view**: Monitor changes and potential conflicts across agent workspaces from a unified dashboard
- **Developer integration**: VS Code extension, Plan/Exec mode, vim input support, and slash commands

### Use Cases

- Managing multiple AI agents working on different features or bug fixes simultaneously
- Experimenting with different approaches to the same problem using isolated workspaces
- Conducting deep, long-running codebase research with dedicated agents exploring different architectural components

## Blink

[Blink](https://blink.coder.com/docs) is a self-hosted platform for deploying custom AI agents that integrate with GitHub, Slack, and web interfaces while maintaining full infrastructure control. The platform ships with a pre-built Scout agent specialized for coding tasks and code research. Visit the [GitHub repository](https://github.com/coder/blink) for source code and documentation.

### Features

- **Pre-built Scout agent**: Customizable coding assistant specialized for codebase research and understanding
- **Multi-channel deployment**: Web UI, Slack integration, and GitHub integration for agent interaction
- **TypeScript SDK**: Build custom agents using a developer-friendly SDK
- **Docker-based deployment**: Containerized agent execution with conversation state management
- **Team management and observability**: User controls, organization management, logs, and traces

### Use Cases

- Investigating complex codebases through natural language questions in Slack
- Providing coding assistance directly in communication channels without context switching
- Analyzing GitHub issues and pull requests with automatic context gathering
- Supporting customers via shared Slack channels with accurate codebase citations
- Diagnosing CI pipeline failures and build issues with intelligent analysis

## Project Status

Both projects are in active research and development, and early access. Coder uses these tools internally for production workflows including customer support, CI diagnostics, and business intelligence. Community feedback and contributions are welcome through their respective GitHub repositories.
