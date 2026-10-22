import type { Meta, StoryObj } from "@storybook/react-vite";
import { spyOn, within } from "storybook/test";
import { API } from "#/api/api";
import { networkPolicyRunLogsKey } from "#/api/queries/networkPolicies";
import {
	MockNetworkPolicy,
	MockNetworkPolicyLogs,
	MockNetworkPolicyRun,
} from "#/testHelpers/networkPolicies";
import { NetworkPolicyRunLogs } from "./NetworkPolicyRunLogs";

const meta = {
	title: "pages/NetworkPolicies/RunLogs",
	component: NetworkPolicyRunLogs,
	// Polling continues after the initial query-cache seed.
	beforeEach: () => {
		spyOn(API.experimental, "getNetworkPolicyRunLogs").mockResolvedValue(
			MockNetworkPolicyLogs,
		);
	},
	args: {
		organization: "default",
		policyId: MockNetworkPolicy.id,
		runId: MockNetworkPolicyRun.id,
	},
} satisfies Meta<typeof NetworkPolicyRunLogs>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Default: Story = {
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
};
export const Empty: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getNetworkPolicyRunLogs").mockResolvedValue([]);
	},
	parameters: {
		queries: [
			{
				key: networkPolicyRunLogsKey(
					"default",
					MockNetworkPolicy.id,
					MockNetworkPolicyRun.id,
				),
				data: [],
			},
		],
	},
};
export const Loading: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getNetworkPolicyRunLogs").mockImplementation(
			() => new Promise(() => {}),
		);
	},
};
export const RefetchError: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getNetworkPolicyRunLogs").mockRejectedValue({
			message: "Unable to refresh provisioner logs.",
			detail:
				"Previously loaded output is still available. Retry to refresh it.",
		});
	},
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
		await within(canvasElement).findByRole(
			"button",
			{ name: "Retry logs" },
			{ timeout: 10000 },
		);
	},
};
export const LoadError: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getNetworkPolicyRunLogs").mockRejectedValue({
			message: "Unable to retrieve provisioner logs.",
			detail: "Retry to request persisted output again.",
		});
	},
};
