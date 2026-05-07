import type { Meta, StoryObj } from "@storybook/react-vite";
import { MockWorkspace } from "#/testHelpers/entities";
import { DesktopLink } from "./DesktopLink";

const meta: Meta<typeof DesktopLink> = {
	title: "modules/resources/DesktopLink",
	component: DesktopLink,
};

export default meta;
type Story = StoryObj<typeof DesktopLink>;

const Example: Story = {
	args: {
		workspaceName: MockWorkspace.name,
	},
};

export { Example as DesktopLink };
