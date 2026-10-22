import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { MockNetworkPolicy } from "#/testHelpers/networkPolicies";
import { NetworkPoliciesPageView } from "./NetworkPoliciesPageView";

const mockDeletingPolicy = {
	...MockNetworkPolicy,
	deletion_requested_at: "2026-10-21T12:00:00Z",
};

const meta = {
	title: "pages/NetworkPolicies/List",
	component: NetworkPoliciesPageView,
	args: {
		policies: [MockNetworkPolicy],
		isLoading: false,
		error: undefined,
		onRetry: fn(),
		basePath: "/organizations/default/network-policies",
	},
} satisfies Meta<typeof NetworkPoliciesPageView>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Default: Story = {};
export const Deleting: Story = {
	args: { policies: [mockDeletingPolicy] },
};
export const Empty: Story = { args: { policies: [] } };
export const Loading: Story = {
	args: { policies: undefined, isLoading: true },
};
export const LoadError: Story = {
	args: {
		policies: undefined,
		error: { message: "Unable to load policies.", detail: "Please retry." },
	},
};
export const RefetchError: Story = {
	args: {
		error: {
			message: "Policy refresh failed.",
			detail: "Your last loaded policies are still shown.",
		},
	},
};
