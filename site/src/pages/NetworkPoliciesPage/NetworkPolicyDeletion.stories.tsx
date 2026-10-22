import type { Meta, StoryObj } from "@storybook/react-vite";
import type {
	NetworkPolicy,
	NetworkPolicyRun,
	NetworkPolicyTarget,
} from "#/api/typesGenerated";
import { MockTemplate } from "#/testHelpers/entities";
import {
	MockNetworkPolicy,
	MockNetworkPolicyRun,
	MockNetworkPolicyTarget,
} from "#/testHelpers/networkPolicies";
import { NetworkPolicyDetail } from "./NetworkPolicyDetail";

const mockRunningTarget: NetworkPolicyTarget = {
	...MockNetworkPolicyTarget,
	status: "running",
};
const mockRunningDestroyRun: NetworkPolicyRun = {
	...MockNetworkPolicyRun,
	operation: "destroy",
	status: "running",
	completed_at: null,
};
const mockDeletingPolicy: NetworkPolicy = {
	...MockNetworkPolicy,
	deletion_requested_at: "2026-10-21T12:00:00Z",
	targets: [mockRunningTarget],
	runs: [mockRunningDestroyRun],
};
const mockFailedTarget: NetworkPolicyTarget = {
	...MockNetworkPolicyTarget,
	status: "failed",
	last_error: "Terraform initialization failed. No resources were changed.",
};
const mockFailedDestroyRun: NetworkPolicyRun = {
	...MockNetworkPolicyRun,
	operation: "destroy",
	status: "failed",
	error: mockFailedTarget.last_error,
};
const mockFailedDeletionPolicy: NetworkPolicy = {
	...mockDeletingPolicy,
	targets: [mockFailedTarget],
	runs: [mockFailedDestroyRun],
};
const mockUnknownTarget: NetworkPolicyTarget = {
	...MockNetworkPolicyTarget,
	status: "unknown",
	last_error:
		"Terraform cleanup outcome is unknown. The target is quarantined.",
};
const mockUnknownDestroyRun: NetworkPolicyRun = {
	...MockNetworkPolicyRun,
	operation: "destroy",
	status: "failed",
	error: mockUnknownTarget.last_error,
};
const mockUnknownDeletionPolicy: NetworkPolicy = {
	...mockDeletingPolicy,
	targets: [mockUnknownTarget],
	runs: [mockUnknownDestroyRun],
};
const mockDestroyedTarget: NetworkPolicyTarget = {
	...MockNetworkPolicyTarget,
	status: "succeeded",
	destroyed_at: "2026-10-21T12:05:00Z",
};
const mockCompletedDestroyRun: NetworkPolicyRun = {
	...MockNetworkPolicyRun,
	operation: "destroy",
	status: "succeeded",
	plan: "Plan: 0 to add, 0 to change, 1 to destroy.",
};
const mockDeletedPolicy: NetworkPolicy = {
	...mockDeletingPolicy,
	deleted_at: "2026-10-21T12:05:00Z",
	targets: [mockDestroyedTarget],
	runs: [mockCompletedDestroyRun],
};

const meta = {
	title: "pages/NetworkPolicies/Deletion",
	component: NetworkPolicyDetail,
	args: {
		organization: "default",
		policy: mockDeletingPolicy,
		templates: [MockTemplate],
	},
} satisfies Meta<typeof NetworkPolicyDetail>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Running: Story = {};
export const Failed: Story = { args: { policy: mockFailedDeletionPolicy } };
export const Unknown: Story = { args: { policy: mockUnknownDeletionPolicy } };
export const Deleted: Story = { args: { policy: mockDeletedPolicy } };
