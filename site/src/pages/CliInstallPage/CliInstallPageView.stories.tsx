import type { Meta, StoryObj } from "@storybook/react-vite";
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

export const LongOrigin: Story = {
	args: {
		origin: "https://coder.alb.eks.us-east-1.aws.internal.cdr.dev",
	},
};

export { Example as CliInstallPage };
