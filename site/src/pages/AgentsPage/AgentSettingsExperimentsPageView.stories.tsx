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
};

const meta = {
	title: "pages/AgentsPage/AgentSettingsExperimentsPageView",
	component: AgentSettingsExperimentsPageView,
	args: baseArgs,
} satisfies Meta<typeof AgentSettingsExperimentsPageView>;

export default meta;
type Story = StoryObj<typeof AgentSettingsExperimentsPageView>;

export const Default: Story = {};

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
