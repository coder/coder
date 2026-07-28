import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import type { FC } from "react";
import { useQueryClient } from "react-query";
import { expect, userEvent, within } from "storybook/test";
import { chatUsageLimitStatusKey } from "#/api/queries/chats";
import { getWorkspaceQuotaQueryKey } from "#/api/queries/workspaceQuota";
import { workspacesKey } from "#/api/queries/workspaces";
import type {
	ChatUsageLimitStatus,
	WorkspaceQuota,
	WorkspacesResponse,
} from "#/api/typesGenerated";
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

const withUsageLimitStatus = (status: ChatUsageLimitStatus) => (Story: FC) => {
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

// Mirrors the sidebar footer wrapper: a fixed-width container with
// container-type set so the trigger inside reacts to the wrapper's width
// instead of the viewport's.
const withUsageIndicatorFrame = (
	widthClassName = "w-[320px]",
	frameTestId?: string,
): Decorator => {
	return (Story) => (
		<div
			data-testid={frameTestId}
			className={`flex h-12 min-w-0 items-stretch justify-end rounded-md bg-surface-secondary [container-type:inline-size] ${widthClassName}`}
		>
			<Story />
		</div>
	);
};

const openUsageMenu = async (canvasElement: HTMLElement) => {
	const canvas = within(canvasElement);
	await userEvent.click(canvas.getByRole("button"));
};

const limitedUsageStatus = (
	overrides: Partial<ChatUsageLimitStatus> = {},
): ChatUsageLimitStatus => ({
	is_limited: true,
	period: "month",
	spend_limit_micros: 50_000_000,
	current_spend: 12_500_000,
	period_start: "2026-02-10T00:00:00Z",
	period_end: "2026-03-12T00:00:00Z",
	...overrides,
});

const unlimitedUsageStatus = {
	is_limited: false,
	current_spend: 0,
} satisfies ChatUsageLimitStatus;

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
		withUsageIndicatorFrame(),
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
		withUsageLimitStatus(limitedUsageStatus()),
		withWorkspaceQuota(noWorkspaceQuota),
	],
};

export const MediumUsage: Story = {
	decorators: [
		withUsageLimitStatus(
			limitedUsageStatus({
				period: "week",
				spend_limit_micros: 20_000_000,
				current_spend: 16_000_000,
			}),
		),
		withWorkspaceQuota(noWorkspaceQuota),
	],
};

export const HighUsage: Story = {
	decorators: [
		withUsageLimitStatus(
			limitedUsageStatus({
				period: "day",
				spend_limit_micros: 10_000_000,
				current_spend: 9_500_000,
			}),
		),
		withWorkspaceQuota(noWorkspaceQuota),
	],
};

export const LimitExceeded: Story = {
	decorators: [
		withUsageLimitStatus(
			limitedUsageStatus({
				spend_limit_micros: 30_000_000,
				current_spend: 32_000_000,
			}),
		),
		withWorkspaceQuota(noWorkspaceQuota),
	],
};

export const WorkspaceQuotaOnly: Story = {
	decorators: [
		withUsageLimitStatus(unlimitedUsageStatus),
		withWorkspaceQuota(defaultWorkspaceQuota),
		withWorkspaceCount(3),
	],
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		expect(
			canvas.getByRole("progressbar", { name: "Workspace quota usage" }),
		).toBeVisible();
		await openUsageMenu(canvasElement);
	},
};

export const UsageAndWorkspaceQuota: Story = {
	decorators: [
		withUsageLimitStatus(limitedUsageStatus()),
		withWorkspaceQuota(defaultWorkspaceQuota),
		withWorkspaceCount(3),
	],
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const progressBars = canvas.getAllByRole("progressbar");

		expect(canvas.getByRole("button", { name: "Usage" })).toBeVisible();
		expect(progressBars.map((bar) => bar.getAttribute("aria-label"))).toEqual([
			"Monthly spend usage",
			"Workspace quota usage",
		]);

		await openUsageMenu(canvasElement);
	},
};

export const TriggerTiny: Story = {
	decorators: [
		withUsageIndicatorFrame("w-[240px]", "usage-indicator-frame"),
		withUsageLimitStatus(limitedUsageStatus()),
		withWorkspaceQuota(defaultWorkspaceQuota),
		withWorkspaceCount(3),
	],
};

export const WorkspaceQuotaUnused: Story = {
	decorators: [
		withUsageLimitStatus(unlimitedUsageStatus),
		withWorkspaceQuota({
			credits_consumed: 0,
			budget: 100,
		}),
	],
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		expect(canvas.queryByRole("button")).not.toBeInTheDocument();
	},
};

export const WorkspaceQuotaWithoutBudget: Story = {
	decorators: [
		withUsageLimitStatus(unlimitedUsageStatus),
		withWorkspaceQuota({
			credits_consumed: 20,
			budget: 0,
		}),
		withWorkspaceCount(1),
	],
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const progressbar = canvas.getByRole("progressbar", {
			name: "Workspace quota usage",
		});

		expect(progressbar).toHaveAttribute("aria-valuenow", "100");

		await openUsageMenu(canvasElement);
		expect(within(document.body).getByText("100%")).toBeInTheDocument();
		expect(
			within(document.body).getByText("1 workspace using 20 of 0 credits"),
		).toBeInTheDocument();
	},
};

export const WorkspaceQuotaExceeded: Story = {
	decorators: [
		withUsageLimitStatus(unlimitedUsageStatus),
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
		withUsageLimitStatus(unlimitedUsageStatus),
		withWorkspaceQuota(defaultWorkspaceQuota),
		withUnavailableWorkspaceCount,
	],
	play: async ({ canvasElement }) => {
		await openUsageMenu(canvasElement);
	},
};

export const NotLimited: Story = {
	decorators: [
		withUsageLimitStatus(unlimitedUsageStatus),
		withWorkspaceQuota(noWorkspaceQuota),
	],
};
