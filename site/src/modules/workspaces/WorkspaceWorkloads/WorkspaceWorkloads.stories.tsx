import type { Meta, StoryObj } from "@storybook/react-vite";
import {
	MockStartingWorkload,
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
			MockStartingWorkload,
			{
				...MockWorkload,
				id: "unknown",
				name: "nightly-triage",
				status: "unknown",
				command:
					"set -eu\ngit fetch origin\n./scripts/triage.sh --since yesterday --label needs-triage --assign-to-oncall",
				identity:
					"test-workspace/workload/1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d",
				scopes: [],
			},
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
