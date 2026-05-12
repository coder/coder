import type { Meta, StoryObj } from "@storybook/react-vite";
import type { FC } from "react";
import { useQueryClient } from "react-query";
import { expect, userEvent, within } from "storybook/test";
import { chatUsageLimitStatusKey } from "#/api/queries/chats";
import { getWorkspaceQuotaQueryKey } from "#/api/queries/workspaceQuota";
import { workspacesKey } from "#/api/queries/workspaces";
import type { WorkspaceQuota, WorkspacesResponse } from "#/api/typesGenerated";
import {
	MockDefaultOrganization,
	MockPermissions,
	MockUserOwner,
} from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
} from "#/testHelpers/storybook";
import { UsageIndicator } from "./UsageIndicator";

const withUsageLimitStatus =
	(status: {
		is_limited: boolean;
		period?: "day" | "week" | "month";
		spend_limit_micros?: number;
		current_spend: number;
		period_start?: string;
		period_end?: string;
	}) =>
	(Story: FC) => {
		const queryClient = useQueryClient();
		queryClient.setQueryData(chatUsageLimitStatusKey, status);
		return <Story />;
	};

const withWorkspaceQuota = (quota: WorkspaceQuota) => (Story: FC) => {
	const queryClient = useQueryClient();
	queryClient.setQueryData(
		getWorkspaceQuotaQueryKey(
			MockDefaultOrganization.name,
			MockUserOwner.username,
		),
		quota,
	);
	return <Story />;
};

const withWorkspaceCount = (count: number) => (Story: FC) => {
	const queryClient = useQueryClient();
	queryClient.setQueryData(workspacesKey(userWorkspacesRequest), {
		workspaces: [],
		count,
	} satisfies WorkspacesResponse);
	return <Story />;
};

const withUnavailableWorkspaceCount = (Story: FC) => {
	const queryClient = useQueryClient();
	queryClient.setQueryData(workspacesKey(userWorkspacesRequest), {
		workspaces: [],
		count: -1,
	} satisfies WorkspacesResponse);
	return <Story />;
};

const withUsageIndicatorFrame = (Story: FC) => (
	<div className="flex h-12 w-[260px] items-stretch justify-end rounded-md bg-surface-secondary">
		<Story />
	</div>
);

const openUsageMenu = async (canvasElement: HTMLElement) => {
	const canvas = within(canvasElement);
	await userEvent.click(canvas.getByRole("button"));
};

const periodStart = new Date().toISOString();
const periodEnd = new Date(Date.now() + 30 * 24 * 60 * 60 * 1000).toISOString();
const userWorkspacesRequest = {
	q: `owner:me organization:${MockDefaultOrganization.name}`,
	limit: 0,
};
const noWorkspaceQuota = {
	credits_consumed: 0,
	budget: 0,
} satisfies WorkspaceQuota;
const defaultWorkspaceQuota = {
	credits_consumed: 30,
	budget: 100,
} satisfies WorkspaceQuota;

const meta: Meta<typeof UsageIndicator> = {
	title: "pages/AgentsPage/UsageIndicator",
	component: UsageIndicator,
	decorators: [
		withAuthProvider,
		withDashboardProvider,
		withUsageIndicatorFrame,
	],
	parameters: {
		user: MockUserOwner,
		permissions: MockPermissions,
	},
};

export default meta;
type Story = StoryObj<typeof UsageIndicator>;

export const LowUsage: Story = {
	decorators: [
		withUsageLimitStatus({
			is_limited: true,
			period: "month",
			spend_limit_micros: 50_000_000,
			current_spend: 12_500_000,
			period_start: periodStart,
			period_end: periodEnd,
		}),
		withWorkspaceQuota(noWorkspaceQuota),
	],
};

export const MediumUsage: Story = {
	decorators: [
		withUsageLimitStatus({
			is_limited: true,
			period: "week",
			spend_limit_micros: 20_000_000,
			current_spend: 16_000_000,
			period_start: periodStart,
			period_end: periodEnd,
		}),
		withWorkspaceQuota(noWorkspaceQuota),
	],
};

export const HighUsage: Story = {
	decorators: [
		withUsageLimitStatus({
			is_limited: true,
			period: "day",
			spend_limit_micros: 10_000_000,
			current_spend: 9_500_000,
			period_start: periodStart,
			period_end: periodEnd,
		}),
		withWorkspaceQuota(noWorkspaceQuota),
	],
};

export const LimitExceeded: Story = {
	decorators: [
		withUsageLimitStatus({
			is_limited: true,
			period: "month",
			spend_limit_micros: 30_000_000,
			current_spend: 32_000_000,
			period_start: periodStart,
			period_end: periodEnd,
		}),
		withWorkspaceQuota(noWorkspaceQuota),
	],
};

export const WorkspaceQuotaOnly: Story = {
	decorators: [
		withUsageLimitStatus({
			is_limited: false,
			current_spend: 0,
		}),
		withWorkspaceQuota(defaultWorkspaceQuota),
		withWorkspaceCount(3),
	],
	play: async ({ canvasElement }) => {
		await openUsageMenu(canvasElement);
	},
};

export const UsageAndWorkspaceQuota: Story = {
	decorators: [
		withUsageLimitStatus({
			is_limited: true,
			period: "month",
			spend_limit_micros: 50_000_000,
			current_spend: 12_500_000,
			period_start: periodStart,
			period_end: periodEnd,
		}),
		withWorkspaceQuota(defaultWorkspaceQuota),
		withWorkspaceCount(3),
	],
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const progressBars = canvas.getAllByRole("progressbar");

		expect(canvas.getByText("Usage")).toBeInTheDocument();
		expect(progressBars.map((bar) => bar.getAttribute("aria-label"))).toEqual([
			"Monthly spend usage",
			"Workspace quota usage",
		]);
		await userEvent.click(canvas.getByRole("button"));
	},
};

export const WorkspaceQuotaExceeded: Story = {
	decorators: [
		withUsageLimitStatus({
			is_limited: false,
			current_spend: 0,
		}),
		withWorkspaceQuota({
			credits_consumed: 125,
			budget: 100,
		}),
		withWorkspaceCount(7),
	],
	play: async ({ canvasElement }) => {
		await openUsageMenu(canvasElement);
	},
};

export const WorkspaceQuotaWithoutWorkspaceCount: Story = {
	decorators: [
		withUsageLimitStatus({
			is_limited: false,
			current_spend: 0,
		}),
		withWorkspaceQuota(defaultWorkspaceQuota),
		withUnavailableWorkspaceCount,
	],
	play: async ({ canvasElement }) => {
		await openUsageMenu(canvasElement);
	},
};

export const NotLimited: Story = {
	decorators: [
		withUsageLimitStatus({
			is_limited: false,
			current_spend: 0,
		}),
		withWorkspaceQuota(noWorkspaceQuota),
	],
};
