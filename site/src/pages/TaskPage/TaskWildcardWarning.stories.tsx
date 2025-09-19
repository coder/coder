import { MockUserOwner } from "testHelpers/entities";
import { withAuthProvider } from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { TaskApps } from "./TaskApps";
import { TaskWildcardWarning } from "./TaskWildcardWarning";

const meta: Meta<typeof TaskWildcardWarning> = {
	title: "pages/TaskPage/TaskWildcardWarning",
	component: TaskWildcardWarning,
	decorators: [withAuthProvider],
	parameters: {
		layout: "fullscreen",
		user: MockUserOwner,
	},
};

export default meta;
type Story = StoryObj<typeof TaskApps>;

export const WithoutEditPermission: Story = {};

export const WithEditPermission: Story = {
	parameters: {
		permissions: {
			editDeploymentConfig: true,
		},
	},
};
