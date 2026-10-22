import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { fn, screen, userEvent, within } from "storybook/test";
import type { NetworkPolicy, NetworkPolicyTarget } from "#/api/typesGenerated";
import { MockTemplate } from "#/testHelpers/entities";
import {
	MockNetworkPolicy,
	MockNetworkPolicyTarget,
} from "#/testHelpers/networkPolicies";
import { NetworkPolicyAttachments } from "./NetworkPolicyAttachments";

const mockBusyTarget: NetworkPolicyTarget = {
	...MockNetworkPolicyTarget,
	status: "running",
};
const mockBusyPolicy: NetworkPolicy = {
	...MockNetworkPolicy,
	targets: [mockBusyTarget],
};

const meta = {
	title: "pages/NetworkPolicies/Attachments",
	component: NetworkPolicyAttachments,
	args: {
		policy: MockNetworkPolicy,
		templates: [MockTemplate],
		isPending: false,
		error: undefined,
		onSubmit: fn(),
	},
} satisfies Meta<typeof NetworkPolicyAttachments>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Dirty: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", {
				name: `Remove ${MockTemplate.display_name}`,
			}),
		);
		await userEvent.tab();
	},
};
export const MobileDirty: Story = {
	globals: { viewport: { value: "iphone12", isRotated: false } },
	play: Dirty.play,
};
export const ConfirmationError: Story = {
	args: {
		error: {
			message: "The policy changed. Reload before updating attachments.",
		},
	},
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
export const BlockedConfirmation: Story = {
	render: function Render(args) {
		const [policy, setPolicy] = useState(args.policy);
		return (
			<NetworkPolicyAttachments
				{...args}
				policy={policy}
				onSubmit={() => setPolicy(mockBusyPolicy)}
			/>
		);
	},
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
		await userEvent.click(
			within(await screen.findByRole("dialog")).getByRole("button", {
				name: "Update attachments",
			}),
		);
	},
};
export const DiscardFocus: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", {
				name: `Remove ${MockTemplate.display_name}`,
			}),
		);
		await userEvent.click(
			canvas.getByRole("button", { name: "Discard attachment changes" }),
		);
	},
};
export const UnavailableFixedAttachment: Story = {
	args: { templates: [] },
};
