# OpenCode Interpreter

OpenCode Interpreter (Open Interpreter) supports custom OpenAI-compatible endpoints via the `api_base` parameter.

## Configuration

You can configure OpenCode Interpreter via CLI arguments or Python code.

<div class="tabs">

### Option 1: CLI Usage

Pass the `api_base` and `api_key` flags when starting the interpreter:

```bash
interpreter --api_base "https://coder.example.com/api/v2/aibridge/openai/v1" --api_key "<your-coder-session-token>"
```

### Option 2: Python Script

If you are using Open Interpreter as a library:

```python
import interpreter

interpreter.llm.api_base = "https://coder.example.com/api/v2/aibridge/openai/v1"
interpreter.llm.api_key = "<your-coder-session-token>"
interpreter.llm.model = "gpt-4o"

interpreter.chat("Hello, world!")
```

</div>

---

**References:** [Open Interpreter Custom Endpoint](https://docs.openinterpreter.com/language-models/local-models/custom-endpoint)
