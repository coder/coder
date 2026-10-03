import type { Meta, StoryObj } from "@storybook/react-vite";
import { MockUserOwner } from "#/testHelpers/entities";
import { withAuthProvider } from "#/testHelpers/storybook";
import { WorkspaceWildcardWarning } from "./WorkspaceWildcardWarning";

const meta: Meta<typeof WorkspaceWildcardWarning> = {
	title: "modules/apps/WorkspaceWildcardWarning",
	component: WorkspaceWildcardWarning,
	decorators: [withAuthProvider],
	parameters: {
		layout: "fullscreen",
		user: MockUserOwner,
	},
};

export default meta;
type Story = StoryObj<typeof meta>;

export const WithoutEditPermission: Story = {};

export const WithEditPermission: Story = {
	parameters: {
		permissions: {
			editDeploymentConfig: true,
		},
	},
};
