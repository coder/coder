import { describe, expect, it } from "vitest";
import { getACPHarnessIcon } from "./acpHarnessIcon";

describe("getACPHarnessIcon", () => {
	it.each([
		["custom-CLAUDE-code", "claude.svg"],
		["openai-codex-acp", "openai-codex.svg"],
		["Gemini CLI", "gemini.svg"],
		["GitHub Copilot (CLI)", "github-copilot.svg"],
		["Open Code", "opencode.svg"],
		["goose-acp", "goose.svg"],
		["Sourcegraph Amp", "amp.svg"],
		["aider-acp", "aider.svg"],
		["Cursor Agent", "cursor.svg"],
		["kiro-cli", "kiro.svg"],
		["auggie-acp", "auggie.svg"],
		["Charm Crush", "charm-crush.svg"],
		["JetBrains Junie", "junie.svg"],
		["Kilo Code", "kilo-code.svg"],
		["Roo Code", "roo-code.svg"],
		["Amazon Q", "amazon-q.svg"],
		["Devin", "devin.svg"],
		["pi-acp", "pi.svg"],
		["Windsurf", "windsurf.svg"],
		["Zed", "zed.svg"],
	])("recognizes %s in either field", (client, icon) => {
		expect(getACPHarnessIcon(client, "Workspace Assistant")).toBe(
			`/icon/${icon}`,
		);
		expect(getACPHarnessIcon("custom-agent", client)).toBe(`/icon/${icon}`);
	});

	it.each([
		["codex", "Claude Code", "/icon/openai-codex.svg"],
		["claude-code", "Codex", "/icon/claude.svg"],
	])("prefers harness ID %s over display name %s", (id, name, icon) => {
		expect(getACPHarnessIcon(id, name)).toBe(icon);
	});

	it.each([
		["", ""],
		["custom-agent", "Workspace Assistant"],
		["example-agent", "Camping Assistant"],
		["customized-agent", "Specialized Assistant"],
	])("leaves unknown harness %s unmatched", (id, name) => {
		expect(getACPHarnessIcon(id, name)).toBeUndefined();
	});
});
