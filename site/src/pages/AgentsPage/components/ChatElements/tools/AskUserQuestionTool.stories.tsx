import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, userEvent, within } from "storybook/test";
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
};

// The running state with no questions: the live status region renders
// instead of a form.
export const RunningEmptyQuestions: Story = {
	args: {
		status: "running",
		args: { questions: [] },
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
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const submitButton = canvas.getByRole("button", { name: "Submit" });

		await userEvent.click(
			canvas.getByRole("radio", { name: /single migration/i }),
		);

		await userEvent.click(submitButton);
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
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const submitButton = canvas.getByRole("button", { name: "Submit" });

		await userEvent.click(canvas.getByRole("radio", { name: /^other/i }));
		const otherInput = canvas.getByRole("textbox", { name: /other response/i });

		await userEvent.type(otherInput, "Use a canary rollout");

		await userEvent.click(submitButton);
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

		await userEvent.click(
			canvas.getByRole("radio", { name: /incremental migrations/i }),
		);
		await userEvent.click(nextButton);
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
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await userEvent.click(
			canvas.getByRole("radio", { name: /incremental migrations/i }),
		);
		await userEvent.click(canvas.getByRole("button", { name: "Next" }));
		await userEvent.click(canvas.getByRole("radio", { name: /small beta/i }));
		await userEvent.click(canvas.getByRole("button", { name: "Submit" }));
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
};

export const PreviouslyAnsweredWizard: Story = {
	args: {
		status: "completed",
		result: JSON.stringify(multipleQuestionsPayload),
		isChatCompleted: true,
		isLatestAskUserQuestion: false,
		previousResponseText: submittedWizardResponse,
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
};

export const CompletedRewrittenByHook: Story = {
	args: {
		status: "completed",
		result: JSON.stringify(multipleQuestionsPayload),
		isChatCompleted: true,
		isLatestAskUserQuestion: false,
		hookRewritten: true,
		onSendAskUserQuestionResponse: fn(),
	},
};

export const CompletedEmptyPayloadRewrittenByHook: Story = {
	args: {
		status: "completed",
		result: JSON.stringify({ questions: [] }),
		isChatCompleted: true,
		isLatestAskUserQuestion: false,
		hookRewritten: true,
	},
};

export const ErrorState: Story = {
	args: {
		status: "completed",
		isError: true,
		result: "The planning agent could not deliver follow-up questions.",
	},
};
