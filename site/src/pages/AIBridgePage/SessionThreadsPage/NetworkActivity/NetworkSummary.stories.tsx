import type { Meta, StoryObj } from "@storybook/react-vite";
import { mockNetworkActivity } from "./mocks";
import { NetworkSummary } from "./NetworkSummary";

const meta: Meta<typeof NetworkSummary> = {
	title: "pages/AIBridgePage/NetworkActivity/Summary",
	component: NetworkSummary,
	// The summary is laid out as a row inside a definition list. Wrap it in a
	// minimal <dl> so it renders with realistic typography in isolation.
	decorators: [
		(Story) => (
			<dl className="m-0 w-64 text-sm text-content-secondary border border-solid rounded-md p-3 flex flex-col gap-2">
				<Story />
			</dl>
		),
	],
};

export default meta;
type Story = StoryObj<typeof NetworkSummary>;

export const NoActivity: Story = {
	args: { networkActivity: mockNetworkActivity("none") },
};

export const AllAllowed: Story = {
	args: { networkActivity: mockNetworkActivity("all-allowed") },
};

export const Mixed: Story = {
	args: { networkActivity: mockNetworkActivity("mixed") },
};

export const ErrorOnly: Story = {
	args: { networkActivity: mockNetworkActivity("error-only") },
};

export const MidSessionFailure: Story = {
	args: { networkActivity: mockNetworkActivity("mid-session-failure") },
};

export const Many: Story = {
	args: { networkActivity: mockNetworkActivity("many") },
};
