import type { Meta, StoryObj } from "@storybook/react-vite";
import {
	MockUserOwner,
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceApp,
} from "#/testHelpers/entities";
import { withAuthProvider, withProxyProvider } from "#/testHelpers/storybook";
import { WorkspaceAppFrame } from "./WorkspaceAppFrame";
import type { WorkspaceAppWithAgent } from "./workspaceApps";

const meta: Meta<typeof WorkspaceAppFrame> = {
	title: "modules/apps/WorkspaceAppFrame",
	component: WorkspaceAppFrame,
	decorators: [withAuthProvider, withProxyProvider()],
	parameters: {
		layout: "fullscreen",
		user: MockUserOwner,
	},
	args: {
		workspace: MockWorkspace,
		app: buildWorkspaceApp(),
		active: true,
	},
};

export default meta;
type Story = StoryObj<typeof meta>;

export const Unhealthy: Story = {
	args: {
		app: buildWorkspaceApp({ health: "unhealthy" }),
	},
};

function buildWorkspaceApp(
	overrides: Partial<WorkspaceAppWithAgent> = {},
): WorkspaceAppWithAgent {
	return {
		...MockWorkspaceApp,
		agent: MockWorkspaceAgent,
		health: "healthy",
		...overrides,
	};
}
