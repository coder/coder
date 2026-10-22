import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { fn, screen, userEvent, within } from "storybook/test";
import type { NetworkPolicy, NetworkPolicyTarget } from "#/api/typesGenerated";
import {
	MockNetworkPolicy,
	MockNetworkPolicyRevision,
	MockNetworkPolicyTarget,
} from "#/testHelpers/networkPolicies";
import { NetworkPolicyPublications } from "./NetworkPolicyPublications";

const mockRunningTarget: NetworkPolicyTarget = {
	...MockNetworkPolicyTarget,
	status: "running",
};
const mockBusyPolicy: NetworkPolicy = {
	...MockNetworkPolicy,
	targets: [mockRunningTarget],
};

const meta = {
	title: "pages/NetworkPolicies/Publications",
	component: NetworkPolicyPublications,
	args: {
		policy: MockNetworkPolicy,
		selectedRevisionId: MockNetworkPolicyRevision.id,
		isPending: false,
		error: undefined,
		onPublish: fn(),
	},
} satisfies Meta<typeof NetworkPolicyPublications>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
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
export const PreviewConfirmation: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("checkbox", {
				name: `Select ${MockNetworkPolicyTarget.name}`,
			}),
		);
		await userEvent.click(
			canvas.getByRole("button", { name: "Preview plan / drift" }),
		);
	},
};
export const UnavailableTargetConfirmation: Story = {
	render: function Render(args) {
		const [policy, setPolicy] = useState(args.policy);
		return (
			<NetworkPolicyPublications
				{...args}
				policy={policy}
				onPublish={() => setPolicy(mockBusyPolicy)}
			/>
		);
	},
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
		await userEvent.click(
			within(await screen.findByRole("dialog")).getByRole("button", {
				name: "Queue operation",
			}),
		);
	},
};
export const ConfirmationError: Story = {
	args: {
		error: {
			message: "Unable to publish selected version.",
			detail: "The policy changed. Reload before queuing an operation.",
		},
	},
	play: PublishConfirmation.play,
};
