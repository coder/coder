import { Meta, StoryObj } from "@storybook/react";
import { ChatToolInvocation } from "./ChatToolInvocation";
import { MockWorkspace } from "testHelpers/entities";

const meta: Meta<typeof ChatToolInvocation> = {
	title: "pages/ChatPage/ChatToolInvocation",
	component: ChatToolInvocation,
};

export default meta;
type Story = StoryObj<typeof ChatToolInvocation>;

export const GetWorkspace: Story = {
	args: {
		toolInvocation: {
			toolName: "coder_get_workspace",
			args: {
				id: MockWorkspace.id,
			},
			result: MockWorkspace,
			state: "result",
			toolCallId: "some-id",
		},
	},
};

export const CreateWorkspace: Story = {
	args: {
		toolInvocation: {
			toolName: "coder_create_workspace",
			args: {
				name: MockWorkspace.name,
				rich_parameters: {},
				template_version_id: MockWorkspace.template_active_version_id,
				user: MockWorkspace.owner_name,
			},
			result: MockWorkspace,
			state: "result",
			toolCallId: "some-id",
		},
	},
};
