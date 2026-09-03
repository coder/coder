import { describe, expect, it } from "vitest";
import {
	getModuleFieldPlaceholder,
	MODULE_FIELD_PLACEHOLDERS,
} from "./moduleFieldPlaceholders";

describe("getModuleFieldPlaceholder", () => {
	it("returns the override for known module/variable pairs", () => {
		expect(getModuleFieldPlaceholder("codex", "model_reasoning_effort")).toBe(
			"none, minimal, low, medium, high, or xhigh",
		);
		expect(getModuleFieldPlaceholder("claude-code", "anthropic_api_key")).toBe(
			"sk-ant-...",
		);
		expect(getModuleFieldPlaceholder("claude-code", "model")).toBe(
			"e.g. claude-sonnet-5",
		);
		expect(getModuleFieldPlaceholder("git-clone", "base_dir")).toBe("$HOME");
		expect(getModuleFieldPlaceholder("git-clone", "branch_name")).toBe(
			"Default branch",
		);
		expect(getModuleFieldPlaceholder("dotfiles", "description")).toBe(
			"e.g https://dotfiles.github.io or an SSH URL git@host:user/repo",
		);
	});

	it("returns undefined for a module without overrides", () => {
		expect(getModuleFieldPlaceholder("code-server", "port")).toBeUndefined();
	});

	it("returns undefined for an unmatched variable on a known module", () => {
		expect(
			getModuleFieldPlaceholder("codex", "does_not_exist"),
		).toBeUndefined();
	});

	it("exports the full override table", () => {
		expect(Object.keys(MODULE_FIELD_PLACEHOLDERS).sort()).toEqual([
			"claude-code",
			"codex",
			"dotfiles",
			"git-clone",
		]);
	});
});
