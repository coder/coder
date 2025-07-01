import type { Meta, StoryObj } from "@storybook/react";
import { expect, fn, userEvent, waitFor, within } from "@storybook/test";
import type { ProvisionerJob } from "api/typesGenerated";
import { useState } from "react";
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
		filter: { status: "", ids: "" },
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

export const OnFilter: Story = {
	render: function FilterWithState({ ...args }) {
		const [jobs, setJobs] = useState<ProvisionerJob[]>([]);
		const [filter, setFilter] = useState({ status: "pending", ids: "" });
		const handleFilterChange = (newFilter: { status: string; ids: string }) => {
			setFilter(newFilter);
			const filteredJobs = MockProvisionerJobs.filter((job) =>
				newFilter.status ? job.status === newFilter.status : true,
			);
			setJobs(filteredJobs);
		};

		return (
			<OrganizationProvisionerJobsPageView
				{...args}
				filter={filter}
				jobs={jobs}
				onFilterChange={handleFilterChange}
			/>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const statusFilter = canvas.getByTestId("status-filter");
		await userEvent.click(statusFilter);

		const body = within(canvasElement.ownerDocument.body);
		const option = await body.findByRole("option", { name: "succeeded" });
		await userEvent.click(option);
	},
};

export const FilterByID: Story = {
	args: {
		jobs: [MockProvisionerJob],
		filter: {
			ids: MockProvisionerJob.id,
			status: "",
		},
	},
};
