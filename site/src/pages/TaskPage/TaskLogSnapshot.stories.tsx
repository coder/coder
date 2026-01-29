import type { Meta, StoryObj } from "@storybook/react-vite";
import { API } from "api/api";
import type { TaskLogsResponse } from "api/typesGenerated";
import { spyOn } from "storybook/test";
import { TaskLogSnapshot } from "./TaskLogSnapshot";

const MockTaskLogsResponse: TaskLogsResponse = {
	logs: [
		{
			id: 1,
			content: "What's the latest GH issue?",
			type: "input",
			time: "2024-01-01T12:00:00Z",
		},
		{
			id: 2,
			content:
				"I'll fetch that for you...\nThe latest issue is #21309: Feature: Improve database migration handling for large tables.",
			type: "output",
			time: "2024-01-01T12:00:05Z",
		},
		{
			id: 3,
			content: "Can you summarize the discussion?",
			type: "input",
			time: "2024-01-01T12:00:30Z",
		},
		{
			id: 4,
			content:
				"The discussion focuses on optimizing migration performance for tables with millions of rows. Key points include:\n\n1. Using batch processing to avoid lock timeouts\n2. Adding progress indicators for long-running migrations\n3. Supporting rollback for failed migrations",
			type: "output",
			time: "2024-01-01T12:00:35Z",
		},
	],
};

const meta: Meta<typeof TaskLogSnapshot> = {
	title: "pages/TaskPage/TaskLogSnapshot",
	component: TaskLogSnapshot,
	args: {
		username: "testuser",
		taskId: "test-task-id",
		actionLabel: "Restart to view full logs",
	},
};

export default meta;
type Story = StoryObj<typeof TaskLogSnapshot>;

export const Loading: Story = {
	beforeEach: () => {
		spyOn(API, "getTaskLogs").mockImplementation(() => new Promise(() => {}));
	},
};

export const WithLogs: Story = {
	beforeEach: () => {
		spyOn(API, "getTaskLogs").mockResolvedValue(MockTaskLogsResponse);
	},
};

export const WithLogsAndLink: Story = {
	args: {
		actionLabel: "View full logs",
		actionHref: "/@testuser/test-workspace/builds/1",
	},
	beforeEach: () => {
		spyOn(API, "getTaskLogs").mockResolvedValue(MockTaskLogsResponse);
	},
};

export const Empty: Story = {
	beforeEach: () => {
		spyOn(API, "getTaskLogs").mockResolvedValue({ logs: [] });
	},
};

export const FetchError: Story = {
	beforeEach: () => {
		spyOn(API, "getTaskLogs").mockRejectedValue(new Error("Failed to fetch"));
	},
};

export const SingleUserMessage: Story = {
	beforeEach: () => {
		spyOn(API, "getTaskLogs").mockResolvedValue({
			logs: [
				{
					id: 1,
					content: "Help me implement a new feature for user authentication",
					type: "input",
					time: "2024-01-01T12:00:00Z",
				},
			],
		});
	},
};

export const SingleAgentMessage: Story = {
	beforeEach: () => {
		spyOn(API, "getTaskLogs").mockResolvedValue({
			logs: [
				{
					id: 1,
					content:
						"I'll help you implement user authentication. Let me analyze the codebase first...",
					type: "output",
					time: "2024-01-01T12:00:00Z",
				},
			],
		});
	},
};

export const LongConversation: Story = {
	beforeEach: () => {
		const logs = [];
		for (let i = 0; i < 20; i++) {
			logs.push({
				id: i * 2 + 1,
				content: `User message ${i + 1}: Can you help me with task ${i + 1}?`,
				type: "input" as const,
				time: new Date(Date.now() + i * 60000).toISOString(),
			});
			logs.push({
				id: i * 2 + 2,
				content: `Agent response ${i + 1}: Sure, I'll help you with task ${i + 1}. Here's what I found...`,
				type: "output" as const,
				time: new Date(Date.now() + i * 60000 + 5000).toISOString(),
			});
		}
		spyOn(API, "getTaskLogs").mockResolvedValue({ logs });
	},
};
