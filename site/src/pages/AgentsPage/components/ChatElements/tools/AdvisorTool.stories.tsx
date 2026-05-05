import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { Tool } from "./Tool";

const sampleQuestion =
	"Should we extract a shared helper for tool result parsing before refactoring the agents page tool cards?";

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
		"The dedicated card should still behave like the existing tool cards, including collapse, expansion, and overflow handling for long guidance.",
		"",
	]).flat(),
	"## Follow-up questions",
	"1. Should the card stay expanded by default?",
	"2. Should limit states include remaining uses when the backend provides them?",
	"3. Should the error state surface the raw provider message or a friendlier summary?",
]
	.flat()
	.join("\n");

const meta: Meta<typeof Tool> = {
	title: "pages/AgentsPage/ChatElements/tools/AdvisorTool",
	component: Tool,
	decorators: [
		(Story) => (
			<div className="max-w-3xl rounded-lg border border-solid border-border-default bg-surface-primary p-4">
				<Story />
			</div>
		),
	],
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
			advisor_model: "GPT-5 Advisor",
			remaining_uses: 3,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(sampleQuestion)).toBeInTheDocument();
		expect(await canvas.findByText("Quick summary")).toBeInTheDocument();
		// Guards against a regression where `resolvedResultType` drops to
		// undefined: the advice body would still render via the fallback
		// branch, but the header badge would silently switch to
		// "No guidance" instead of "Guidance ready".
		expect(canvas.getByText("Guidance ready")).toBeInTheDocument();
		expect(
			canvas.getByText(
				(_, element) =>
					element?.textContent?.replace(/\s+/g, " ").trim() ===
					"Advisor model: GPT-5 Advisor",
			),
		).toBeInTheDocument();
		expect(
			canvas.getByText(
				(_, element) =>
					element?.textContent?.replace(/\s+/g, " ").trim() ===
					"Remaining uses: 3",
			),
		).toBeInTheDocument();
	},
};

export const Running: Story = {
	args: {
		status: "running",
		args: { question: sampleQuestion },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(sampleQuestion)).toBeInTheDocument();
		// "Consulting advisor…" appears in both the header status badge and
		// the body spinner label, so we expect exactly two matches. Asserting
		// the count keeps the coverage for the body indicator even if the
		// header ever stops rendering the same string.
		expect(canvas.getAllByText("Consulting advisor…")).toHaveLength(2);
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
		expect(canvas.getByText("Advisor limit reached.")).toBeInTheDocument();
		expect(
			canvas.getByText(
				"You have reached the advisor limit for this conversation.",
			),
		).toBeInTheDocument();
		// Assert the semantic role screen readers rely on to announce the
		// limit state. A refactor that drops role="status" should fail here.
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
		expect(canvas.getByText("Advisor request failed.")).toBeInTheDocument();
		expect(
			canvas.getByText("The advisor service is temporarily unavailable."),
		).toBeInTheDocument();
		// Assert the semantic role screen readers rely on to announce the
		// error state. A refactor that drops role="alert" should fail here.
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
		expect(canvas.getByText("No question provided.")).toBeInTheDocument();
		// Confirm the advice body still renders alongside the blank-question
		// fallback, so a future refactor that suppresses the body for empty
		// questions cannot pass silently.
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
		expect(
			canvas.getByText("Advisor returned no guidance."),
		).toBeInTheDocument();
		expect(canvas.getByText("No guidance")).toBeInTheDocument();
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
		expect(canvas.getByText("Advisor request failed.")).toBeInTheDocument();
		expect(
			canvas.getByText("Advisor could not return guidance."),
		).toBeInTheDocument();
		expect(canvas.getByRole("alert")).toBeInTheDocument();
	},
};

// Mirrors the backend path where a tool call is marked execution-failed
// (status === "error") without a structured result payload. The renderer
// must fold the error status into the error signal so the card surfaces
// the failure instead of falling through to "Advisor returned no guidance".
export const StatusErrorWithoutResult: Story = {
	args: {
		status: "error",
		args: { question: sampleQuestion },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Advisor request failed.")).toBeInTheDocument();
		expect(
			canvas.getByText("Advisor could not return guidance."),
		).toBeInTheDocument();
		expect(canvas.getByRole("alert")).toBeInTheDocument();
	},
};

// Mirrors the backend path where a tool call is marked execution-failed
// (status === "error") and the result payload is a raw string instead of
// a structured object. AdvisorRenderer must route the string through the
// `errorMessage` branch so the failure surfaces in the error card rather
// than being rendered as advice text.
export const StatusErrorWithStringResult: Story = {
	args: {
		status: "error",
		args: { question: sampleQuestion },
		result: "Connection timed out",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Advisor request failed.")).toBeInTheDocument();
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
		expect(canvas.getByText(sampleQuestion)).toBeInTheDocument();
		expect(
			await canvas.findByText(
				"Prefer extracting a shared helper once two renderers need it.",
			),
		).toBeInTheDocument();
	},
};

export const LongAdvice: Story = {
	args: {
		status: "completed",
		args: { question: sampleQuestion },
		result: {
			type: "advice",
			advice: longAdvice,
			advisor_model: "GPT-5 Advisor",
			remaining_uses: 12,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const toggle = canvas.getByRole("button");

		expect(toggle).toHaveAttribute("aria-expanded", "true");
		await userEvent.click(toggle);
		expect(toggle).toHaveAttribute("aria-expanded", "false");
		expect(canvas.queryByText("Follow-up questions")).not.toBeInTheDocument();

		await userEvent.click(toggle);
		expect(toggle).toHaveAttribute("aria-expanded", "true");
		expect(await canvas.findByText("Follow-up questions")).toBeInTheDocument();

		const scrollArea = canvas.getByTestId("advisor-tool-scroll-area");
		const viewport = scrollArea.querySelector(
			"[data-radix-scroll-area-viewport]",
		);
		if (!(viewport instanceof HTMLElement)) {
			throw new globalThis.Error("Expected advisor scroll viewport.");
		}

		viewport.scrollTop = viewport.scrollHeight;
		viewport.dispatchEvent(new Event("scroll"));
		expect(viewport.scrollTop).toBeGreaterThan(0);
	},
};
