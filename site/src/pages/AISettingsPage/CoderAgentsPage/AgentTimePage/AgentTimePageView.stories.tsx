import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, screen, userEvent, within } from "storybook/test";
import type { AgentTimeReport } from "#/api/typesGenerated";
import type { PaginationResult } from "#/components/PaginationWidget/PaginationContainer";
import {
	MockAgentTimeBackfillFailureReport,
	MockAgentTimeBackfillingReport,
	MockAgentTimeEmptyReport,
	MockAgentTimeNow,
	MockAgentTimeOrganizationOneId,
	MockAgentTimeOrganizationReport,
	MockAgentTimePartialHistoryReport,
	MockAgentTimeReport,
	MockAgentTimeUserOneId,
	mockAgentTimeReportWithCount,
} from "#/testHelpers/agentTime";
import AgentTimePageView, { type AgentTimeQuery } from "./AgentTimePageView";

function localDate(year: number, month: number, day: number): Date {
	return new Date(year, month - 1, day);
}

function makePagination(
	report: AgentTimeReport,
): PaginationResult<AgentTimeReport> {
	return {
		data: report,
		isPlaceholderData: false,
		currentPage: 1,
		limit: 25,
		onPageChange: fn(),
		goToPreviousPage: fn(),
		goToNextPage: fn(),
		goToFirstPage: fn(),
		isSuccess: true,
		hasNextPage: report.count > 25,
		hasPreviousPage: false,
		totalRecords: report.count,
		totalPages: Math.max(1, Math.ceil(report.count / 25)),
		currentOffsetStart: report.count > 0 ? 1 : 0,
		countIsCapped: false,
	};
}

function makeQuery(report: AgentTimeReport): AgentTimeQuery {
	return {
		...makePagination(report),
		isLoading: false,
		isFetching: false,
		error: null,
		refetch: fn(),
	};
}

function makeLoadingQuery(): AgentTimeQuery {
	return {
		data: undefined,
		isLoading: true,
		isFetching: true,
		error: null,
		refetch: fn(),
		isPlaceholderData: false,
		currentPage: 1,
		limit: 25,
		onPageChange: fn(),
		goToPreviousPage: fn(),
		goToNextPage: fn(),
		goToFirstPage: fn(),
		isSuccess: false,
		hasNextPage: false,
		hasPreviousPage: false,
		totalRecords: undefined,
		totalPages: undefined,
		currentOffsetStart: undefined,
		countIsCapped: false,
	};
}

function makeErrorQuery(): AgentTimeQuery {
	return {
		...makeLoadingQuery(),
		isLoading: false,
		isFetching: false,
		error: new Error("Failed to load agent time"),
	};
}

function firstElement<T>(items: readonly T[], label: string): T {
	const first = items.at(0);
	if (first === undefined) {
		throw new Error(`${label} was not found`);
	}
	return first;
}

const meta: Meta<typeof AgentTimePageView> = {
	title: "pages/AISettingsPage/CoderAgentsPage/AgentTimePageView",
	component: AgentTimePageView,
	args: {
		query: makeQuery(MockAgentTimeReport),
		now: MockAgentTimeNow,
		dateRange: {
			startDate: localDate(2026, 8, 6),
			endDate: localDate(2026, 9, 4),
		},
		activePreset: "last_30_days",
		isAllHistory: false,
		endDate: "2026-09-05",
		interval: "day",
		sortBy: "agent_time",
		sortOrder: "desc",
		tableGroup: "organization",
		onGroupChange: fn(),
		selectedOrganizationId: undefined,
		selectedUserId: undefined,
		onDateRangeChange: fn(),
		onPresetChange: fn(),
		onIntervalChange: fn(),
		onSortChange: fn(),
		onSelectOrganization: fn(),
		onClearOrganization: fn(),
		onSelectUser: fn(),
		onClearUser: fn(),
		onRetry: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof AgentTimePageView>;

export const Default: Story = {
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.findByRole("heading", { name: "Agent time" }),
		).resolves.toBeInTheDocument();
		await expect(await canvas.findByRole("application")).toBeVisible();
		await expect(await canvas.findByText("12h")).toBeVisible();
		await userEvent.click(canvas.getByRole("button", { name: "Last 7 days" }));
		await expect(args.onPresetChange).toHaveBeenCalledWith("last_7_days");
	},
};

export const Loading: Story = {
	args: {
		query: makeLoadingQuery(),
	},
};

export const LoadError: Story = {
	args: {
		query: makeErrorQuery(),
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: "Retry" }));
		await expect(args.onRetry).toHaveBeenCalled();
	},
};

export const Empty: Story = {
	args: {
		query: makeQuery(MockAgentTimeEmptyReport),
	},
};

export const Refreshing: Story = {
	args: {
		query: {
			...makeQuery(MockAgentTimeReport),
			isFetching: true,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByRole("status")).toHaveTextContent(
			"Refreshing agent time...",
		);
	},
};

export const Backfilling: Story = {
	args: {
		query: makeQuery(MockAgentTimeBackfillingReport),
	},
};

export const BackfillFailure: Story = {
	args: {
		query: makeQuery(MockAgentTimeBackfillFailureReport),
	},
};

export const PartialHistory: Story = {
	args: {
		query: makeQuery(MockAgentTimePartialHistoryReport),
		activePreset: "all_history",
		isAllHistory: true,
		interval: "month",
	},
};

export const IntervalSelection: Story = {
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("combobox", { name: "Chart interval" }),
		);
		await userEvent.click(
			await screen.findByRole("option", { name: "Weekly" }),
		);
		await expect(args.onIntervalChange).toHaveBeenCalledWith("week");
	},
};

export const OrganizationSelection: Story = {
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		const button = firstElement(
			canvas.getAllByRole("button", { name: "View users" }),
			"View users button",
		);
		await userEvent.click(button);
		await expect(args.onSelectOrganization).toHaveBeenCalledWith(
			MockAgentTimeOrganizationOneId,
		);
	},
};

export const OrganizationDrilldown: Story = {
	args: {
		query: makeQuery(MockAgentTimeOrganizationReport),
		tableGroup: "user",
		selectedOrganizationId: MockAgentTimeOrganizationOneId,
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		const button = firstElement(
			canvas.getAllByRole("button", { name: "View user" }),
			"View user button",
		);
		await userEvent.click(button);
		await expect(args.onSelectUser).toHaveBeenCalledWith(
			MockAgentTimeUserOneId,
		);
	},
};

export const UserDrilldown: Story = {
	args: {
		query: makeQuery(MockAgentTimeOrganizationReport),
		tableGroup: "user",
		selectedOrganizationId: MockAgentTimeOrganizationOneId,
		selectedUserId: MockAgentTimeUserOneId,
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: /clear user/i }));
		await expect(args.onClearUser).toHaveBeenCalled();
	},
};

export const KeyboardSorting: Story = {
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		const nameSort = canvas.getByRole("button", { name: "Name" });
		nameSort.focus();
		await userEvent.keyboard("{Enter}");
		await expect(args.onSortChange).toHaveBeenCalledWith("name");
	},
};

export const Paginated: Story = {
	args: {
		query: makeQuery(mockAgentTimeReportWithCount(75)),
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: "Next page" }));
		await expect(args.query.onPageChange).toHaveBeenCalledWith(2);
	},
};

export const RefetchFailureKeepsReport: Story = {
	args: {
		query: {
			...makeQuery(MockAgentTimeReport),
			error: new Error("Temporary report error"),
		},
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByText("Temporary report error"),
		).toBeVisible();
		await expect(canvas.getByText("50.00 hours")).toBeVisible();
		await expect(
			canvas.getByRole("table", { name: "Organizations agent time" }),
		).toBeVisible();
		await userEvent.click(canvas.getByRole("button", { name: "Retry" }));
		await expect(args.onRetry).toHaveBeenCalled();
	},
};

export const KeyboardAccessibleTable: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const region = await canvas.findByRole("region", {
			name: "Organizations table, scroll horizontally",
		});
		region.focus();
		await userEvent.keyboard("{ArrowRight}");
		await expect(region).toHaveFocus();
		await expect(
			canvas.getByRole("table", { name: "Organizations agent time" }),
		).toBeVisible();
	},
};
