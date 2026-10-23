import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { mockApiError } from "#/testHelpers/entities";
import { MockTemplateResourceLog } from "#/testHelpers/templateResources";
import { ProvisionerJobLogs } from "./ProvisionerJobLogs";

const meta = {
	title: "components/ProvisionerJobLogs",
	component: ProvisionerJobLogs,
	args: {
		logs: undefined,
		error: undefined,
		isLoading: false,
		onRetry: fn(),
		emptyMessage: "No job logs yet.",
	},
} satisfies Meta<typeof ProvisionerJobLogs>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Loading: Story = { args: { isLoading: true } };
export const LoadError: Story = {
	args: { error: mockApiError({ message: "Unable to load job logs." }) },
};
export const RefetchError: Story = {
	args: {
		logs: [MockTemplateResourceLog],
		error: mockApiError({
			message: "Unable to refresh job logs. Showing retained output.",
		}),
	},
};
