import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, userEvent, within } from "storybook/test";
import {
	MockNetworkPolicy,
	MockNetworkPolicyTarget,
} from "#/testHelpers/networkPolicies";
import { NetworkPolicyTargetForm } from "./NetworkPolicyTargetForm";

const meta = {
	title: "pages/NetworkPolicies/TargetForm",
	component: NetworkPolicyTargetForm,
	args: {
		generation: MockNetworkPolicy.generation,
		isPending: false,
		error: undefined,
		onSubmit: fn(),
	},
} satisfies Meta<typeof NetworkPolicyTargetForm>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const InvalidProvisionerTags: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.type(
			canvas.getByRole("textbox", { name: /^Cluster/ }),
			MockNetworkPolicyTarget.cluster,
		);
		await userEvent.type(
			canvas.getByRole("textbox", { name: /^Kubernetes context/ }),
			MockNetworkPolicyTarget.context,
		);
		await userEvent.type(
			canvas.getByRole("textbox", { name: /^Namespace/ }),
			MockNetworkPolicyTarget.namespace,
		);
		await userEvent.type(
			canvas.getByRole("textbox", { name: "Provisioner tags" }),
			"region=eu-west-1\nregion=us-east-1",
		);
		await userEvent.click(canvas.getByRole("button", { name: "Add target" }));
	},
};
export const RequestError: Story = {
	args: {
		error: {
			message: "Unable to add target.",
			detail: "The policy generation changed. Reload before adding a target.",
		},
	},
};
