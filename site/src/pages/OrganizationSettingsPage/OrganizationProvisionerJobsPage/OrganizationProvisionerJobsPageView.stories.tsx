import type { Meta, StoryObj } from "@storybook/react";
import { expect, fn, userEvent, waitFor, within } from "@storybook/test";
import type { ProvisionerJob } from "api/typesGenerated";
import { MockOrganization, MockProvisionerJob } from "testHelpers/entities";
import { daysAgo } from "utils/time";
import OrganizationProvisionerJobsPageView from "./OrganizationProvisionerJobsPageView";

const MockProvisionerJobs: ProvisionerJob[] = Array.from(
	{ length: 50 },
	(_, i) => ({
		...MockProvisionerJob,
		id: i.toString(),
		created_at: daysAgo(2),
	}),
);

const meta: Meta<typeof OrganizationProvisionerJobsPageView> = {
	title: "pages/OrganizationProvisionerJobsPage",
	component: OrganizationProvisionerJobsPageView,
	args: {
		organization: MockOrganization,
		jobs: MockProvisionerJobs,
		onRetry: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof OrganizationProvisionerJobsPageView>;

export const Default: Story = {};

export const OrganizationNotFound: Story = {
	args: {
		organization: undefined,
	},
};

export const Loading: Story = {
	args: {
		jobs: undefined,
	},
};

export const LoadingError: Story = {
	args: {
		jobs: undefined,
		error: new Error("Failed to load jobs"),
	},
};

export const RetryAfterError: Story = {
	args: {
		jobs: undefined,
		error: new Error("Failed to load jobs"),
		onRetry: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const retryButton = await canvas.findByRole("button", { name: "Retry" });
		userEvent.click(retryButton);

		await waitFor(() => {
			expect(args.onRetry).toHaveBeenCalled();
		});
	},
	parameters: {
		chromatic: {
			disableSnapshot: true,
		},
	},
};

export const Empty: Story = {
	args: {
		jobs: [],
	},
};
