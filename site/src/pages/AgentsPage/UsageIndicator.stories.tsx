import type { Meta, StoryObj } from "@storybook/react-vite";
import { chatUsageLimitStatusKey } from "api/queries/chats";
import type { FC } from "react";
import { useQueryClient } from "react-query";
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

const periodStart = new Date().toISOString();
const periodEnd = new Date(Date.now() + 30 * 24 * 60 * 60 * 1000).toISOString();

const meta: Meta<typeof UsageIndicator> = {
	title: "pages/AgentsPage/UsageIndicator",
	component: UsageIndicator,
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
	],
};

export const NotLimited: Story = {
	decorators: [
		withUsageLimitStatus({
			is_limited: false,
			current_spend: 0,
		}),
	],
};
