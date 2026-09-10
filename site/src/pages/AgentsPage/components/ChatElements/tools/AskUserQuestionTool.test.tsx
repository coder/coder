import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "react-query";
import { describe, expect, it, vi } from "vitest";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { AskUserQuestionTool } from "./AskUserQuestionTool";

const singleQuestion = [
	{
		header: "Implementation Approach",
		question: "How should we structure the database migration?",
		options: [
			{
				label: "Single migration",
				description: "Apply all changes in one migration.",
			},
			{
				label: "Incremental migrations",
				description: "Split into sequential migrations.",
			},
		],
	},
];

const wizardQuestions = [
	singleQuestion[0],
	{
		header: "Release Plan",
		question: "Which rollout path should we use?",
		options: [
			{ label: "Internal dry run", description: "Ship to the team first." },
			{ label: "Small beta", description: "Start with limited workspaces." },
			{ label: "General rollout", description: "Release everywhere." },
		],
	},
];

type RenderOptions = {
	questions: typeof singleQuestion;
	onSubmitAnswer: (message: string) => void | Promise<void>;
};

const renderTool = ({ questions, onSubmitAnswer }: RenderOptions) => {
	render(
		<QueryClientProvider client={createTestQueryClient()}>
			<AskUserQuestionTool
				questions={questions}
				status="completed"
				isError={false}
				isChatCompleted
				isLatestAskUserQuestion
				onSubmitAnswer={onSubmitAnswer}
			/>
		</QueryClientProvider>,
	);
};

describe("AskUserQuestionTool", () => {
	it("allows retrying the same answer after submission fails", async () => {
		const user = userEvent.setup();
		const onSubmitAnswer = vi
			.fn()
			.mockRejectedValue(new Error("Failed to send."));

		renderTool({ questions: singleQuestion, onSubmitAnswer });

		const submitButton = screen.getByRole("button", { name: "Submit" });
		await user.click(submitButton);
		await screen.findByRole("alert");
		await user.click(submitButton);

		expect(onSubmitAnswer).toHaveBeenCalledTimes(2);
		expect(onSubmitAnswer).toHaveBeenNthCalledWith(1, "Single migration");
		expect(onSubmitAnswer).toHaveBeenNthCalledWith(2, "Single migration");
	});

	it("sends the selected option instead of the default", async () => {
		const user = userEvent.setup();
		const onSubmitAnswer = vi.fn();

		renderTool({ questions: singleQuestion, onSubmitAnswer });

		await user.click(
			screen.getByRole("radio", { name: /incremental migrations/i }),
		);
		await user.click(screen.getByRole("button", { name: "Submit" }));

		expect(onSubmitAnswer).toHaveBeenCalledWith("Incremental migrations");
	});

	it("formats the Other response as 'Other: <typed text>'", async () => {
		const user = userEvent.setup();
		const onSubmitAnswer = vi.fn();

		renderTool({ questions: singleQuestion, onSubmitAnswer });

		await user.click(screen.getByRole("radio", { name: /^other/i }));
		const otherInput = screen.getByRole("textbox", {
			name: /other response/i,
		});
		await user.type(otherInput, "Use a canary rollout");
		await user.click(screen.getByRole("button", { name: "Submit" }));

		expect(onSubmitAnswer).toHaveBeenCalledTimes(1);
		expect(onSubmitAnswer).toHaveBeenCalledWith("Other: Use a canary rollout");
	});

	it("numbers each answer in a multi-question response", async () => {
		const user = userEvent.setup();
		const onSubmitAnswer = vi.fn();

		renderTool({ questions: wizardQuestions, onSubmitAnswer });

		await user.click(
			screen.getByRole("radio", { name: /incremental migrations/i }),
		);
		await user.click(screen.getByRole("button", { name: "Next" }));
		await user.click(screen.getByRole("radio", { name: /small beta/i }));
		await user.click(screen.getByRole("button", { name: "Submit" }));

		expect(onSubmitAnswer).toHaveBeenCalledTimes(1);
		expect(onSubmitAnswer).toHaveBeenCalledWith(
			[
				"1. Implementation Approach: Incremental migrations",
				"2. Release Plan: Small beta",
			].join("\n"),
		);
	});
});
