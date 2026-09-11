import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "react-query";
import { describe, expect, it, vi } from "vitest";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { AskUserQuestionTool } from "./AskUserQuestionTool";

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
});
