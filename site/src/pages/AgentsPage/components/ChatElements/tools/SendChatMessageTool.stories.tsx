import type { Meta, StoryObj } from "@storybook/react-vite";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { SendChatMessageTool } from "./SendChatMessageTool";

const meta: Meta<typeof SendChatMessageTool> = {
	title: "pages/AgentsPage/ChatElements/tools/SendChatMessageTool",
	component: SendChatMessageTool,
	args: {
		targetChatId: "chat-tree-root",
		targetTitle: "Root",
		relation: "parent",
		message: "Login test is fixed; ready for review.",
		delivery: "started",
		previousStatus: "waiting",
		targetStatus: "running",
		relayHop: 1,
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
type Story = StoryObj<typeof SendChatMessageTool>;

export const Started: Story = {};

export const Queued: Story = {
	args: {
		targetChatId: "chat-tree-grandchild",
		targetTitle: "Investigate CI timeout",
		relation: "child",
		delivery: "queued",
		previousStatus: "running",
		targetStatus: "running",
	},
};

export const InterruptingDowngraded: Story = {
	args: {
		requestedDelivery: "interrupt",
		delivery: "queued",
		downgradedFrom: "interrupt",
		previousStatus: "requires_action",
		targetStatus: "requires_action",
		relayHop: 2,
	},
};

export const Running: Story = {
	args: {
		status: "running",
		targetChatId: "parent",
		targetTitle: undefined,
		relation: undefined,
		delivery: undefined,
		previousStatus: undefined,
		targetStatus: undefined,
	},
};

export const ErrorResult: Story = {
	args: {
		status: "error",
		isError: true,
		targetChatId: "parent",
		targetTitle: undefined,
		relation: undefined,
		delivery: undefined,
		previousStatus: undefined,
		targetStatus: undefined,
		errorMessage:
			"relay depth limit reached; wait for a human to continue the conversation",
	},
};
