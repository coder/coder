import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
import { AsyncQuestionCard } from "./AsyncQuestionCard";

const questions = [
	{
		header: "Cache",
		question: "Which cache should we use?",
		options: [{ label: "Redis", description: "Shared by all replicas." }],
	},
];
const defaultProps: React.ComponentProps<typeof AsyncQuestionCard> = {
	questions,
	status: "completed",
	isError: false,
	questionStatus: undefined,
	answerText: undefined,
	isInQuestionPanel: false,
};

describe("AsyncQuestionCard", () => {
	it.each([
		{
			name: "running",
			props: { status: "running" },
			lines: [
				"Asking for clarification...",
				"Asked: Which cache should we use?",
			],
		},
		{
			name: "open in the question panel",
			props: { questionStatus: "open", isInQuestionPanel: true },
			lines: [
				"Asked: Which cache should we use?",
				"Answer in the question panel above the chat input.",
			],
		},
		{
			name: "open without a question panel",
			props: { questionStatus: "open" },
			lines: ["Asked: Which cache should we use?"],
		},
		{
			name: "pending",
			props: { questionStatus: "pending", answerText: "Redis" },
			lines: ["Asked: Which cache should we use?", "Submitted answer", "Redis"],
		},
		{
			name: "answered",
			props: { questionStatus: "answered", answerText: "Redis" },
			lines: ["Asked: Which cache should we use?", "Submitted answer", "Redis"],
		},
		{
			name: "closed",
			props: { questionStatus: "closed" },
			lines: ["Asked: Which cache should we use?", "This question is closed."],
		},
		{
			name: "failed",
			props: { isError: true, errorMessage: "Invalid question." },
			lines: ["Invalid question."],
		},
		{
			name: "running without questions",
			props: { status: "running", questions: [] },
			lines: ["Asking for clarification..."],
		},
		{
			name: "completed without questions",
			props: { questions: [] },
			lines: ["No questions available."],
		},
		{
			name: "answered with no answer text",
			props: { questionStatus: "answered", answerText: "" },
			lines: [
				"Asked: Which cache should we use?",
				"Submitted answer",
				"No answer recorded.",
			],
		},
	] satisfies {
		name: string;
		props: Partial<React.ComponentProps<typeof AsyncQuestionCard>>;
		lines: string[];
	}[])("renders $name without answer controls", ({ props, lines }) => {
		const { container } = render(
			<TooltipProvider>
				<AsyncQuestionCard {...defaultProps} {...props} />
			</TooltipProvider>,
		);

		for (const line of lines) {
			expect(container.textContent).toContain(line);
		}
		if (!props.isInQuestionPanel) {
			expect(container.textContent).not.toContain(
				"Answer in the question panel above the chat input.",
			);
		}
		expect(screen.queryAllByRole("radio")).toHaveLength(0);
		expect(screen.queryAllByRole("textbox")).toHaveLength(0);
		expect(screen.queryAllByRole("button")).toHaveLength(0);
	});
});
