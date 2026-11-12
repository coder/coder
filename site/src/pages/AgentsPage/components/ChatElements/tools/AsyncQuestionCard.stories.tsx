import type { Meta, StoryObj } from "@storybook/react-vite";
import type { AsyncQuestionStatus } from "../../ChatConversation/asyncQuestions";
import type { MergedTool } from "../../ChatConversation/types";
import { AsyncQuestionsContext } from "./AsyncQuestionsContext";
import { Tool } from "./Tool";

const questionId = "5f0c0000-0000-4000-8000-000000000001";
const questions = [
	{
		header: "Cache",
		question: "Which cache should we use?",
		options: [
			{ label: "Redis", description: "Shared by all replicas." },
			{ label: "In-memory", description: "No new service." },
		],
	},
];
const tool: MergedTool = {
	id: "question-call",
	name: "ask_user_question_async",
	args: { questions },
	result: { question_id: questionId, status: "Continue working." },
	status: "completed",
	isError: false,
};

const meta: Meta<typeof Tool> = {
	title: "pages/AgentsPage/ChatElements/tools/AsyncQuestionCard",
	component: Tool,
	decorators: [
		(Story) => (
			<div className="max-w-2xl">
				<Story />
			</div>
		),
	],
	args: {
		name: tool.name,
		args: tool.args,
		result: tool.result,
		status: "completed",
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

const renderQuestion = (
	status: AsyncQuestionStatus,
	answerText: string | undefined,
	isInQuestionPanel = false,
): Story["render"] => {
	return (args) => (
		<AsyncQuestionsContext
			value={{
				byId: new Map([[questionId, { questionId, tool, status, answerText }]]),
				panelQuestionId: isInQuestionPanel ? questionId : undefined,
			}}
		>
			<Tool {...args} />
		</AsyncQuestionsContext>
	);
};

export const Running: Story = {
	args: { status: "running", result: undefined },
};

export const Open: Story = {
	render: renderQuestion("open", undefined, true),
};

export const Pending: Story = {
	render: renderQuestion("pending", "Redis", true),
};

export const Answered: Story = {
	render: renderQuestion("answered", "Redis"),
};

export const Closed: Story = {
	render: renderQuestion("closed", undefined),
};

export const Failed: Story = {
	args: { isError: true, result: "Failed to ask questions" },
};

export const SeveralQuestionsAnswered: Story = {
	args: {
		args: {
			questions: [
				...questions,
				{
					header: "Rollout",
					question: "Which rollout path should we use?",
					options: [{ label: "Small beta", description: "Start small." }],
				},
			],
		},
	},
	render: renderQuestion("answered", "1. Cache: Redis\n2. Rollout: Small beta"),
};
