import type { Meta, StoryObj } from "@storybook/react";
import { CliInstallPageView } from "./CliInstallPageView";

const meta: Meta<typeof CliInstallPageView> = {
	title: "pages/CliInstallPage",
	component: CliInstallPageView,
	args: {
		origin: "https://example.com",
	},
};

export default meta;
type Story = StoryObj<typeof CliInstallPageView>;

const Example: Story = {};

export { Example as CliInstallPage };
