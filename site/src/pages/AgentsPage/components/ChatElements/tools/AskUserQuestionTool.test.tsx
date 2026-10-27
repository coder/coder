import { render, screen, waitFor } from "@testing-library/react";
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

const asyncAnswer = `${getAsyncAnswerPrefix("call-123")}Single migration`;

const renderAsyncQuestion = (
	onSubmitAnswer: React.ComponentProps<
		typeof AskUserQuestionTool
	>["onSubmitAnswer"],
) => {
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
	return {
		queueAnswer: () =>
			rerender(<AsyncQuestion previousResponseText={asyncAnswer} />),
		removeAnswer: () => rerender(<AsyncQuestion />),
	};
};

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
			const question = renderAsyncQuestion(onSubmitAnswer);

			await user.click(screen.getByRole("button", { name: "Submit" }));
			question.queueAnswer();
			question.removeAnswer();
			pendingSend.resolve(undefined);
			await user.click(await screen.findByRole("button", { name: "Submit" }));

			expect(onSubmitAnswer).toHaveBeenCalledTimes(2);
			expect(onSubmitAnswer).toHaveBeenNthCalledWith(2, asyncAnswer);
		},
	);

	it.each([
		{ failure: "before the queue shows its answer", failsFirst: true },
		{ failure: "after its answer left the queue", failsFirst: false },
	])(
		"clears the error of a send that fails $failure",
		async ({ failsFirst }) => {
			const user = userEvent.setup();
			const send = createDeferred<undefined>();
			const failSend = () => send.reject(new Error("Failed to send."));
			const question = renderAsyncQuestion(
				vi.fn().mockReturnValueOnce(send.promise),
			);

			await user.click(screen.getByRole("button", { name: "Submit" }));
			if (failsFirst) {
				failSend();
				await screen.findByRole("alert");
			}
			question.queueAnswer();
			question.removeAnswer();
			if (!failsFirst) {
				failSend();
			}

			await waitFor(() =>
				expect(screen.getByRole("button", { name: "Submit" })).toBeEnabled(),
			);
			await waitFor(() =>
				expect(screen.queryByRole("alert")).not.toBeInTheDocument(),
			);
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
