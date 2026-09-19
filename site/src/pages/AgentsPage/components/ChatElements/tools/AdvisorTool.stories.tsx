import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
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

		await userEvent.click(toggle);
	},
};

export const Running: Story = {
	args: {
		status: "running",
		args: { question: sampleQuestion },
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
		const toggle = canvas.getByRole("button", {
			name: /Weighing a refactor tradeoff/,
		});
		await userEvent.click(toggle);
	},
};

export const RunningWithStreamedAdvice: Story = {
	args: {
		status: "running",
		args: { question: sampleQuestion },
		result: "Use the smaller diff while the advisor is still responding.",
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
		const toggle = canvas.getByRole("button", {
			name: /Advisor limit reached/,
		});
		await userEvent.click(toggle);
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
		await userEvent.click(toggle);
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
		await userEvent.click(canvas.getByRole("button"));
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
		await userEvent.click(canvas.getByRole("button"));
	},
};

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
		await userEvent.click(toggle);
	},
};
