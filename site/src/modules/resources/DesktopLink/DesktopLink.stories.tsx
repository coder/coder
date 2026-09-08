import type { Meta, StoryObj } from "@storybook/react-vite";
import { MockWorkspace, MockWorkspaceAgent } from "#/testHelpers/entities";
import { DesktopLink } from "./DesktopLink";

const meta: Meta<typeof DesktopLink> = {
	title: "modules/resources/DesktopLink",
	component: DesktopLink,
	args: {
		workspaceName: MockWorkspace.name,
		agentName: MockWorkspaceAgent.name,
		userName: MockWorkspace.owner_name,
	},
};

export default meta;
type Story = StoryObj<typeof DesktopLink>;

export const Default: Story = {};
