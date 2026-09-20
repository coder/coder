import type { Meta, StoryObj } from "@storybook/react-vite";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { ListChatTreeTool } from "./ListChatTreeTool";

const meta: Meta<typeof ListChatTreeTool> = {
	title: "pages/AgentsPage/ChatElements/tools/ListChatTreeTool",
	component: ListChatTreeTool,
	args: {
		self: {
			chatId: "chat-tree-child",
			title: "Fix flaky login test",
			kind: "chat",
		},
		parent: {
			chatId: "chat-tree-root",
			title: "Root",
			kind: "root",
			status: "waiting",
		},
		childChats: [
			{
				chatId: "chat-tree-grandchild",
				title: "Investigate CI timeout",
				status: "running",
				updatedAt: "2026-02-18T00:00:00.000Z",
			},
			{
				chatId: "chat-tree-grandchild-2",
				title: "Backfill test fixtures",
				status: "waiting",
				updatedAt: "2026-02-17T12:00:00.000Z",
			},
		],
		status: "completed",
		isError: false,
	},
	parameters: {
		reactRouter: reactRouterParameters({ routing: { path: "/" } }),
	},
	decorators: [
		(Story) => (
			<div className="mx-auto w-full max-w-3xl py-6 font-sans text-xs">
				<Story />
			</div>
		),
	],
};
export default meta;
type Story = StoryObj<typeof ListChatTreeTool>;

export const WithParentAndChildren: Story = {};

export const RootWithoutParent: Story = {
	args: {
		self: { chatId: "chat-tree-root", title: "Root", kind: "root" },
		parent: null,
	},
};

export const NoChildren: Story = {
	args: { childChats: [] },
};

export const MissingParentField: Story = {
	args: { parent: undefined },
};

export const ChildWithUnparseableTimestamp: Story = {
	args: {
		childChats: [
			{
				chatId: "chat-tree-grandchild",
				title: "Investigate CI timeout",
				status: "running",
				updatedAt: "not-a-timestamp",
			},
		],
	},
};

export const Running: Story = {
	args: { status: "running", childChats: [] },
};

export const ErrorResult: Story = {
	args: {
		status: "error",
		isError: true,
		childChats: [],
		errorMessage: "list_chat_tree is not available in subagents",
	},
};
