import type { Meta, StoryObj } from "@storybook/react-vite";
import { MockWorkspace } from "#/testHelpers/entities";
import { BrowserLink } from "./BrowserLink";

const meta: Meta<typeof BrowserLink> = {
	title: "modules/resources/BrowserLink",
	component: BrowserLink,
};

export default meta;
type Story = StoryObj<typeof BrowserLink>;

const Example: Story = {
	args: {
		workspaceName: MockWorkspace.name,
	},
};

export { Example as BrowserLink };
