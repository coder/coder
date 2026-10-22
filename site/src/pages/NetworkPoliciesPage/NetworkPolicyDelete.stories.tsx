import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { fn, screen, userEvent, within } from "storybook/test";
import { MockNetworkPolicy } from "#/testHelpers/networkPolicies";
import { NetworkPolicyDelete } from "./NetworkPolicyDelete";

const meta = {
	title: "pages/NetworkPolicies/Delete",
	component: NetworkPolicyDelete,
	args: {
		name: MockNetworkPolicy.name,
		generation: MockNetworkPolicy.generation,
		isPending: false,
		deleted: false,
		blockedReason: undefined,
		error: undefined,
		onDelete: fn(),
	},
} satisfies Meta<typeof NetworkPolicyDelete>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Confirmation: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Delete policy" }),
		);
	},
};
export const Quarantined: Story = {
	args: {
		blockedReason:
			"Deletion is blocked because a previous Terraform operation has an unknown outcome. Recovery is not implemented in this POC. The policy and logs are retained to avoid losing track of managed resources.",
	},
};
export const Pending: Story = {
	render: function Render(args) {
		const [isPending, setPending] = useState(false);
		return (
			<NetworkPolicyDelete
				{...args}
				isPending={isPending}
				onDelete={() => setPending(true)}
			/>
		);
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Delete policy" }),
		);
		await userEvent.click(
			within(await screen.findByRole("dialog")).getByRole("button", {
				name: "Delete",
			}),
		);
	},
};
export const StaleConfirmation: Story = {
	render: function Render(args) {
		const [generation, setGeneration] = useState(args.generation);
		return (
			<NetworkPolicyDelete
				{...args}
				generation={generation}
				onDelete={() => setGeneration(generation + 1)}
			/>
		);
	},
	play: Pending.play,
};
export const NewlyQuarantinedConfirmation: Story = {
	render: function Render(args) {
		const [blockedReason, setBlockedReason] = useState<string>();
		return (
			<NetworkPolicyDelete
				{...args}
				blockedReason={blockedReason}
				onDelete={() => setBlockedReason(Quarantined.args?.blockedReason)}
			/>
		);
	},
	play: Pending.play,
};
export const DeleteError: Story = {
	args: { error: { message: "The policy changed. Reload before deleting." } },
	play: Confirmation.play,
};
export const Mobile: Story = {
	globals: { viewport: { value: "iphone12", isRotated: false } },
	play: Confirmation.play,
};
