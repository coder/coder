import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { Tool } from "./Tool";

const runningPayload = {
	questions: [
		{
			header: "Implementation Approach",
			question: "How should we structure the database migration?",
			options: [
				{ label: "Use a single migration." },
				{ label: "Use incremental migrations." },
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
				{ label: "Use a single migration." },
				{ label: "Use incremental migrations." },
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
				{ label: "Use a single migration." },
				{ label: "Use incremental migrations." },
			],
		},
		{
			header: "Release Plan",
			question: "Which rollout path should we use for the new agent workflow?",
			options: [
				{ label: "Do an internal dry run first." },
				{ label: "Start with a small beta." },
				{ label: "Roll out to every workspace once it is validated." },
			],
		},
	],
};

const submittedWizardResponse = [
	"1. Implementation Approach: Use incremental migrations.",
	"2. Release Plan: Start with a small beta.",
].join("\n");

const markdownQuestionPayload = {
	questions: [
		{
			header: "Release hold",
			question:
				"### Which release does the hold wait for?\n\nThe bottom PR, **#30996**, already merged.\n\n| PR | Kind | Status |\n|----|------|--------|\n| #30997 | tests only | CI passed |\n| #30998 | **breaking**: removes the `v0` decoder | CI passed |",
			options: [
				{ label: "Merge right after the next release branch is cut." },
				{ label: "Land the tests now and hold only the breaking PRs." },
			],
		},
	],
};

const mockLegacyOptionDescriptionsPayload = {
	questions: [
		{
			...singleQuestionPayload.questions[0],
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
	args: {
		name: "ask_user_question",
		organizationId: "organization-id",
		mcpServers: [],
		isError: false,
		subagentTitles: new Map(),
		subagentVariants: new Map(),
		shellToolDisplayMode: "auto",
		codeDiffDisplayMode: "auto",
	},
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
			"Use a single migration.",
		);
		expect(canvas.getByText("Submitted answer")).toBeInTheDocument();
		expect(canvas.getByText("Use a single migration.")).toBeInTheDocument();
		expect(
			canvas.queryByRole("button", { name: "Submit" }),
		).not.toBeInTheDocument();
	},
};

export const InteractiveMarkdownQuestion: Story = {
	args: {
		status: "completed",
		result: JSON.stringify(markdownQuestionPayload),
		isChatCompleted: true,
		isLatestAskUserQuestion: true,
		onSendAskUserQuestionResponse: fn(),
	},
};

export const PreviouslyAnsweredMarkdownQuestion: Story = {
	args: {
		status: "completed",
		result: JSON.stringify(markdownQuestionPayload),
		isChatCompleted: true,
		isLatestAskUserQuestion: false,
		previousResponseText: "Merge right after the next release branch is cut.",
	},
};

export const LegacyOptionDescriptions: Story = {
	args: {
		status: "completed",
		result: JSON.stringify(mockLegacyOptionDescriptionsPayload),
		isChatCompleted: true,
		isLatestAskUserQuestion: true,
		onSendAskUserQuestionResponse: fn(),
	},
};

export const LongUnbrokenAnswers: Story = {
	args: {
		status: "completed",
		result: JSON.stringify({
			questions: [
				{
					...singleQuestionPayload.questions[0],
					options: [
						{
							label:
								"Read site/src/pages/AgentsPage/components/ChatElements/tools/AskUserQuestionTool.stories.tsx first.",
						},
						{
							label: "Legacy option",
							description:
								"https://github.com/coder/coder/blob/main/coderd/x/chatd/chattool/askuserquestion.go",
						},
					],
				},
			],
		}),
		isChatCompleted: true,
		isLatestAskUserQuestion: true,
		onSendAskUserQuestionResponse: fn(),
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

		const otherInput = canvas.getByRole("textbox", { name: /other response/i });
		await userEvent.click(otherInput);
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
		await userEvent.tab();
		expect(submitButton).toHaveFocus();

		await userEvent.keyboard("{Enter}");

		if (!args.onSendAskUserQuestionResponse) {
			throw new Error("Missing ask-user-question response callback.");
		}
		expect(args.onSendAskUserQuestionResponse).toHaveBeenCalledWith(
			"Use incremental migrations.",
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

		await userEvent.tab();
		const otherInput = canvas.getByRole("textbox", { name: /other response/i });
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

export const NumberKeySelection: Story = {
	args: {
		status: "completed",
		result: JSON.stringify(singleQuestionPayload),
		isChatCompleted: true,
		isLatestAskUserQuestion: true,
		onSendAskUserQuestionResponse: fn(),
	},
	play: async () => {
		await userEvent.tab();
		await userEvent.keyboard("2");
	},
};

export const OtherAnswerTyped: Story = {
	args: {
		status: "completed",
		result: JSON.stringify(singleQuestionPayload),
		isChatCompleted: true,
		isLatestAskUserQuestion: true,
		onSendAskUserQuestionResponse: fn(),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.type(
			canvas.getByRole("textbox", { name: /other response/i }),
			"Use a canary rollout",
		);
	},
};

export const Submitting: Story = {
	args: {
		status: "completed",
		result: JSON.stringify(singleQuestionPayload),
		isChatCompleted: true,
		isLatestAskUserQuestion: true,
		onSendAskUserQuestionResponse: fn(() => new Promise<void>(() => {})),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: "Submit" }));
		await canvas.findByRole("button", { name: /submitting/i });
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
		previousResponseText: "Use a single migration.",
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
