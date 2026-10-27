import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "react-query";
import { describe, expect, it, vi } from "vitest";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { AskUserQuestionTool } from "./AskUserQuestionTool";
import { getAsyncAnswerPrefix } from "./asyncAnswer";

const questions = [
	{
		header: "Implementation Approach",
		question: "How should we structure the database migration?",
		options: [
			{
				label: "Single migration",
				description: "Apply all changes in one migration.",
			},
		],
	},
];

describe("AskUserQuestionTool", () => {
	it("allows retrying the same answer after submission fails", async () => {
		const user = userEvent.setup();
		const onSubmitAnswer = vi
			.fn()
			.mockRejectedValue(new Error("Failed to send."));

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

		const submitButton = screen.getByRole("button", { name: "Submit" });
		await user.click(submitButton);
		await screen.findByRole("alert");
		await user.click(submitButton);

		expect(onSubmitAnswer).toHaveBeenCalledTimes(2);
		expect(onSubmitAnswer).toHaveBeenNthCalledWith(1, "Single migration");
		expect(onSubmitAnswer).toHaveBeenNthCalledWith(2, "Single migration");
	});

	it("moves focus to the new question on Next and Back", async () => {
		const user = userEvent.setup();
		render(
			<QueryClientProvider client={createTestQueryClient()}>
				<AskUserQuestionTool
					questions={[
						...questions,
						{
							header: "Rollout",
							question: "How should the change ship?",
							options: [
								{ label: "Feature flag", description: "Ship it dark first." },
							],
						},
					]}
					status="completed"
					isError={false}
					isChatCompleted
					isLatestAskUserQuestion
					onSubmitAnswer={vi.fn()}
				/>
			</QueryClientProvider>,
		);

		await user.click(screen.getByRole("button", { name: "Next" }));
		expect(
			screen.getByText("How should the change ship?").closest("p"),
		).toHaveFocus();
		await user.click(screen.getByRole("button", { name: "Back" }));
		expect(
			screen
				.getByText("How should we structure the database migration?")
				.closest("p"),
		).toHaveFocus();
	});

	it("reopens an async question when its sent answer leaves the queue undelivered", async () => {
		const user = userEvent.setup();
		const onSubmitAnswer = vi.fn();
		const answer = `${getAsyncAnswerPrefix("call-123")}Single migration`;
		const queryClient = createTestQueryClient();
		const AsyncQuestion: React.FC<{ previousResponseText?: string }> = ({
			previousResponseText,
		}) => (
			<QueryClientProvider client={queryClient}>
				<AskUserQuestionTool
					questions={questions}
					status="completed"
					isError={false}
					isLatestAskUserQuestion
					asyncToolCallId="call-123"
					previousResponseText={previousResponseText}
					onSubmitAnswer={onSubmitAnswer}
				/>
			</QueryClientProvider>
		);
		const { rerender } = render(<AsyncQuestion />);

		await user.click(screen.getByRole("button", { name: "Submit" }));
		rerender(<AsyncQuestion previousResponseText={answer} />);
		rerender(<AsyncQuestion />);
		await user.click(screen.getByRole("button", { name: "Submit" }));

		expect(onSubmitAnswer).toHaveBeenCalledTimes(2);
		expect(onSubmitAnswer).toHaveBeenNthCalledWith(2, answer);
	});

	it.each([
		["announces an answerable async question", vi.fn(), "The agent asked"],
		["stays quiet without an answer handler", undefined, ""],
	])("%s", (_, onSubmitAnswer, announcement) => {
		render(
			<QueryClientProvider client={createTestQueryClient()}>
				<AskUserQuestionTool
					questions={questions}
					status="completed"
					isError={false}
					isLatestAskUserQuestion
					asyncToolCallId="call-123"
					onSubmitAnswer={onSubmitAnswer}
				/>
			</QueryClientProvider>,
		);

		expect(screen.getByRole("status")).toHaveTextContent(announcement);
	});
});
