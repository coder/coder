import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { Tool } from "./Tool";

const sampleQuestion =
	"Should we extract a shared helper for tool result parsing before refactoring the agents page tool cards?";

const longQuestion = [
	"We are planning a risky refactor of the advisor tool UI after several rounds of feedback from designers, frontend engineers, and dogfood users. The goal is to keep the row readable when the advisor includes a long prompt, a remaining-use count, and an expanded body with long markdown guidance.",
	"Before changing the layout further, I want advice on whether the metadata should remain inline with the title, move into compact trailing text, or disappear when horizontal space is tight. Please weigh readability, scanability, accessibility, and consistency with adjacent tool cards.",
	"The edge case I care about most is a real agent asking a verbose strategic question that includes implementation history, user feedback, test expectations, and design constraints in one tool call. The row should still make the question easy to scan, truncate gracefully, and keep the advisor identity visually distinct from the answer.",
].join(" ");

const sampleAdvice = [
	"# Quick summary",
	"",
	"Yes, extract a helper only if at least two tool renderers will share the same normalization logic.",
	"",
	"## Why this is a good tradeoff",
	"- It keeps the renderer focused on presentation instead of JSON parsing.",
	"- It gives Storybook fixtures a smaller, more stable prop surface.",
	"- It avoids duplicating defensive fallbacks across multiple tool cards.",
	"",
	"## Suggested next steps",
	"1. Start with a small adapter in `Tool.tsx`.",
	"2. Keep the UI component free of raw transport details.",
	"3. Add stories for the success, limit, and error states before refactoring more tools.",
	"",
	"```ts",
	"type AdvisorResult = {",
	"  type: 'advice' | 'limit_reached' | 'error';",
	"  advice?: string;",
	"};",
	"```",
].join("\n");

const longAdvice = [
	"# Recommendation",
	"",
	"Prefer a dedicated presenter with a narrow prop shape.",
	"",
	"## Context",
	"This keeps the transport parsing in one place and makes visual changes easier to test.",
	"",
	...Array.from({ length: 10 }, (_, index) => [
		`### Consideration ${index + 1}`,
		"- Keep the header readable even when the question is long.",
		"- Use markdown rendering for prose and code examples.",
		"- Preserve a subtle metadata footer for debugging and support.",
		"",
		"The dedicated row should still behave like the existing tool rows, including collapse, expansion, and overflow handling for long guidance.",
		"",
	]).flat(),
	"## Follow-up questions",
	"1. Should the row stay expanded by default while running?",
	"2. Should limit states include remaining uses when the backend provides them?",
	"3. Should the error state surface the raw provider message or a friendlier summary?",
]
	.flat()
	.join("\n");

const meta: Meta<typeof Tool> = {
	title: "pages/AgentsPage/ChatElements/tools/AdvisorTool",
	component: Tool,
	args: { name: "advisor" },
};
export default meta;
type Story = StoryObj<typeof Tool>;

// Completed advice starts collapsed; expanding reveals the question and the
// markdown guidance in the card.
export const SuccessfulAdvice: Story = {
	args: {
		status: "completed",
		args: { question: sampleQuestion },
		result: {
			type: "advice",
			advice: sampleAdvice,
			advisor_model: "openai/gpt-5.1",
			remaining_uses: 3,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const toggle = canvas.getByRole("button");

		// Collapsed by default: the header is a status line and the question
		// and advice live in the card until expanded.
		expect(toggle).toHaveAttribute("aria-expanded", "false");
		expect(canvas.getByText("Consulted the advisor")).toBeInTheDocument();
		expect(canvas.queryByText(sampleQuestion)).not.toBeInTheDocument();
		expect(canvas.queryByText("Quick summary")).not.toBeInTheDocument();

		// Model name and quota are not rendered in the header.
		expect(canvas.queryByText("openai/gpt-5.1")).not.toBeInTheDocument();
		expect(canvas.queryByText("3 left")).not.toBeInTheDocument();

		await userEvent.click(toggle);
		expect(toggle).toHaveAttribute("aria-expanded", "true");
		expect(await canvas.findByText(sampleQuestion)).toBeInTheDocument();
		expect(await canvas.findByText("Quick summary")).toBeInTheDocument();
	},
};

export const Running: Story = {
	args: {
		status: "running",
		args: { question: sampleQuestion },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Running rows open expanded so streamed guidance is visible live.
		expect(canvas.getByRole("button")).toHaveAttribute("aria-expanded", "true");
		expect(canvas.getByText("Consulting the advisor")).toBeInTheDocument();
		expect(canvas.getByText(sampleQuestion)).toBeInTheDocument();
		expect(
			canvas.getByText("Reviewing context and preparing guidance."),
		).toBeInTheDocument();
	},
};

// When the model supplies a model_intent, it is the whole header label,
// matching how the exec tool renders its intent.
export const WithModelIntent: Story = {
	args: {
		status: "completed",
		args: {
			question: sampleQuestion,
			model_intent: "Weighing a refactor tradeoff",
		},
		// The backend surfaces model_intent as a top-level tool field, so the
		// story passes it the same way the timeline does.
		modelIntent: "Weighing a refactor tradeoff",
		result: {
			type: "advice",
			advice: sampleAdvice,
			remaining_uses: 2,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const toggle = canvas.getByRole("button");
		expect(toggle).toHaveAttribute("aria-expanded", "false");
		expect(
			canvas.getByText("Weighing a refactor tradeoff"),
		).toBeInTheDocument();
		expect(canvas.queryByText(/Consulted the advisor/)).not.toBeInTheDocument();
		expect(canvas.queryByText("2 left")).not.toBeInTheDocument();

		await userEvent.click(toggle);
		expect(await canvas.findByText(sampleQuestion)).toBeInTheDocument();
	},
};

export const RunningWithStreamedAdvice: Story = {
	args: {
		status: "running",
		args: { question: sampleQuestion },
		result: "Use the smaller diff while the advisor is still responding.",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Running rows are expanded, so the question shows in the card.
		expect(canvas.getByText(sampleQuestion)).toBeInTheDocument();
		expect(canvas.getByText("Consulting the advisor")).toBeInTheDocument();
		expect(
			await canvas.findByText(
				"Use the smaller diff while the advisor is still responding.",
			),
		).toBeInTheDocument();
		expect(
			canvas.queryByText("Advisor returned no guidance."),
		).not.toBeInTheDocument();
		expect(
			canvas.queryByText("Reviewing context and preparing guidance."),
		).not.toBeInTheDocument();
	},
};

export const LimitReached: Story = {
	args: {
		status: "completed",
		args: { question: sampleQuestion },
		result: {
			type: "limit_reached",
			remaining_uses: 0,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const toggle = canvas.getByRole("button");
		expect(toggle).toHaveAttribute("aria-expanded", "false");

		// The limit message lives in the body, so it appears once expanded.
		await userEvent.click(toggle);
		expect(
			await canvas.findByText("Advisor limit reached."),
		).toBeInTheDocument();
		expect(
			canvas.getByText(
				"You have reached the advisor limit for this conversation.",
			),
		).toBeInTheDocument();
		// Screen readers announce the limit state via role="status".
		expect(canvas.getByRole("status")).toBeInTheDocument();
	},
};

export const ErrorState: Story = {
	name: "Error",
	args: {
		status: "completed",
		args: { question: sampleQuestion },
		result: {
			type: "error",
			error: "The advisor service is temporarily unavailable.",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const toggle = canvas.getByRole("button");
		expect(toggle).toHaveAttribute("aria-expanded", "false");

		await userEvent.click(toggle);
		expect(
			await canvas.findByText("Advisor request failed."),
		).toBeInTheDocument();
		expect(
			canvas.getByText("The advisor service is temporarily unavailable."),
		).toBeInTheDocument();
		// Screen readers announce the error state via role="alert".
		expect(canvas.getByRole("alert")).toBeInTheDocument();
	},
};

export const EmptyQuestion: Story = {
	args: {
		status: "completed",
		args: { question: "   " },
		result: {
			type: "advice",
			advice: sampleAdvice,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// The fallback question is inside the card, hidden while collapsed.
		expect(canvas.queryByText("No question provided.")).not.toBeInTheDocument();
		await userEvent.click(canvas.getByRole("button"));
		expect(
			await canvas.findByText("No question provided."),
		).toBeInTheDocument();
		// The advice body still renders after expanding, so a refactor that
		// suppresses the body for empty questions cannot pass silently.
		expect(await canvas.findByText("Quick summary")).toBeInTheDocument();
	},
};

export const EmptyAdvice: Story = {
	args: {
		status: "completed",
		args: { question: sampleQuestion },
		result: {
			type: "advice",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button"));
		expect(
			await canvas.findByText("Advisor returned no guidance."),
		).toBeInTheDocument();
		expect(canvas.queryByText("No guidance")).not.toBeInTheDocument();
	},
};

export const BlankError: Story = {
	args: {
		status: "completed",
		isError: true,
		args: { question: sampleQuestion },
		result: {
			type: "error",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button"));
		expect(
			await canvas.findByText("Advisor request failed."),
		).toBeInTheDocument();
		expect(
			canvas.getByText("Advisor could not return guidance."),
		).toBeInTheDocument();
		expect(canvas.getByRole("alert")).toBeInTheDocument();
	},
};

// Mirrors the backend path where a tool call is marked execution-failed
// (status === "error") without a structured result payload. The renderer
// must fold the error status into the error signal so the row surfaces
// the failure instead of falling through to "Advisor returned no guidance".
export const StatusErrorWithoutResult: Story = {
	args: {
		status: "error",
		args: { question: sampleQuestion },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button"));
		expect(
			await canvas.findByText("Advisor request failed."),
		).toBeInTheDocument();
		expect(
			canvas.getByText("Advisor could not return guidance."),
		).toBeInTheDocument();
		expect(canvas.getByRole("alert")).toBeInTheDocument();
	},
};

// Mirrors the backend path where a tool call is marked execution-failed
// (status === "error") and the result payload is a raw string instead of
// a structured object. AdvisorRenderer must route the string through the
// `errorMessage` branch so the failure surfaces rather than being rendered
// as advice text.
export const StatusErrorWithStringResult: Story = {
	args: {
		status: "error",
		args: { question: sampleQuestion },
		result: "Connection timed out",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button"));
		expect(
			await canvas.findByText("Advisor request failed."),
		).toBeInTheDocument();
		expect(canvas.getByText("Connection timed out")).toBeInTheDocument();
		expect(canvas.getByRole("alert")).toBeInTheDocument();
	},
};

// Exercises the plain-string result branch in AdvisorRenderer (Tool.tsx),
// where a non-object `result` is treated as raw advice text when
// `isError` is false.
export const PlainStringResult: Story = {
	args: {
		status: "completed",
		args: { question: sampleQuestion },
		result: "Prefer extracting a shared helper once two renderers need it.",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Collapsed: the question is inside the card until expanded.
		expect(canvas.queryByText(sampleQuestion)).not.toBeInTheDocument();
		await userEvent.click(canvas.getByRole("button"));
		expect(
			await canvas.findByText(
				"Prefer extracting a shared helper once two renderers need it.",
			),
		).toBeInTheDocument();
		expect(await canvas.findByText(sampleQuestion)).toBeInTheDocument();
	},
};

// A long question renders in full inside the card; the long guidance shares
// the same scrollable region.
export const LongAdviceLongQuestion: Story = {
	name: "Long Advice + long question",
	args: {
		status: "completed",
		args: { question: longQuestion },
		result: {
			type: "advice",
			advice: longAdvice,
			advisor_model: "openai/gpt-5.1",
			remaining_uses: 12,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const toggle = canvas.getByRole("button");

		// Collapsed: neither the question nor the advice is visible.
		expect(toggle).toHaveAttribute("aria-expanded", "false");
		expect(canvas.queryByText(longQuestion)).not.toBeInTheDocument();
		expect(canvas.queryByText("12 left")).not.toBeInTheDocument();
		expect(canvas.queryByText("Follow-up questions")).not.toBeInTheDocument();

		await userEvent.click(toggle);
		expect(toggle).toHaveAttribute("aria-expanded", "true");
		expect(await canvas.findByText(longQuestion)).toBeInTheDocument();
		expect(await canvas.findByText("Follow-up questions")).toBeInTheDocument();
	},
};
