import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import type { ProvisionerJob } from "#/api/typesGenerated";
import type { PaginationResult } from "#/components/PaginationWidget/PaginationContainer";
import {
	mockInitialRenderResult,
	mockSuccessResult,
} from "#/components/PaginationWidget/PaginationContainer.mocks";
import { MockOrganization, MockProvisionerJob } from "#/testHelpers/entities";
import { daysAgo } from "#/utils/time";
import OrganizationProvisionerJobsPageView from "./OrganizationProvisionerJobsPageView";

const MockProvisionerJobs: ProvisionerJob[] = Array.from(
	{ length: 25 },
	(_, i) => ({
		...MockProvisionerJob,
		id: i.toString(),
		created_at: daysAgo(2),
	}),
);

const mockMultiPageResult = {
	...mockSuccessResult,
	totalPages: 4,
	totalRecords: 100,
	hasNextPage: true,
	hasPreviousPage: false,
	onPageChange: fn(),
	goToNextPage: fn(),
	goToPreviousPage: fn(),
} as const satisfies PaginationResult;

const meta: Meta<typeof OrganizationProvisionerJobsPageView> = {
	title: "pages/OrganizationProvisionerJobsPage",
	component: OrganizationProvisionerJobsPageView,
	args: {
		organization: MockOrganization,
		jobs: MockProvisionerJobs,
		jobsQuery: mockMultiPageResult,
		filter: { status: "", ids: "" },
		isNonInitialPage: false,
		onRetry: fn(),
		onFilterChange: fn(),
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
		jobsQuery: mockInitialRenderResult,
	},
};

export const LoadingError: Story = {
	args: {
		jobs: undefined,
		error: new Error("Failed to load jobs"),
		jobsQuery: mockInitialRenderResult,
	},
};

export const RetryAfterError: Story = {
	args: {
		jobs: undefined,
		error: new Error("Failed to load jobs"),
		onRetry: fn(),
		jobsQuery: mockInitialRenderResult,
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
		pixel: { exclude: true },
	},
};

export const Empty: Story = {
	args: {
		jobs: [],
		jobsQuery: {
			...mockSuccessResult,
			totalRecords: 0,
			totalPages: 0,
		},
	},
};

export const EmptyPage: Story = {
	args: {
		jobs: [],
		isNonInitialPage: true,
		jobsQuery: {
			...mockSuccessResult,
			currentPage: 2,
			currentOffsetStart: 26,
			totalRecords: 25,
			totalPages: 1,
			hasPreviousPage: true,
			hasNextPage: false,
		},
	},
};

export const Pagination: Story = {
	args: {
		jobsQuery: {
			...mockMultiPageResult,
			onPageChange: fn(),
		},
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const nextPage = await canvas.findByRole("button", { name: "Next page" });
		await userEvent.click(nextPage);

		await waitFor(() => {
			expect(args.jobsQuery.onPageChange).toHaveBeenCalledWith(2);
		});
	},
	parameters: {
		pixel: { exclude: true },
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
		jobsQuery: {
			...mockSuccessResult,
			totalRecords: 1,
			totalPages: 1,
		},
		filter: {
			ids: MockProvisionerJob.id,
			status: "",
		},
	},
};
