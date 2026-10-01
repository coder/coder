// Prelude evaluated before every script. Defines the mcp global, which
// talks to the host through the /mcp/call file: the script writes one
// JSON request and reads one JSON envelope back.
"use strict";
(function () {
  const CALL_PATH = "/mcp/call";

  function fail(code, message) {
    const err = new Error("[" + code + "] " + message);
    err.name = "MCPError";
    err.code = code;
    return err;
  }

  function request(body) {
    const f = std.open(CALL_PATH, "r+");
    if (!f) {
      throw fail("unavailable", "MCP is not available in this run");
    }
    let text;
    try {
      f.puts(JSON.stringify(body));
      f.flush();
      if (f.error()) {
        throw fail("request_too_large", "request exceeds the host call request limit");
      }
      text = f.readAsString();
    } finally {
      f.close();
    }
    let envelope;
    try {
      envelope = JSON.parse(text);
    } catch (e) {
      throw fail("internal", "unreadable host response: " + e.message);
    }
    if (!envelope || typeof envelope !== "object") {
      throw fail("internal", "unreadable host response");
    }
    if (!envelope.ok) {
      throw fail(envelope.code || "internal", envelope.error || "host call failed");
    }
    return envelope.result;
  }

  globalThis.mcp = Object.freeze({
    call(tool, args) {
      return request({ op: "call", tool: tool, args: args === undefined ? {} : args });
    },
    tools() {
      return request({ op: "tools" });
    },
    schema(tool) {
      return request({ op: "schema", tool: tool });
    },
    text(result) {
      if (!result || !Array.isArray(result.content)) {
        return "";
      }
      return result.content
        .filter((c) => c && c.type === "text" && typeof c.text === "string")
        .map((c) => c.text)
        .join("\n");
    },
  });
})();
