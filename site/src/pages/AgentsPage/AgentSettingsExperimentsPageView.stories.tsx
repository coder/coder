import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import {
	AgentSettingsExperimentsPageView,
	type AgentSettingsExperimentsPageViewProps,
} from "./AgentSettingsExperimentsPageView";

const baseArgs: AgentSettingsExperimentsPageViewProps = {
	desktopEnabledData: { enable_desktop: false },
	onSaveDesktopEnabled: fn(),
	isSavingDesktopEnabled: false,
	isSaveDesktopEnabledError: false,
	debugLoggingData: {
		allow_users: false,
		forced_by_deployment: false,
	},
	onSaveDebugLogging: fn(),
	isSavingDebugLogging: false,
	isSaveDebugLoggingError: false,
};

const meta = {
	title: "pages/AgentsPage/AgentSettingsExperimentsPageView",
	component: AgentSettingsExperimentsPageView,
	args: baseArgs,
} satisfies Meta<typeof AgentSettingsExperimentsPageView>;

export default meta;
type Story = StoryObj<typeof AgentSettingsExperimentsPageView>;

export const AllowUsersOff: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const toggle = await canvas.findByRole("switch", {
			name: "Allow users to enable chat debug logging",
		});

		expect(
			await canvas.findByText("Let users record chat debug logs"),
		).toBeInTheDocument();
		expect(toggle).not.toBeChecked();
	},
};

export const AllowUsersOn: Story = {
	args: {
		debugLoggingData: {
			allow_users: true,
			forced_by_deployment: false,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const toggle = await canvas.findByRole("switch", {
			name: "Allow users to enable chat debug logging",
		});

		expect(toggle).toBeChecked();
	},
};

export const ForcedByDeployment: Story = {
	args: {
		debugLoggingData: {
			allow_users: true,
			forced_by_deployment: true,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const toggle = await canvas.findByRole("switch", {
			name: "Allow users to enable chat debug logging",
		});

		expect(toggle).toBeDisabled();
		expect(
			await canvas.findByText(
				/Debug logging is already enabled deployment-wide/i,
			),
		).toBeInTheDocument();
	},
};

export const DesktopSetting: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByText("Virtual Desktop");
		await canvas.findByText(
			/Allow agents to use a virtual, graphical desktop within workspaces./i,
		);
		await canvas.findByRole("switch", { name: "Enable" });
	},
};

export const TogglesDesktop: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const toggle = await canvas.findByRole("switch", { name: "Enable" });

		await userEvent.click(toggle);
		await waitFor(() => {
			expect(args.onSaveDesktopEnabled).toHaveBeenCalledWith({
				enable_desktop: true,
			});
		});
	},
};
