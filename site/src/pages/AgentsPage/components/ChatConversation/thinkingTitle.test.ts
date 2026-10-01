import { describe, expect, it } from "vitest";
import { getThinkingDisclosureDisplay } from "./thinkingTitle";

describe("getThinkingDisclosureDisplay", () => {
	it("returns the default title for empty text", () => {
		expect(getThinkingDisclosureDisplay("  ")).toEqual({
			title: "Thinking",
			body: "  ",
		});
	});

	it("truncates long text without a heading into the title", () => {
		const text = `${"a".repeat(120)} more`;
		expect(getThinkingDisclosureDisplay(text)).toEqual({
			title: `${"a".repeat(100)}…`,
			body: text,
		});
	});

	it("strips inline Markdown from the preview title", () => {
		const text = "Check **the** `config` in [docs](https://example.com)\nnow.";
		expect(getThinkingDisclosureDisplay(text)).toEqual({
			title: "Check the config in docs now.",
			body: text,
		});
	});

	it("keeps text that Markdown renders literally in the preview title", () => {
		const text =
			"Check `Promise<User>`, `*args*`, 0 < n and n > 0 in user_id_field and APP__DB__URL";
		expect(getThinkingDisclosureDisplay(text)).toEqual({
			title:
				"Check Promise<User>, *args*, 0 < n and n > 0 in user_id_field and APP__DB__URL",
			body: text,
		});
	});

	it("keeps asterisks next to whitespace in the preview title", () => {
		const text = "Compute 2 * n * m, then compare 2 ** 10 and 3 ** 5";
		expect(getThinkingDisclosureDisplay(text)).toEqual({
			title: text,
			body: text,
		});
	});

	it("uses the text as the title when there is no heading", () => {
		expect(getThinkingDisclosureDisplay("Let me think this through.")).toEqual({
			title: "Let me think this through.",
			body: "Let me think this through.",
		});
	});

	it("uses the first ATX heading and removes it from the body", () => {
		expect(
			getThinkingDisclosureDisplay(
				[
					"I need to inspect the configuration.",
					"",
					"### Configuring model settings",
					"The model has several options.",
				].join("\n"),
			),
		).toEqual({
			title: "Thinking about configuring model settings",
			body: [
				"I need to inspect the configuration.",
				"",
				"The model has several options.",
			].join("\n"),
		});
	});

	it("preserves existing body content before the first heading", () => {
		expect(
			getThinkingDisclosureDisplay(
				[
					"  I need to inspect the configuration.",
					"",
					"### Configuring model settings",
					"The model has several options.",
				].join("\n"),
			),
		).toEqual({
			title: "Thinking about configuring model settings",
			body: [
				"  I need to inspect the configuration.",
				"",
				"The model has several options.",
			].join("\n"),
		});
	});

	it("uses a leading header-like paragraph and removes it from the body", () => {
		expect(
			getThinkingDisclosureDisplay(
				[
					"**Configuring model settings**",
					"",
					"I need to inspect the model configuration.",
				].join("\n"),
			),
		).toEqual({
			title: "Thinking about configuring model settings",
			body: "I need to inspect the model configuration.",
		});
	});

	it("uses a body-only emphasized heading", () => {
		expect(getThinkingDisclosureDisplay("**Checking tool execution**")).toEqual(
			{
				title: "Thinking about checking tool execution",
				body: "",
			},
		);
	});

	it("keeps ordinary opening sentences in the body", () => {
		expect(
			getThinkingDisclosureDisplay(
				[
					"I need to inspect the model configuration",
					"",
					"The model has several options.",
				].join("\n"),
			),
		).toEqual({
			title:
				"I need to inspect the model configuration The model has several options.",
			body: [
				"I need to inspect the model configuration",
				"",
				"The model has several options.",
			].join("\n"),
		});
	});

	it("uses setext headings and removes them from the body", () => {
		expect(
			getThinkingDisclosureDisplay(
				[
					"Configuring model settings",
					"---",
					"The model has several options.",
				].join("\n"),
			),
		).toEqual({
			title: "Thinking about configuring model settings",
			body: "The model has several options.",
		});
	});

	it("ignores headings inside fenced code blocks", () => {
		expect(
			getThinkingDisclosureDisplay(
				["```md", "# Not the title", "```", "## Reviewing logs", "Done"].join(
					"\n",
				),
			),
		).toEqual({
			title: "Thinking about reviewing logs",
			body: ["```md", "# Not the title", "```", "Done"].join("\n"),
		});
	});

	it("cleans common inline markdown from headings", () => {
		expect(
			getThinkingDisclosureDisplay(
				"### **Configuring** `model` settings [docs](https://example.com) ###",
			),
		).toEqual({
			title: "Thinking about configuring model settings docs",
			body: "",
		});
	});

	it("preserves acronym and mixed-case heading starts", () => {
		expect(getThinkingDisclosureDisplay("### API configuration").title).toBe(
			"Thinking about API configuration",
		);
		expect(getThinkingDisclosureDisplay("### GitHub Actions").title).toBe(
			"Thinking about GitHub Actions",
		);
	});
});
