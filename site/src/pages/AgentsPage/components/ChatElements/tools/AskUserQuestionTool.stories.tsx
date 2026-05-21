import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { Tool } from "./Tool";

const runningPayload = {
	questions: [
		{
			header: "Implementation Approach",
			question: "How should we structure the database migration?",
			options: [
				{
					label: "Single migration",
					description:
						"One migration file with all changes. Simpler but harder to roll back.",
				},
				{
					label: "Incremental migrations",
					description:
						"Split into multiple sequential migrations. More flexible rollback.",
				},
			],
		},
	],
};

const singleQuestionPayload = {
	questions: [
		{
			header: "Implementation Approach",
			question: "How should we structure the database migration?",
			options: [
				{
					label: "Single migration",
					description:
						"One migration file with all changes. Simpler but harder to roll back.",
				},
				{
					label: "Incremental migrations",
					description:
						"Split into multiple sequential migrations. More flexible rollback.",
				},
			],
		},
	],
};

const multipleQuestionsPayload = {
	questions: [
		{
			header: "Implementation Approach",
			question: "How should we structure the database migration?",
			options: [
				{
					label: "Single migration",
					description:
						"One migration file with all changes. Simpler but harder to roll back.",
				},
				{
					label: "Incremental migrations",
					description:
						"Split into multiple sequential migrations. More flexible rollback.",
				},
			],
		},
		{
			header: "Release Plan",
			question: "Which rollout path should we use for the new agent workflow?",
			options: [
				{
					label: "Internal dry run",
					description:
						"Ship to the team first and confirm the migration flow before broader rollout.",
				},
				{
					label: "Small beta",
					description:
						"Start with a limited set of workspaces so we can gather feedback quickly.",
				},
				{
					label: "General rollout",
					description:
						"Release to every workspace after validation is complete.",
				},
			],
		},
	],
};

const submittedWizardResponse = [
	"1. Implementation Approach: Incremental migrations",
	"2. Release Plan: Small beta",
].join("\n");

const meta: Meta<typeof Tool> = {
	title: "pages/AgentsPage/ChatElements/tools/AskUserQuestion",
	component: Tool,
	decorators: [
		(Story) => (
			<div className="max-w-2xl">
				<Story />
			</div>
		),
	],
	args: { name: "ask_user_question" },
};

export default meta;

type Story = StoryObj<typeof Tool>;

export const Running: Story = {
	args: {
		status: "running",
		args: runningPayload,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		expect(canvas.getByText("Asking for clarification...")).toBeInTheDocument();
		expect(
			canvas.getByTestId("ask-user-question-loading-icon"),
		).toBeInTheDocument();
		expect(canvas.getAllByRole("radio")).toHaveLength(3);
	},
};

export const InteractiveSingleQuestion: Story = {
	args: {
		status: "completed",
		result: JSON.stringify(singleQuestionPayload),
		isChatCompleted: true,
		isLatestAskUserQuestion: true,
		onSendAskUserQuestionResponse: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const submitButton = canvas.getByRole("button", { name: "Submit" });

		expect(submitButton).toBeEnabled();
		expect(canvas.getAllByRole("radio")).toHaveLength(3);

		await userEvent.click(
			canvas.getByRole("radio", { name: /single migration/i }),
		);
		expect(submitButton).toBeEnabled();

		await userEvent.click(submitButton);
		if (!args.onSendAskUserQuestionResponse) {
			throw new Error("Missing ask-user-question response callback.");
		}
		expect(args.onSendAskUserQuestionResponse).toHaveBeenCalledWith(
			"Single migration",
		);
		expect(canvas.getByText("Submitted answer")).toBeInTheDocument();
		expect(canvas.getByText("Single migration")).toBeInTheDocument();
		expect(
			canvas.queryByRole("button", { name: "Submit" }),
		).not.toBeInTheDocument();
	},
};

export const InteractiveSingleQuestionOther: Story = {
	args: {
		status: "completed",
		result: JSON.stringify(singleQuestionPayload),
		isChatCompleted: true,
		isLatestAskUserQuestion: true,
		onSendAskUserQuestionResponse: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const submitButton = canvas.getByRole("button", { name: "Submit" });

		await userEvent.click(canvas.getByRole("radio", { name: /^other/i }));
		const otherInput = canvas.getByRole("textbox", { name: /other response/i });
		expect(otherInput).toHaveFocus();
		expect(submitButton).toBeDisabled();

		await userEvent.type(otherInput, "Use a canary rollout");
		expect(submitButton).toBeEnabled();

		await userEvent.click(submitButton);
		if (!args.onSendAskUserQuestionResponse) {
			throw new Error("Missing ask-user-question response callback.");
		}
		expect(args.onSendAskUserQuestionResponse).toHaveBeenCalledWith(
			"Other: Use a canary rollout",
		);
		expect(canvas.getByText("Other: Use a canary rollout")).toBeInTheDocument();
	},
};

export const KeyboardNavigation: Story = {
	args: {
		status: "completed",
		result: JSON.stringify(singleQuestionPayload),
		isChatCompleted: true,
		isLatestAskUserQuestion: true,
		onSendAskUserQuestionResponse: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const firstRadio = canvas.getByRole("radio", {
			name: /single migration/i,
		});
		const secondRadio = canvas.getByRole("radio", {
			name: /incremental migrations/i,
		});
		const submitButton = canvas.getByRole("button", { name: "Submit" });

		expect(firstRadio).toBeChecked();

		await userEvent.tab();
		expect(firstRadio).toHaveFocus();

		await userEvent.keyboard("{ArrowDown}");
		expect(secondRadio).toHaveFocus();

		await userEvent.keyboard(" ");
		expect(secondRadio).toBeChecked();

		await userEvent.tab();
		expect(submitButton).toHaveFocus();

		await userEvent.keyboard("{Enter}");

		if (!args.onSendAskUserQuestionResponse) {
			throw new Error("Missing ask-user-question response callback.");
		}
		expect(args.onSendAskUserQuestionResponse).toHaveBeenCalledWith(
			"Incremental migrations",
		);
		expect(canvas.getByText("Submitted answer")).toBeInTheDocument();
	},
};

export const KeyboardOtherSubmit: Story = {
	args: {
		status: "completed",
		result: JSON.stringify(singleQuestionPayload),
		isChatCompleted: true,
		isLatestAskUserQuestion: true,
		onSendAskUserQuestionResponse: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const firstRadio = canvas.getByRole("radio", {
			name: /single migration/i,
		});
		const submitButton = canvas.getByRole("button", { name: "Submit" });

		expect(firstRadio).toBeChecked();

		await userEvent.tab();
		expect(firstRadio).toHaveFocus();

		const secondRadio = canvas.getByRole("radio", {
			name: /incremental migrations/i,
		});
		const otherRadio = canvas.getByRole("radio", { name: /other/i });

		await userEvent.keyboard("{ArrowDown}");
		expect(secondRadio).toHaveFocus();

		await userEvent.keyboard("{ArrowDown}");
		expect(otherRadio).toHaveFocus();

		await userEvent.keyboard(" ");
		expect(otherRadio).toBeChecked();
		expect(submitButton).toBeDisabled();

		const otherInput = canvas.getByPlaceholderText("Describe another answer");
		expect(otherInput).toHaveFocus();

		await userEvent.type(otherInput, "Custom approach");
		expect(submitButton).toBeEnabled();

		await userEvent.keyboard("{Enter}");

		if (!args.onSendAskUserQuestionResponse) {
			throw new Error("Missing ask-user-question response callback.");
		}
		expect(args.onSendAskUserQuestionResponse).toHaveBeenCalledWith(
			"Other: Custom approach",
		);
		expect(canvas.getByText("Submitted answer")).toBeInTheDocument();
	},
};

export const InteractiveWizardStep: Story = {
	args: {
		status: "completed",
		result: JSON.stringify(multipleQuestionsPayload),
		isChatCompleted: true,
		isLatestAskUserQuestion: true,
		onSendAskUserQuestionResponse: fn(),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const nextButton = canvas.getByRole("button", { name: "Next" });

		expect(canvas.getByText("Question 1 of 2")).toBeInTheDocument();
		expect(nextButton).toBeEnabled();
		expect(
			canvas.queryByText(/Which rollout path should we use/i),
		).not.toBeInTheDocument();

		await userEvent.click(
			canvas.getByRole("radio", { name: /incremental migrations/i }),
		);
		expect(nextButton).toBeEnabled();

		await userEvent.click(nextButton);
		expect(canvas.getByText("Question 2 of 2")).toBeInTheDocument();
		expect(
			canvas.getByText(/Which rollout path should we use/i),
		).toBeInTheDocument();
		expect(canvas.getByRole("button", { name: "Submit" })).toBeEnabled();
	},
};

export const SubmittedWizard: Story = {
	args: {
		status: "completed",
		result: JSON.stringify(multipleQuestionsPayload),
		isChatCompleted: true,
		isLatestAskUserQuestion: true,
		onSendAskUserQuestionResponse: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);

		await userEvent.click(
			canvas.getByRole("radio", { name: /incremental migrations/i }),
		);
		await userEvent.click(canvas.getByRole("button", { name: "Next" }));
		await userEvent.click(canvas.getByRole("radio", { name: /small beta/i }));
		await userEvent.click(canvas.getByRole("button", { name: "Submit" }));

		if (!args.onSendAskUserQuestionResponse) {
			throw new Error("Missing ask-user-question response callback.");
		}
		expect(args.onSendAskUserQuestionResponse).toHaveBeenCalledWith(
			submittedWizardResponse,
		);
		expect(canvas.queryAllByRole("radio")).toHaveLength(0);
		expect(
			canvas.queryByRole("button", { name: "Next" }),
		).not.toBeInTheDocument();
		expect(
			canvas.queryByRole("button", { name: "Submit" }),
		).not.toBeInTheDocument();
		const submittedAnswer = canvas.getByText("Submitted answer");
		expect(submittedAnswer).toBeInTheDocument();
		expect(submittedAnswer.nextElementSibling?.textContent).toBe(
			submittedWizardResponse,
		);
	},
};

export const PreviouslyAnsweredSingleQuestion: Story = {
	args: {
		status: "completed",
		result: JSON.stringify(singleQuestionPayload),
		isChatCompleted: true,
		isLatestAskUserQuestion: false,
		previousResponseText: "Single migration",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		expect(
			canvas.getByText("How should we structure the database migration?"),
		).toBeInTheDocument();
		expect(canvas.queryAllByRole("radio")).toHaveLength(0);
		expect(canvas.queryByText("Submitted answer")).not.toBeInTheDocument();
		expect(
			canvas.queryByRole("button", { name: "Submit" }),
		).not.toBeInTheDocument();
	},
};

export const PreviouslyAnsweredWizard: Story = {
	args: {
		status: "completed",
		result: JSON.stringify(multipleQuestionsPayload),
		isChatCompleted: true,
		isLatestAskUserQuestion: false,
		previousResponseText: submittedWizardResponse,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		expect(
			canvas.getByText(/How should we structure the database migration/),
		).toBeInTheDocument();
		expect(
			canvas.getByText(/Which rollout path should we use/),
		).toBeInTheDocument();
		expect(canvas.queryAllByRole("radio")).toHaveLength(0);
		expect(canvas.queryByText("Submitted answer")).not.toBeInTheDocument();
		expect(
			canvas.queryByRole("button", { name: "Next" }),
		).not.toBeInTheDocument();
		expect(
			canvas.queryByRole("button", { name: "Submit" }),
		).not.toBeInTheDocument();
	},
};

export const ReadOnlyPreviousCall: Story = {
	args: {
		status: "completed",
		result: JSON.stringify(multipleQuestionsPayload),
		isChatCompleted: true,
		isLatestAskUserQuestion: false,
		onSendAskUserQuestionResponse: fn(),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const radios = canvas.getAllByRole("radio");

		expect(radios).toHaveLength(7);
		expect(radios[0]).toBeDisabled();
		expect(
			canvas.getByText(/How should we structure the database migration/),
		).toBeInTheDocument();
		expect(
			canvas.getByText(/Which rollout path should we use/),
		).toBeInTheDocument();
		expect(
			canvas.queryByRole("button", { name: "Next" }),
		).not.toBeInTheDocument();
		expect(
			canvas.queryByRole("button", { name: "Submit" }),
		).not.toBeInTheDocument();
	},
};

export const ErrorState: Story = {
	args: {
		status: "completed",
		isError: true,
		result: "The planning agent could not deliver follow-up questions.",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		expect(canvas.getByRole("alert")).toBeInTheDocument();
		expect(
			canvas.getByText(
				"The planning agent could not deliver follow-up questions.",
			),
		).toBeInTheDocument();
		expect(canvas.getByLabelText("Error")).toBeInTheDocument();
	},
};
