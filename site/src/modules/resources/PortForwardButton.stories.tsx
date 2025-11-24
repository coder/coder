import {
	MockListeningPortsResponse,
	MockSharedPortsResponse,
	MockTemplate,
	MockWorkspace,
	MockWorkspaceAgent,
} from "testHelpers/entities";
import { withDashboardProvider } from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { PortForwardButton } from "./PortForwardButton";

const meta: Meta<typeof PortForwardButton> = {
	title: "modules/resources/PortForwardButton",
	component: PortForwardButton,
	decorators: [withDashboardProvider],
	args: {
		host: "*.coder.com",
		agent: MockWorkspaceAgent,
		workspace: MockWorkspace,
		template: MockTemplate,
	},
};

export default meta;
type Story = StoryObj<typeof PortForwardButton>;

export const Example: Story = {
	parameters: {
		queries: [
			{
				key: ["portForward", MockWorkspaceAgent.id],
				data: MockListeningPortsResponse,
			},
			{
				key: ["sharedPorts", MockWorkspace.id],
				data: MockSharedPortsResponse,
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const button = canvas.getByRole("button");
		await userEvent.click(button);
	},
};

export const Loading: Story = {};
