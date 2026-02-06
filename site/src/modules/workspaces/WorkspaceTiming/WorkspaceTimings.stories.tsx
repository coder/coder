import { chromatic } from "testHelpers/chromatic";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { WorkspaceTimingsResponse } from "./storybookData";
import { WorkspaceTimings } from "./WorkspaceTimings";

const meta: Meta<typeof WorkspaceTimings> = {
	title: "modules/workspaces/WorkspaceTimings",
	component: WorkspaceTimings,
	args: {
		defaultIsOpen: true,
		provisionerTimings: WorkspaceTimingsResponse.provisioner_timings,
		agentScriptTimings: WorkspaceTimingsResponse.agent_script_timings,
		agentConnectionTimings: WorkspaceTimingsResponse.agent_connection_timings,
	},
	parameters: {
		chromatic,
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
		agentConnectionTimings: undefined,
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
		await user.click(canvas.getByText("Build timeline", { exact: false }));
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
		await user.click(canvas.getByText("Build timeline", { exact: false }));
		await waitFor(() =>
			expect(canvas.queryByText("workspace boot")).not.toBeInTheDocument(),
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
			name: "View run startup scripts details",
		});
		await user.click(detailsButton);
		await canvas.findByText("Startup Script");
	},
};

// Test case for https://github.com/coder/coder/issues/15413
export const DuplicatedScriptTiming: Story = {
	args: {
		agentScriptTimings: [
			WorkspaceTimingsResponse.agent_script_timings[0],
			{
				...WorkspaceTimingsResponse.agent_script_timings[0],
				started_at: "2021-09-01T00:00:00Z",
				ended_at: "2021-09-01T00:00:00Z",
			},
		],
	},
};

// Loading when agent script timings are empty
// Test case for https://github.com/coder/coder/issues/15273
export const LoadingWhenAgentScriptTimingsAreEmpty: Story = {
	args: {
		agentScriptTimings: undefined,
	},
};

export const LongTimeRange = {
	args: {
		provisionerTimings: [
			{
				...WorkspaceTimingsResponse.provisioner_timings[0],
				started_at: "2021-09-01T00:00:00Z",
				ended_at: "2021-09-01T00:10:00Z",
			},
		],
		agentConnectionTimings: [
			{
				...WorkspaceTimingsResponse.agent_connection_timings[0],
				started_at: "2021-09-01T00:10:00Z",
				ended_at: "2021-09-01T00:35:00Z",
			},
		],
		agentScriptTimings: [
			{
				...WorkspaceTimingsResponse.agent_script_timings[0],
				started_at: "2021-09-01T00:35:00Z",
				ended_at: "2021-09-01T01:00:00Z",
			},
		],
	},
};

// We want to gracefully handle the case when the action is added in the BE but
// not in the FE. This is a temporary fix until we can have strongly provisioner
// timing action types in the BE.
export const MissedAction: Story = {
	args: {
		agentConnectionTimings: [
			{
				ended_at: "2025-03-12T18:15:13.651163Z",
				stage: "connect",
				started_at: "2025-03-12T18:15:10.249068Z",
				workspace_agent_id: "41ab4fd4-44f8-4f3a-bb69-262ae85fba0b",
				workspace_agent_name: "Interface",
			},
		],
		agentScriptTimings: [
			{
				display_name: "Startup Script",
				ended_at: "2025-03-12T18:16:44.771508Z",
				exit_code: 0,
				stage: "start",
				started_at: "2025-03-12T18:15:13.847336Z",
				status: "ok",
				workspace_agent_id: "41ab4fd4-44f8-4f3a-bb69-262ae85fba0b",
				workspace_agent_name: "Interface",
			},
		],
		provisionerTimings: [
			{
				action: "create",
				ended_at: "2025-03-12T18:08:07.402358Z",
				job_id: "a7c4a05d-1c36-4264-8275-8107c93c5fc8",
				resource: "coder_agent.Interface",
				source: "coder",
				stage: "apply",
				started_at: "2025-03-12T18:08:07.194957Z",
			},
			{
				action: "create",
				ended_at: "2025-03-12T18:08:08.029908Z",
				job_id: "a7c4a05d-1c36-4264-8275-8107c93c5fc8",
				resource: "null_resource.validate_url",
				source: "null",
				stage: "apply",
				started_at: "2025-03-12T18:08:07.399387Z",
			},
			{
				action: "create",
				ended_at: "2025-03-12T18:08:07.440785Z",
				job_id: "a7c4a05d-1c36-4264-8275-8107c93c5fc8",
				resource: "module.emu_host.random_id.emulator_host_id",
				source: "random",
				stage: "apply",
				started_at: "2025-03-12T18:08:07.403171Z",
			},
			{
				action: "missed action",
				ended_at: "2025-03-12T18:08:08.029752Z",
				job_id: "a7c4a05d-1c36-4264-8275-8107c93c5fc8",
				resource: "null_resource.validate_url",
				source: "null",
				stage: "apply",
				started_at: "2025-03-12T18:08:07.410219Z",
			},
		],
	},
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const applyButton = canvas.getByRole("button", {
			name: "View apply details",
		});
		await user.click(applyButton);
		await canvas.findByText("missed action");
	},
};

// Ref: #15432
export const InvalidTimeRange: Story = {
	args: {
		provisionerTimings: [
			{
				...WorkspaceTimingsResponse.provisioner_timings[0],
				stage: "init",
				started_at: "2025-01-01T00:00:00Z",
				ended_at: "2025-01-01T00:01:00Z",
			},
			{
				...WorkspaceTimingsResponse.provisioner_timings[0],
				stage: "plan",
				started_at: "2025-01-01T00:01:00Z",
				ended_at: "0001-01-01T00:00:00Z",
			},
			{
				...WorkspaceTimingsResponse.provisioner_timings[0],
				stage: "graph",
				started_at: "0001-01-01T00:00:00Z",
				ended_at: "2025-01-01T00:03:00Z",
			},
			{
				...WorkspaceTimingsResponse.provisioner_timings[0],
				stage: "apply",
				started_at: "2025-01-01T00:03:00Z",
				ended_at: "2025-01-01T00:04:00Z",
			},
		],
		agentConnectionTimings: [
			{
				started_at: "2025-01-01T00:05:00Z",
				ended_at: "2025-01-01T00:06:00Z",
				stage: "connect",
				workspace_agent_id: "67e37a9d-ccac-497e-8f48-4093bcc4f3e7",
				workspace_agent_name: "main",
			},
		],
		agentScriptTimings: [
			{
				...WorkspaceTimingsResponse.agent_script_timings[0],
				display_name: "Startup Script 1",
				started_at: "0001-01-01T00:00:00Z",
				ended_at: "2025-01-01T00:10:00Z",
				workspace_agent_id: "67e37a9d-ccac-497e-8f48-4093bcc4f3e7",
				workspace_agent_name: "main",
			},
		],
	},
};

// Test case for multiple agents (e.g., main + devcontainer) where each agent
// should only show its own timings, not duplicated across all agents.
export const MultipleAgents: Story = {
	decorators: [
		(Story) => (
			<div css={{ "--collapse-body-height": "600px" }}>
				<Story />
			</div>
		),
	],
	args: {
		provisionerTimings: [
			{
				job_id: "fb0a0941-5052-4f8b-8046-64da139220cd",
				started_at: "2026-02-02T08:34:16.067798Z",
				ended_at: "2026-02-02T08:34:16.626888Z",
				stage: "init",
				source: "coder",
				action: "terraform",
				resource: "coder_stage_init",
			},
			{
				job_id: "fb0a0941-5052-4f8b-8046-64da139220cd",
				started_at: "2026-02-02T08:34:16.649547Z",
				ended_at: "2026-02-02T08:34:18.65871Z",
				stage: "plan",
				source: "coder",
				action: "terraform",
				resource: "coder_stage_plan",
			},
			{
				job_id: "fb0a0941-5052-4f8b-8046-64da139220cd",
				started_at: "2026-02-02T08:34:18.722631Z",
				ended_at: "2026-02-02T08:34:21.332458Z",
				stage: "apply",
				source: "coder",
				action: "terraform",
				resource: "coder_stage_apply",
			},
			{
				job_id: "fb0a0941-5052-4f8b-8046-64da139220cd",
				started_at: "2026-02-02T08:34:21.698283Z",
				ended_at: "2026-02-02T08:34:22.014735Z",
				stage: "graph",
				source: "coder",
				action: "terraform",
				resource: "coder_stage_graph",
			},
		],
		agentConnectionTimings: [
			{
				started_at: "2026-02-02T08:34:22.092544Z",
				ended_at: "2026-02-02T08:34:23.090936Z",
				stage: "connect",
				workspace_agent_id: "a1d50955-a671-4e6c-8e0e-6bc938e931bf",
				workspace_agent_name: "dev",
			},
			{
				started_at: "2026-02-02T08:34:50.171921Z",
				ended_at: "2026-02-02T08:34:51.36274Z",
				stage: "connect",
				workspace_agent_id: "afbdd368-b7b8-453e-af5a-02b13bc45553",
				workspace_agent_name: "coder",
			},
		],
		agentScriptTimings: [
			{
				started_at: "2026-02-02T08:34:23.745887Z",
				ended_at: "2026-02-02T08:34:25.23973Z",
				exit_code: 0,
				stage: "start",
				status: "ok",
				display_name: "Installing Dependencies",
				workspace_agent_id: "a1d50955-a671-4e6c-8e0e-6bc938e931bf",
				workspace_agent_name: "dev",
			},
			{
				started_at: "2026-02-02T08:34:23.743853Z",
				ended_at: "2026-02-02T08:34:23.809943Z",
				exit_code: 0,
				stage: "start",
				status: "ok",
				display_name: "Git Clone",
				workspace_agent_id: "a1d50955-a671-4e6c-8e0e-6bc938e931bf",
				workspace_agent_name: "dev",
			},
			{
				started_at: "2026-02-02T08:34:23.74382Z",
				ended_at: "2026-02-02T08:34:40.822488Z",
				exit_code: 0,
				stage: "start",
				status: "ok",
				display_name: "code-server",
				workspace_agent_id: "a1d50955-a671-4e6c-8e0e-6bc938e931bf",
				workspace_agent_name: "dev",
			},
			// Same display_name as dev agent to test dedup scoping by agent ID.
			{
				started_at: "2026-02-02T08:34:51.5Z",
				ended_at: "2026-02-02T08:34:53.2Z",
				exit_code: 0,
				stage: "start",
				status: "ok",
				display_name: "Installing Dependencies",
				workspace_agent_id: "afbdd368-b7b8-453e-af5a-02b13bc45553",
				workspace_agent_name: "coder",
			},
			{
				started_at: "2026-02-02T08:34:53.3Z",
				ended_at: "2026-02-02T08:34:55.1Z",
				exit_code: 0,
				stage: "start",
				status: "ok",
				display_name: "Personalize",
				workspace_agent_id: "afbdd368-b7b8-453e-af5a-02b13bc45553",
				workspace_agent_name: "coder",
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Verify both agents are shown
		await canvas.findByText("agent (dev)");
		await canvas.findByText("agent (coder)");
	},
};

// A template with no agent scripts.
export const NoAgentScripts: Story = {
	args: {
		provisionerTimings: [
			{
				...WorkspaceTimingsResponse.provisioner_timings[0],
				stage: "init",
				started_at: "2025-01-01T00:00:00Z",
				ended_at: "2025-01-01T00:01:00Z",
			},
			{
				...WorkspaceTimingsResponse.provisioner_timings[0],
				stage: "plan",
				started_at: "2025-01-01T00:01:00Z",
				ended_at: "0001-01-01T00:00:00Z",
			},
			{
				...WorkspaceTimingsResponse.provisioner_timings[0],
				stage: "graph",
				started_at: "0001-01-01T00:00:00Z",
				ended_at: "2025-01-01T00:03:00Z",
			},
			{
				...WorkspaceTimingsResponse.provisioner_timings[0],
				stage: "apply",
				started_at: "2025-01-01T00:03:00Z",
				ended_at: "2025-01-01T00:04:00Z",
			},
		],
		agentConnectionTimings: [
			{
				started_at: "2025-01-01T00:05:00Z",
				ended_at: "2025-01-01T00:06:00Z",
				stage: "connect",
				workspace_agent_id: "67e37a9d-ccac-497e-8f48-4093bcc4f3e7",
				workspace_agent_name: "main",
			},
		],
		agentScriptTimings: [
			// No agent scripts in the template
		],
	},
};
