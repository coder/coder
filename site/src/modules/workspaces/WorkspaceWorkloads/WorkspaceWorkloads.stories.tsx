import type { Meta, StoryObj } from "@storybook/react-vite";
import {
	MockPendingWorkload,
	MockWorkload,
	mockApiError,
} from "#/testHelpers/entities";
import { WorkspaceWorkloads } from "./WorkspaceWorkloads";

const meta: Meta<typeof WorkspaceWorkloads> = {
	title: "modules/workspaces/WorkspaceWorkloads",
	component: WorkspaceWorkloads,
	args: {
		workspaceName: "dev",
		workloads: [MockWorkload],
		error: undefined,
	},
};

export default meta;
type Story = StoryObj<typeof WorkspaceWorkloads>;

export const Running: Story = {};

export const EveryStatus: Story = {
	args: {
		workloads: [
			MockWorkload,
			{
				...MockWorkload,
				id: "starting",
				name: "write-docs",
				status: "starting",
				command: undefined,
				identity: "test-workspace/pod/0f9e8d7c-6b5a-4c3d-2e1f-0a9b8c7d6e5f",
			},
			{
				...MockWorkload,
				id: "disconnected",
				name: "nightly-triage",
				status: "disconnected",
				command:
					"set -eu\ngit fetch origin\n./scripts/triage.sh --since yesterday --label needs-triage --assign-to-oncall",
				identity: "test-workspace/pod/1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d",
			},
			MockPendingWorkload,
		],
	},
};

export const Empty: Story = {
	args: { workloads: [] },
};

export const Loading: Story = {
	args: { workloads: undefined },
};

export const Failed: Story = {
	args: {
		workloads: undefined,
		error: mockApiError({
			message:
				"Runtime workloads functionality requires enabling the 'runtime-workloads' experiment.",
		}),
	},
};

export const FailedRefetch: Story = {
	args: {
		error: mockApiError({ message: "Internal error fetching workloads." }),
	},
};
