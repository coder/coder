import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
	createTestQueryClient,
	renderComponent,
} from "#/testHelpers/renderHelpers";
import { AskUserQuestionTool } from "./AskUserQuestionTool";

const questions = [
	{
		header: "Implementation Approach",
		question: "How should we structure the database migration?",
		options: [
			{ label: "Use a single migration." },
			{ label: "Use incremental migrations." },
		],
	},
];

const mockTwoQuestions = [
	...questions,
	{
		header: "Rollout",
		question: "How should the change ship?",
		options: [{ label: "Ship it dark behind a flag." }],
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

		renderComponent(
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
		expect(onSubmitAnswer).toHaveBeenNthCalledWith(
			1,
			"Use a single migration.",
		);
		expect(onSubmitAnswer).toHaveBeenNthCalledWith(
			2,
			"Use a single migration.",
		);
	});

	it("selects an answer from a click on its text and sends it", async () => {
		const user = userEvent.setup();
		const onSubmitAnswer = vi.fn();
		renderComponent(
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

		await user.click(screen.getByText("Use incremental migrations."));
		expect(
			screen.getByRole("radio", { name: "Use incremental migrations." }),
		).toBeChecked();
		await user.click(screen.getByRole("button", { name: "Submit" }));

		expect(onSubmitAnswer).toHaveBeenCalledWith("Use incremental migrations.");
	});

	it("describes an answer only when its option has a description", () => {
		renderComponent(
			<QueryClientProvider client={createTestQueryClient()}>
				<AskUserQuestionTool
					questions={[
						{
							...questions[0],
							options: [
								{ label: "Use a single migration." },
								{
									label: "Incremental migrations",
									description: "Split the change into steps.",
								},
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

		expect(
			screen.getByRole("radio", { name: "Use a single migration." }),
		).not.toHaveAccessibleDescription();
		expect(
			screen.getByRole("radio", { name: "Incremental migrations" }),
		).toHaveAccessibleDescription("Split the change into steps.");
	});

	it("moves focus to the new question on Next and Back", async () => {
		const user = userEvent.setup();
		renderComponent(
			<QueryClientProvider client={createTestQueryClient()}>
				<AskUserQuestionTool
					questions={mockTwoQuestions}
					status="completed"
					isError={false}
					isChatCompleted
					isLatestAskUserQuestion
					onSubmitAnswer={vi.fn()}
				/>
			</QueryClientProvider>,
		);

		await user.click(screen.getByRole("button", { name: "Next" }));
		expect(document.activeElement).toHaveTextContent(
			/^How should the change ship\?$/,
		);
		await user.click(screen.getByRole("button", { name: "Back" }));
		expect(document.activeElement).toHaveTextContent(
			/^How should we structure the database migration\?$/,
		);
	});

	it("selects Other from its field, but not when Tab passes through it", async () => {
		const user = userEvent.setup();
		renderComponent(
			<QueryClientProvider client={createTestQueryClient()}>
				<AskUserQuestionTool
					questions={questions}
					status="completed"
					isError={false}
					isChatCompleted
					isLatestAskUserQuestion
					onSubmitAnswer={vi.fn()}
				/>
			</QueryClientProvider>,
		);
		const otherRadio = screen.getByRole("radio", { name: "Other" });

		await user.tab();
		await user.tab();
		expect(otherRadio).not.toBeChecked();
		await user.click(screen.getByRole("textbox", { name: /other response/i }));
		expect(otherRadio).toBeChecked();
	});

	it("picks answers with number keys, but not with a modifier", async () => {
		const user = userEvent.setup();
		renderComponent(
			<QueryClientProvider client={createTestQueryClient()}>
				<AskUserQuestionTool
					questions={questions}
					status="completed"
					isError={false}
					isChatCompleted
					isLatestAskUserQuestion
					onSubmitAnswer={vi.fn()}
				/>
			</QueryClientProvider>,
		);
		const radios = screen.getAllByRole("radio");
		expect(
			radios.map((radio) => radio.getAttribute("aria-keyshortcuts")),
		).toEqual(["1", "2", "3"]);

		await user.click(radios[0]);
		await user.keyboard("2");
		expect(radios[1]).toBeChecked();
		expect(radios[1]).toHaveFocus();

		await user.keyboard("{Control>}1{/Control}{Meta>}1{/Meta}");
		expect(radios[1]).toBeChecked();

		await user.keyboard("3");
		expect(radios[2]).toBeChecked();
		expect(
			screen.getByRole("textbox", { name: /other response/i }),
		).toHaveFocus();
	});

	it("types digits in the Other field as text", async () => {
		const user = userEvent.setup();
		const onSubmitAnswer = vi.fn();
		renderComponent(
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

		await user.type(
			screen.getByRole("textbox", { name: /other response/i }),
			"2",
		);
		expect(screen.getByRole("radio", { name: "Other" })).toBeChecked();
		await user.click(screen.getByRole("button", { name: "Submit" }));

		expect(onSubmitAnswer).toHaveBeenCalledExactlyOnceWith("Other: 2");
	});

	it("moves on with Enter on an answer, then sends on the last question", async () => {
		const user = userEvent.setup();
		const onSubmitAnswer = vi.fn();
		renderComponent(
			<QueryClientProvider client={createTestQueryClient()}>
				<AskUserQuestionTool
					questions={mockTwoQuestions}
					status="completed"
					isError={false}
					isChatCompleted
					isLatestAskUserQuestion
					onSubmitAnswer={onSubmitAnswer}
				/>
			</QueryClientProvider>,
		);

		await user.click(
			screen.getByRole("radio", { name: "Use incremental migrations." }),
		);
		await user.keyboard("{Enter}");
		expect(document.activeElement).toHaveTextContent(
			/^How should the change ship\?$/,
		);
		expect(onSubmitAnswer).not.toHaveBeenCalled();

		await user.click(
			screen.getByRole("radio", { name: "Ship it dark behind a flag." }),
		);
		await user.keyboard("{Enter}");
		expect(onSubmitAnswer).toHaveBeenCalledExactlyOnceWith(
			"1. Implementation Approach: Use incremental migrations.\n2. Rollout: Ship it dark behind a flag.",
		);
	});

	it("answers every question of a set that grew while it streamed", async () => {
		const user = userEvent.setup();
		const onSubmitAnswer = vi.fn();
		const queryClient = createTestQueryClient();
		const renderTool = (
			props: Partial<React.ComponentProps<typeof AskUserQuestionTool>>,
		) => (
			<QueryClientProvider client={queryClient}>
				<AskUserQuestionTool
					questions={mockTwoQuestions}
					status="running"
					isError={false}
					{...props}
				/>
			</QueryClientProvider>
		);
		const { rerender } = renderComponent(
			renderTool({ questions: [questions[0]] }),
		);
		rerender(renderTool({}));
		rerender(
			renderTool({
				status: "completed",
				isChatCompleted: true,
				isLatestAskUserQuestion: true,
				onSubmitAnswer,
			}),
		);

		await user.click(
			screen.getByRole("radio", { name: "Use incremental migrations." }),
		);
		await user.keyboard("{Enter}");
		await user.click(
			screen.getByRole("radio", { name: "Ship it dark behind a flag." }),
		);
		await user.keyboard("{Enter}");
		expect(onSubmitAnswer).toHaveBeenCalledExactlyOnceWith(
			"1. Implementation Approach: Use incremental migrations.\n2. Rollout: Ship it dark behind a flag.",
		);
	});

	it("sends a skipped question as Skipped", async () => {
		const user = userEvent.setup();
		const onSubmitAnswer = vi.fn();
		renderComponent(
			<QueryClientProvider client={createTestQueryClient()}>
				<AskUserQuestionTool
					questions={mockTwoQuestions}
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
			"1. Implementation Approach: Skipped\n2. Rollout: Ship it dark behind a flag.",
		);
	});

	it("sends at once when the last question is skipped", async () => {
		const user = userEvent.setup();
		const onSubmitAnswer = vi.fn();
		renderComponent(
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

		await user.click(screen.getByRole("button", { name: "Skip" }));

		expect(onSubmitAnswer).toHaveBeenCalledExactlyOnceWith("Skipped");
	});

	it("restores an unsent draft and drops it once the answer sends", async () => {
		const user = userEvent.setup();
		const onSubmitAnswer = vi.fn();
		const DraftQuestion: React.FC = () => (
			<QueryClientProvider client={createTestQueryClient()}>
				<AskUserQuestionTool
					questions={mockTwoQuestions}
					status="completed"
					isError={false}
					isChatCompleted
					isLatestAskUserQuestion
					toolCallId="call-draft"
					onSubmitAnswer={onSubmitAnswer}
				/>
			</QueryClientProvider>
		);
		const firstRender = renderComponent(<DraftQuestion />);
		await user.type(
			screen.getByRole("textbox", { name: /other response/i }),
			"Two migrations",
		);
		await user.click(screen.getByRole("button", { name: "Next" }));
		firstRender.unmount();

		const secondRender = renderComponent(<DraftQuestion />);
		const backButton = screen.getByRole("button", { name: "Back" });
		expect(backButton).toBeEnabled();
		await user.click(backButton);
		expect(
			screen.getByRole("textbox", { name: /other response/i }),
		).toHaveValue("Two migrations");
		await user.click(screen.getByRole("button", { name: "Next" }));
		await user.click(screen.getByRole("button", { name: "Submit" }));
		secondRender.unmount();
		renderComponent(<DraftQuestion />);

		expect(onSubmitAnswer).toHaveBeenCalledExactlyOnceWith(
			"1. Implementation Approach: Other: Two migrations\n2. Rollout: Ship it dark behind a flag.",
		);
		expect(screen.getByRole("button", { name: "Back" })).toBeDisabled();
		expect(
			screen.getByRole("textbox", { name: /other response/i }),
		).toHaveValue("");
	});

	it("drops a saved draft once the question shows an answer", async () => {
		const user = userEvent.setup();
		const queryClient = createTestQueryClient();
		const DraftQuestion: React.FC<{ previousResponseText?: string }> = ({
			previousResponseText,
		}) => (
			<QueryClientProvider client={queryClient}>
				<AskUserQuestionTool
					questions={questions}
					status="completed"
					isError={false}
					isChatCompleted
					isLatestAskUserQuestion
					toolCallId="call-answered"
					previousResponseText={previousResponseText}
					onSubmitAnswer={vi.fn()}
				/>
			</QueryClientProvider>
		);
		const { rerender } = renderComponent(<DraftQuestion />);
		await user.type(
			screen.getByRole("textbox", { name: /other response/i }),
			"Later",
		);
		expect(localStorage).toHaveLength(1);

		rerender(<DraftQuestion previousResponseText="Use a single migration." />);

		expect(localStorage).toHaveLength(0);
	});
});
