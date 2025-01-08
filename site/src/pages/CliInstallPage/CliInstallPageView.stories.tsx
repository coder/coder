import type { Meta, StoryObj } from "@storybook/react";
import { CliInstallPageView } from "./CliInstallPageView";

const meta: Meta<typeof CliInstallPageView> = {
	title: "pages/CliAuthPage",
	component: CliInstallPageView,
	args: {
		sessionToken: "some-session-token",
	},
};

export default meta;
type Story = StoryObj<typeof CliInstallPageView>;

const Example: Story = {};

export { Example as CliAuthPage };
