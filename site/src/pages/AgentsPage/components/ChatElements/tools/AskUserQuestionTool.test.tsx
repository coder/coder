import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "react-query";
import { describe, expect, it, vi } from "vitest";
import { createDeferred } from "#/testHelpers/deferred";
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

	it.each([
		{ removal: "after its send resolves", isSendPending: false },
		{ removal: "while its send is pending", isSendPending: true },
	])(
		"reopens an async question when its queued answer is removed $removal",
		async ({ isSendPending }) => {
			const user = userEvent.setup();
			const pendingSend = createDeferred<undefined>();
			const onSubmitAnswer = vi
				.fn()
				.mockReturnValueOnce(isSendPending ? pendingSend.promise : undefined);
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
			pendingSend.resolve(undefined);
			await user.click(await screen.findByRole("button", { name: "Submit" }));

			expect(onSubmitAnswer).toHaveBeenCalledTimes(2);
			expect(onSubmitAnswer).toHaveBeenNthCalledWith(2, answer);
		},
	);

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
