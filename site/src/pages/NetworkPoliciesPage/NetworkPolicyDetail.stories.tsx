import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, spyOn, userEvent, within } from "storybook/test";
import { API } from "#/api/api";
import { networkPolicyRunLogsKey } from "#/api/queries/networkPolicies";
import type {
	NetworkPolicy,
	NetworkPolicyRevision,
	NetworkPolicyRun,
	NetworkPolicyTarget,
} from "#/api/typesGenerated";
import { MockTemplate } from "#/testHelpers/entities";
import {
	MockNetworkPolicy,
	MockNetworkPolicyLogs,
	MockNetworkPolicyRevision,
	MockNetworkPolicyRun,
	MockNetworkPolicyTarget,
} from "#/testHelpers/networkPolicies";
import { NetworkPolicyDetail } from "./NetworkPolicyDetail";

const mockDriftedTarget: NetworkPolicyTarget = {
	...MockNetworkPolicyTarget,
	drifted: true,
};
const mockDriftedPolicy: NetworkPolicy = {
	...MockNetworkPolicy,
	targets: [mockDriftedTarget],
};
const mockUnknownTarget: NetworkPolicyTarget = {
	...MockNetworkPolicyTarget,
	status: "unknown",
	drifted: true,
	last_error: "The worker stopped before the apply outcome could be confirmed.",
};
const mockUnknownPolicy: NetworkPolicy = {
	...MockNetworkPolicy,
	targets: [mockUnknownTarget],
};
const mockPolicyWithoutTargets: NetworkPolicy = {
	...MockNetworkPolicy,
	targets: [],
	runs: [],
};
const mockPendingTarget: NetworkPolicyTarget = {
	...MockNetworkPolicyTarget,
	status: "pending",
};
const mockPendingRun: NetworkPolicyRun = {
	...MockNetworkPolicyRun,
	status: "pending",
	plan: "",
	completed_at: null,
};
const mockPendingPolicy: NetworkPolicy = {
	...MockNetworkPolicy,
	targets: [mockPendingTarget],
	runs: [mockPendingRun],
};
const mockFailedTarget: NetworkPolicyTarget = {
	...MockNetworkPolicyTarget,
	status: "failed",
	last_error: "No provisioner matched the target tags.",
};
const mockFailedRun: NetworkPolicyRun = {
	...MockNetworkPolicyRun,
	status: "failed",
	error: "No provisioner matched the target tags.",
	plan: "",
};
const mockFailedPolicy: NetworkPolicy = {
	...MockNetworkPolicy,
	targets: [mockFailedTarget],
	runs: [mockFailedRun],
};
const mockNewerRevision: NetworkPolicyRevision = {
	...MockNetworkPolicyRevision,
	id: "80000000-0000-4000-8000-000000000002",
	created_at: "2026-10-21T12:00:00Z",
};
const mockPolicyWithHistory: NetworkPolicy = {
	...MockNetworkPolicy,
	revisions: [MockNetworkPolicyRevision, mockNewerRevision],
};

const meta = {
	title: "pages/NetworkPolicies/Detail",
	component: NetworkPolicyDetail,
	// Open log viewers poll after the initial query-cache seed.
	beforeEach: () => {
		spyOn(API.experimental, "getNetworkPolicyRunLogs").mockResolvedValue(
			MockNetworkPolicyLogs,
		);
	},
	args: {
		organization: "default",
		policy: MockNetworkPolicy,
		templates: [MockTemplate],
	},
} satisfies Meta<typeof NetworkPolicyDetail>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Default: Story = {};
export const DriftDetected: Story = {
	args: { policy: mockDriftedPolicy },
};
export const Mobile: Story = {
	globals: { viewport: { value: "iphone12", isRotated: false } },
};
export const DetachConfirmation: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", {
				name: `Remove ${MockTemplate.display_name}`,
			}),
		);
		await userEvent.click(
			canvas.getByRole("button", { name: "Save attachments" }),
		);
	},
};
export const UnknownOutcome: Story = {
	args: { policy: mockUnknownPolicy },
};
export const NoTargets: Story = {
	args: { policy: mockPolicyWithoutTargets },
};
export const Pending: Story = {
	args: { policy: mockPendingPolicy },
};
export const Failed: Story = {
	args: { policy: mockFailedPolicy },
};
export const PlanReport: Story = {
	parameters: {
		queries: [
			{
				key: networkPolicyRunLogsKey(
					"default",
					MockNetworkPolicy.id,
					MockNetworkPolicyRun.id,
				),
				data: MockNetworkPolicyLogs,
			},
		],
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByText(
				`plan on ${MockNetworkPolicyTarget.name}`,
			),
		);
	},
};
export const FailedRunReport: Story = {
	args: { policy: mockFailedPolicy },
	parameters: {
		queries: [
			{
				key: networkPolicyRunLogsKey(
					"default",
					MockNetworkPolicy.id,
					mockFailedRun.id,
				),
				data: MockNetworkPolicyLogs,
			},
		],
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByText(
				`plan on ${MockNetworkPolicyTarget.name}`,
			),
		);
	},
};
export const UnsavedChanges: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.type(
			await within(canvasElement).findByRole("textbox", { name: "Policy HCL" }),
			"\n# Draft changes",
		);
	},
};
export const PublishConfirmation: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("checkbox", {
				name: `Select ${MockNetworkPolicyTarget.name}`,
			}),
		);
		await userEvent.click(
			canvas.getByRole("button", { name: "Publish selected version" }),
		);
	},
};
export const VersionSelection: Story = {
	args: { policy: mockPolicyWithHistory },
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("combobox", { name: "Version" }),
		);
	},
};
export const HistoricalVersion: Story = {
	args: VersionSelection.args,
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("combobox", { name: "Version" }),
		);
		await userEvent.click(
			await screen.findByRole("option", {
				name: `Version 1 (${MockNetworkPolicyRevision.id.slice(0, 8)})`,
			}),
		);
	},
};
