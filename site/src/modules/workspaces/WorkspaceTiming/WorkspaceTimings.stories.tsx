import type { Meta, StoryObj } from "@storybook/react";
import { expect, userEvent, waitFor, within } from "@storybook/test";
import { WorkspaceTimings } from "./WorkspaceTimings";
import { WorkspaceTimingsResponse } from "./storybookData";

const meta: Meta<typeof WorkspaceTimings> = {
	title: "modules/workspaces/WorkspaceTimings",
	component: WorkspaceTimings,
	args: {
		defaultIsOpen: true,
		provisionerTimings: WorkspaceTimingsResponse.provisioner_timings,
		agentScriptTimings: WorkspaceTimingsResponse.agent_script_timings,
	},
};

export default meta;
type Story = StoryObj<typeof WorkspaceTimings>;

export const Open: Story = {};

export const Close: Story = {
	args: {
		defaultIsOpen: false,
	},
};

export const Loading: Story = {
	args: {
		provisionerTimings: undefined,
		agentScriptTimings: undefined,
	},
};

export const ClickToOpen: Story = {
	args: {
		defaultIsOpen: false,
	},
	parameters: {
		chromatic: { disableSnapshot: true },
	},
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		await user.click(canvas.getByRole("button"));
		await canvas.findByText("provisioning");
	},
};

export const ClickToClose: Story = {
	parameters: {
		chromatic: { disableSnapshot: true },
	},
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		await canvas.findByText("provisioning");
		await user.click(canvas.getByText("Provisioning time", { exact: false }));
		await waitFor(() =>
			expect(canvas.getByText("workspace boot")).not.toBeVisible(),
		);
	},
};

const [first, ...others] = WorkspaceTimingsResponse.agent_script_timings;
export const FailedScript: Story = {
	args: {
		agentScriptTimings: [
			{ ...first, status: "exit_failure", exit_code: 1 },
			...others,
		],
	},
};

// Navigate into a provisioning stage
export const NavigateToPlanStage: Story = {
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const detailsButton = canvas.getByRole("button", {
			name: "View plan details",
		});
		await user.click(detailsButton);
		await canvas.findByText(
			"module.dotfiles.data.coder_parameter.dotfiles_uri[0]",
		);
	},
};

// Navigating into a workspace boot stage
export const NavigateToStartStage: Story = {
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const detailsButton = canvas.getByRole("button", {
			name: "View start details",
		});
		await user.click(detailsButton);
		await canvas.findByText("Startup Script");
	},
};
