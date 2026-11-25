import { describe, expect, it } from "vitest";
import { getThinkingDisclosureDisplay } from "./thinkingTitle";

describe("getThinkingDisclosureDisplay", () => {
	it("returns the default title for empty text", () => {
		expect(getThinkingDisclosureDisplay("  ", { isStreaming: false })).toEqual({
			title: "Thinking",
			body: "  ",
		});
	});

	it("truncates long text without a heading into the title", () => {
		const text = `${"a".repeat(120)} more`;
		expect(getThinkingDisclosureDisplay(text, { isStreaming: false })).toEqual({
			title: `${"a".repeat(100)}…`,
			ariaLabel: `Thinking: ${"a".repeat(100)}…`,
			body: text,
		});
	});

	it.each(["\u{1F600}", "e\u0301"])(
		"counts the title length in graphemes: %j",
		(grapheme) => {
			const text = grapheme.repeat(101);
			const title = `${grapheme.repeat(100)}…`;
			expect(
				getThinkingDisclosureDisplay(text, { isStreaming: false }),
			).toEqual({ title, ariaLabel: `Thinking: ${title}`, body: text });
		},
	);

	it("ends the title with an ellipsis when the bounded source is cut", () => {
		// The emoji straddles the 400-character source bound.
		const text = `[docs](https://example.com/${"x".repeat(365)}) then \u{1F600} more`;
		expect(getThinkingDisclosureDisplay(text, { isStreaming: false })).toEqual({
			title: "docs then…",
			ariaLabel: "Thinking: docs then…",
			body: text,
		});
	});

	it("strips inline Markdown from the preview title", () => {
		const text = "Check **the** `config` in [docs](https://example.com)\nnow.";
		expect(getThinkingDisclosureDisplay(text, { isStreaming: false })).toEqual({
			title: "Check the config in docs now.",
			ariaLabel: "Thinking: Check the config in docs now.",
			body: text,
		});
	});

	it.each([
		[
			"This is **strong and *emphasized* text** now",
			"This is strong and emphasized text now",
		],
		[
			"This is __strong and _emphasized_ text__ now",
			"This is strong and emphasized text now",
		],
		["See [docs](https://example.com/a_(b)) next", "See docs next"],
		["See ![docs](https://example.com/a_(b).png) next", "See docs next"],
		[
			"Use [state](draft value) before updating",
			"Use [state](draft value) before updating",
		],
		[
			"Check `Promise<User>`, `*args*`, 0 < n and n > 0 in user_id_field and APP__DB__URL",
			"Check Promise<User>, *args*, 0 < n and n > 0 in user_id_field and APP__DB__URL",
		],
		[
			"Render <ComponentName prop={value} /> and keep \\*literal\\* stars",
			"Render <ComponentName prop={value} /> and keep *literal* stars",
		],
		[
			"Use ``foo`bar``, x`` `y` ``z and ```a` as typed",
			"Use foo`bar, x`y`z and ```a` as typed",
		],
		[
			"Compute 2 * n * m, then compare 2 ** 10 and 3 ** 5",
			"Compute 2 * n * m, then compare 2 ** 10 and 3 ** 5",
		],
	])("previews the text the body renders: %j", (text, title) => {
		expect(getThinkingDisclosureDisplay(text, { isStreaming: false })).toEqual({
			title,
			ariaLabel: `Thinking: ${title}`,
			body: text,
		});
	});

	it("uses the text as the title when there is no heading", () => {
		expect(
			getThinkingDisclosureDisplay("Let me think this through.", {
				isStreaming: false,
			}),
		).toEqual({
			title: "Let me think this through.",
			ariaLabel: "Thinking: Let me think this through.",
			body: "Let me think this through.",
		});
	});

	it.each(["**Checking the co", "__Checking the co", "  \n###"])(
		"keeps the default title while a heading streams in: %j",
		(text) => {
			expect(getThinkingDisclosureDisplay(text, { isStreaming: true })).toEqual(
				{ title: "Thinking", body: text },
			);
		},
	);

	it("previews unclosed emphasis once streaming ends", () => {
		expect(
			getThinkingDisclosureDisplay("**Checking the co", { isStreaming: false }),
		).toEqual({
			title: "**Checking the co",
			ariaLabel: "Thinking: **Checking the co",
			body: "**Checking the co",
		});
	});

	it("hides unclosed inline markup in the preview while streaming", () => {
		expect(
			getThinkingDisclosureDisplay("I should **inspect", { isStreaming: true }),
		).toEqual({
			title: "I should inspect",
			ariaLabel: "Thinking: I should inspect",
			body: "I should **inspect",
		});
	});

	it("previews closed emphasis while streaming", () => {
		expect(
			getThinkingDisclosureDisplay("**Checking** the co", {
				isStreaming: true,
			}),
		).toEqual({
			title: "Checking the co",
			ariaLabel: "Thinking: Checking the co",
			body: "**Checking** the co",
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
				{ isStreaming: false },
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
				{ isStreaming: false },
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
				{ isStreaming: false },
			),
		).toEqual({
			title: "Thinking about configuring model settings",
			body: "I need to inspect the model configuration.",
		});
	});

	it("uses a body-only emphasized heading", () => {
		expect(
			getThinkingDisclosureDisplay("**Checking tool execution**", {
				isStreaming: false,
			}),
		).toEqual({
			title: "Thinking about checking tool execution",
			body: "",
		});
	});

	it("keeps ordinary opening sentences in the body", () => {
		expect(
			getThinkingDisclosureDisplay(
				[
					"I need to inspect the model configuration",
					"",
					"The model has several options.",
				].join("\n"),
				{ isStreaming: false },
			),
		).toEqual({
			title:
				"I need to inspect the model configuration The model has several options.",
			ariaLabel:
				"Thinking: I need to inspect the model configuration The model has several options.",
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
				{ isStreaming: false },
			),
		).toEqual({
			title: "Thinking about configuring model settings",
			body: "The model has several options.",
		});
	});

	it("uses a setext heading with an underline too long to parse", () => {
		expect(
			getThinkingDisclosureDisplay(`Title\n${"-".repeat(399)}\n\n## Next`, {
				isStreaming: false,
			}),
		).toEqual({ title: "Thinking about title", body: "## Next" });
	});

	it.each([
		// A stream can pause on the next item's marker, which looks like a
		// setext underline.
		["- Leap years are divisible by 4\n-", "- Leap years are divisible by 4 -"],
		["1. First step\n\n2. Second step", "1. First step 2. Second step"],
	])("does not treat list items as headings: %j", (text, title) => {
		expect(getThinkingDisclosureDisplay(text, { isStreaming: false })).toEqual({
			title,
			ariaLabel: `Thinking: ${title}`,
			body: text,
		});
	});

	it.each([
		[
			"- [ ] Verify migration\n- [x] Run tests",
			"- [ ] Verify migration - [x] Run tests",
		],
		["Compare A[^1]B first.\n\n[^1]: note", "Compare A[^1]B first. note"],
	])("keeps task states and footnote references: %j", (text, title) => {
		expect(getThinkingDisclosureDisplay(text, { isStreaming: false })).toEqual({
			title,
			ariaLabel: `Thinking: ${title}`,
			body: text,
		});
	});

	it.each([
		`# Plan ${"step ".repeat(100)}\n\n## Next`,
		`Plan ${"step ".repeat(100)}\n---\n\n## Next`,
	])("previews a first heading too long to parse: %j", (text) => {
		const title = `Plan ${"step ".repeat(19).trimEnd()}…`;
		expect(getThinkingDisclosureDisplay(text, { isStreaming: false })).toEqual({
			title,
			ariaLabel: `Thinking: ${title}`,
			body: text,
		});
	});

	it.each(["- ", "> "])(
		"scans past a long %j line that is not a heading",
		(marker) => {
			const body = `${marker}${"step ".repeat(100)}\n---\n\n`;
			expect(
				getThinkingDisclosureDisplay(`${body}## Next`, { isStreaming: false }),
			).toEqual({ title: "Thinking about next", body });
		},
	);

	it("ignores headings inside fenced code blocks", () => {
		expect(
			getThinkingDisclosureDisplay(
				["```md", "# Not the title", "```", "## Reviewing logs", "Done"].join(
					"\n",
				),
				{ isStreaming: false },
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
				{ isStreaming: false },
			),
		).toEqual({
			title: "Thinking about configuring model settings docs",
			body: "",
		});
	});

	it("preserves acronym and mixed-case heading starts", () => {
		expect(
			getThinkingDisclosureDisplay("### API configuration", {
				isStreaming: false,
			}).title,
		).toBe("Thinking about API configuration");
		expect(
			getThinkingDisclosureDisplay("### GitHub Actions", { isStreaming: false })
				.title,
		).toBe("Thinking about GitHub Actions");
	});
});
