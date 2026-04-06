import type { Meta, StoryObj } from "@storybook/react-vite";
import { WorkspaceAlert } from "./WorkspaceAlert";

const meta: Meta<typeof WorkspaceAlert> = {
	title: "pages/WorkspacePage/WorkspaceAlert",
	component: WorkspaceAlert,
	args: {
		title: "Something went wrong",
		detail:
			"A useful description of what happened and what the user can do about it.",
		troubleshootingURL: "https://coder.com/docs/troubleshoot",
	},
};

export default meta;
type Story = StoryObj<typeof WorkspaceAlert>;

export const WarningProminent: Story = {
	args: {
		severity: "warning",
		prominent: true,
	},
};

export const WarningSubtle: Story = {
	args: {
		severity: "warning",
		prominent: false,
	},
};

export const InfoProminent: Story = {
	args: {
		severity: "info",
		prominent: true,
	},
};

export const InfoSubtle: Story = {
	args: {
		severity: "info",
		prominent: false,
	},
};

export const WithoutTroubleshootingURL: Story = {
	args: {
		severity: "warning",
		prominent: true,
		troubleshootingURL: undefined,
	},
};
