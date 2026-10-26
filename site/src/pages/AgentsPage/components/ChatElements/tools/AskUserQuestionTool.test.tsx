import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "react-query";
import { describe, expect, it, vi } from "vitest";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { AskUserQuestionTool } from "./AskUserQuestionTool";
import { Tool } from "./Tool";

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

	it("answers an async question while the chat is still running", async () => {
		const user = userEvent.setup();
		const onSubmitAnswer = vi.fn();

		render(
			<QueryClientProvider client={createTestQueryClient()}>
				<Tool
					organizationId="organization-id"
					name="ask_user_question"
					toolCallId="call-123"
					status="completed"
					result={JSON.stringify({ questions, async: true })}
					isError={false}
					isChatCompleted={false}
					isLatestAskUserQuestion
					onSendAskUserQuestionResponse={onSubmitAnswer}
					subagentTitles={new Map()}
					subagentVariants={new Map()}
					mcpServers={[]}
					shellToolDisplayMode="auto"
					codeDiffDisplayMode="auto"
				/>
			</QueryClientProvider>,
		);

		await user.click(screen.getByRole("button", { name: "Submit" }));

		expect(onSubmitAnswer).toHaveBeenCalledWith(
			"Answer to ask_user_question call call-123:\nSingle migration",
		);
	});

	it("keeps a blocking question disabled while the chat is running", () => {
		render(
			<QueryClientProvider client={createTestQueryClient()}>
				<AskUserQuestionTool
					questions={questions}
					status="completed"
					isError={false}
					isChatCompleted={false}
					isLatestAskUserQuestion
					onSubmitAnswer={vi.fn()}
				/>
			</QueryClientProvider>,
		);

		for (const radio of screen.getAllByRole("radio")) {
			expect(radio).toBeDisabled();
		}
	});
});
