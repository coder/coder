import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { OrganizationAISpendOverviewInterval } from "#/api/typesGenerated";
import { MockOrganizationAISpendOverview } from "#/testHelpers/entities";
import { SpendTrendsCard } from "./SpendTrendsCard";

// A full 31-day month whose last days have no spend yet.
const mockMonthIntervals: OrganizationAISpendOverviewInterval[] = Array.from(
	{ length: 31 },
	(_, i) => {
		const day = String(i + 1).padStart(2, "0");
		const nextDay =
			i === 30 ? "2026-04-01" : `2026-03-${String(i + 2).padStart(2, "0")}`;
		return {
			start_time: `2026-03-${day}T00:00:00Z`,
			end_time: `${nextDay}T00:00:00Z`,
			spend_micros: i < 18 ? ((i * 37) % 11) * 45_000_000 + 20_000_000 : 0,
		};
	},
);

const meta = {
	title: "pages/AISettingsPage/AISpendPage/SpendTrendsCard",
	component: SpendTrendsCard,
	args: {
		days: { startDate: "2026-03-06", endDate: "2026-03-12" },
		intervals: MockOrganizationAISpendOverview.intervals,
	},
	decorators: [
		(Story) => (
			<div className="max-w-xl">
				<Story />
			</div>
		),
	],
} satisfies Meta<typeof SpendTrendsCard>;

export default meta;
type Story = StoryObj<typeof SpendTrendsCard>;

export const Week: Story = {};

export const Month: Story = {
	args: {
		days: { startDate: "2026-03-01", endDate: "2026-03-31" },
		intervals: mockMonthIntervals,
	},
};

export const SingleDay: Story = {
	args: {
		days: { startDate: "2026-03-12", endDate: "2026-03-12" },
		intervals: MockOrganizationAISpendOverview.intervals.slice(-1),
	},
};

export const Loading: Story = {
	args: { intervals: undefined },
};

export const NoSpend: Story = {
	args: {
		intervals: MockOrganizationAISpendOverview.intervals.map((interval) => ({
			...interval,
			spend_micros: 0,
		})),
	},
};

export const TooltipOpen: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Wait for the chart to mount so it can receive focus.
		await canvas.findByRole("group", { name: "AI spend trends" });
		await userEvent.tab();
		await userEvent.keyboard("{ArrowRight}");
	},
};
