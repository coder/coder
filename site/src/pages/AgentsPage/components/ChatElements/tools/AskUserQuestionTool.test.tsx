import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
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

const twoQuestions = [
	...questions,
	{
		header: "Rollout",
		question: "How should the change ship?",
		options: [{ label: "Feature flag", description: "Ship it dark first." }],
	},
];

afterEach(() => {
	localStorage.clear();
});

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
					questions={twoQuestions}
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

	it("sends a skipped question as Skipped", async () => {
		const user = userEvent.setup();
		const onSubmitAnswer = vi.fn();
		render(
			<QueryClientProvider client={createTestQueryClient()}>
				<AskUserQuestionTool
					questions={twoQuestions}
					status="completed"
					isError={false}
					isChatCompleted
					isLatestAskUserQuestion
					onSubmitAnswer={onSubmitAnswer}
				/>
			</QueryClientProvider>,
		);

		await user.click(screen.getByRole("button", { name: "Skip" }));
		await user.click(screen.getByRole("button", { name: "Submit" }));

		expect(onSubmitAnswer).toHaveBeenCalledExactlyOnceWith(
			"1. Implementation Approach: Skipped\n2. Rollout: Feature flag",
		);
	});

	it("restores an unsent draft and drops it once the answer sends", async () => {
		const user = userEvent.setup();
		const onSubmitAnswer = vi.fn();
		const DraftQuestion: React.FC = () => (
			<QueryClientProvider client={createTestQueryClient()}>
				<AskUserQuestionTool
					questions={twoQuestions}
					status="completed"
					isError={false}
					isChatCompleted
					isLatestAskUserQuestion
					toolCallId="call-draft"
					onSubmitAnswer={onSubmitAnswer}
				/>
			</QueryClientProvider>
		);
		const firstRender = render(<DraftQuestion />);
		await user.type(
			screen.getByRole("textbox", { name: /other response/i }),
			"Two migrations",
		);
		await user.click(screen.getByRole("button", { name: "Next" }));
		firstRender.unmount();

		const secondRender = render(<DraftQuestion />);
		expect(screen.getByText("How should the change ship?")).toBeVisible();
		await user.click(screen.getByRole("button", { name: "Back" }));
		expect(
			screen.getByRole("textbox", { name: /other response/i }),
		).toHaveValue("Two migrations");
		await user.click(screen.getByRole("button", { name: "Next" }));
		await user.click(screen.getByRole("button", { name: "Submit" }));
		secondRender.unmount();
		render(<DraftQuestion />);

		expect(onSubmitAnswer).toHaveBeenCalledExactlyOnceWith(
			"1. Implementation Approach: Other: Two migrations\n2. Rollout: Feature flag",
		);
		expect(
			screen.getByText("How should we structure the database migration?"),
		).toBeVisible();
		expect(
			screen.getByRole("textbox", { name: /other response/i }),
		).toHaveValue("");
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

	it("announces an answerable async question", () => {
		render(
			<QueryClientProvider client={createTestQueryClient()}>
				<AskUserQuestionTool
					questions={questions}
					status="completed"
					isError={false}
					isLatestAskUserQuestion
					asyncToolCallId="call-123"
					onSubmitAnswer={vi.fn()}
				/>
			</QueryClientProvider>,
		);

		expect(screen.getByRole("status")).toHaveTextContent("The agent asked");
	});
});
