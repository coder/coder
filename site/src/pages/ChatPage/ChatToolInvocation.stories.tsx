import { Meta, StoryObj } from "@storybook/react";
import { ChatToolInvocation } from "./ChatToolInvocation";
import {
	MockStartingWorkspace,
	MockStoppedWorkspace,
	MockStoppingWorkspace,
	MockWorkspace,
} from "testHelpers/entities";

const meta: Meta<typeof ChatToolInvocation> = {
	title: "pages/ChatPage/ChatToolInvocation",
	component: ChatToolInvocation,
};

export default meta;
type Story = StoryObj<typeof ChatToolInvocation>;

export const GetWorkspace: Story = {
	render: () =>
		renderInvocations(
			"coder_get_workspace",
			{
				id: MockWorkspace.id,
			},
			MockWorkspace,
		),
};

export const CreateWorkspace: Story = {
	render: () =>
		renderInvocations(
			"coder_create_workspace",
			{
				name: MockWorkspace.name,
				rich_parameters: {},
				template_version_id: MockWorkspace.template_active_version_id,
				user: MockWorkspace.owner_name,
			},
			MockWorkspace,
		),
};

export const ListWorkspaces: Story = {
	render: () =>
		renderInvocations(
			"coder_list_workspaces",
			{
				owner: "me",
			},
			[
				MockWorkspace,
				MockStoppedWorkspace,
				MockStoppingWorkspace,
				MockStartingWorkspace,
			],
		),
};

const renderInvocations = <T extends ChatToolInvocation["toolName"]>(
	toolName: T,
	args: Extract<ChatToolInvocation, { toolName: T }>["args"],
	result: Extract<
		ChatToolInvocation,
		{ toolName: T; state: "result" }
	>["result"],
	error?: string,
) => {
	return (
		<>
			<ChatToolInvocation
				toolInvocation={{
					toolCallId: "call",
					toolName,
					args: args as any,
					state: "call",
				}}
			/>
			<ChatToolInvocation
				toolInvocation={{
					toolCallId: "partial-call",
					toolName,
					args: args as any,
					state: "partial-call",
				}}
			/>
			<ChatToolInvocation
				toolInvocation={{
					toolCallId: "result",
					toolName,
					args: args as any,
					state: "result",
					result: result as any,
				}}
			/>
			<ChatToolInvocation
				toolInvocation={{
					toolCallId: "result",
					toolName,
					args: args as any,
					state: "result",
					result: {
						error: error || "Something bad happened!",
					},
				}}
			/>
		</>
	);
};
