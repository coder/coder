import { describe, expect, it } from "vitest";
import { getThinkingDisclosureDisplay } from "./thinkingTitle";

describe("getThinkingDisclosureDisplay", () => {
	it("returns the default title and original body when there is no heading", () => {
		expect(getThinkingDisclosureDisplay("Let me think this through.")).toEqual({
			title: "Thinking",
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
			title: "Thinking",
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
