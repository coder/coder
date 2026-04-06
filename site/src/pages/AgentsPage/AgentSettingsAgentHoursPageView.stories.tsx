import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, within } from "storybook/test";
import type { ChatRuntimeSummary } from "#/api/typesGenerated";
import { AgentSettingsAgentHoursPageView } from "./AgentSettingsAgentHoursPageView";

// Generate 30 days of mock runtime data with realistic variation.
function generateMockDaily(days: number): ChatRuntimeSummary["daily"] {
	const daily: Array<ChatRuntimeSummary["daily"][number]> = [];
	const base = new Date("2026-03-01T00:00:00Z");
	for (let i = 0; i < days; i++) {
		const date = new Date(base);
		date.setDate(base.getDate() + i);
		// Weekdays get more usage than weekends.
		const dayOfWeek = date.getDay();
		const isWeekend = dayOfWeek === 0 || dayOfWeek === 6;
		const baseMs = isWeekend ? 1_800_000 : 7_200_000;
		// Add some random-looking variation using a simple deterministic
		// pattern so the story renders the same every time.
		const variation = ((i * 7 + 3) % 11) / 11;
		const totalRuntimeMs = Math.round(baseMs * (0.5 + variation));
		const messageCount = Math.round(totalRuntimeMs / 120_000);

		daily.push({
			date: date.toISOString(),
			total_runtime_ms: totalRuntimeMs,
			message_count: messageCount,
		});
	}
	return daily;
}

const mockDaily = generateMockDaily(30);
const totalRuntimeMs = mockDaily.reduce((s, d) => s + d.total_runtime_ms, 0);
const projectedYearlyRuntimeMs = Math.round((totalRuntimeMs / 30) * 365);

const mockSummary: ChatRuntimeSummary = {
	start_date: "2026-03-01T00:00:00Z",
	end_date: "2026-03-31T00:00:00Z",
	total_runtime_ms: totalRuntimeMs,
	daily: mockDaily,
	projected_yearly_runtime_ms: projectedYearlyRuntimeMs,
};

const meta = {
	title: "pages/AgentsPage/AgentSettingsAgentHoursPageView",
	component: AgentSettingsAgentHoursPageView,
	args: {
		data: undefined,
		isLoading: false,
		error: undefined,
		onRetry: fn(),
		rangeLabel: "Mar 1 – Mar 31, 2026",
	},
} satisfies Meta<typeof AgentSettingsAgentHoursPageView>;

export default meta;
type Story = StoryObj<typeof AgentSettingsAgentHoursPageView>;

export const WithData: Story = {
	args: {
		data: mockSummary,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await canvas.findByText("Agent Hours");
		await expect(canvas.getByText("Total in Period")).toBeInTheDocument();
		await expect(canvas.getByText("Projected Yearly")).toBeInTheDocument();
		await expect(canvas.getByText("Daily Runtime")).toBeInTheDocument();
	},
};

export const Loading: Story = {
	args: {
		isLoading: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await canvas.findByText("Agent Hours");
		// Summary cards and chart should not be visible while loading.
		expect(canvas.queryByText("Total in Period")).not.toBeInTheDocument();
	},
};

export const ErrorState: Story = {
	args: {
		error: new global.Error("Something went wrong"),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await canvas.findByText("Failed to load runtime data.");
		await expect(canvas.getByText("Retry")).toBeInTheDocument();
	},
};

export const Empty: Story = {
	args: {
		data: {
			start_date: "2026-03-01T00:00:00Z",
			end_date: "2026-03-31T00:00:00Z",
			total_runtime_ms: 0,
			daily: [],
			projected_yearly_runtime_ms: 0,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await canvas.findByText("Agent Hours");
		await expect(
			canvas.getByText("No runtime data available for this period."),
		).toBeInTheDocument();
	},
};

export const SmallValues: Story = {
	args: {
		data: {
			start_date: "2026-03-01T00:00:00Z",
			end_date: "2026-03-31T00:00:00Z",
			total_runtime_ms: 180_000, // 3 minutes total
			daily: [
				{
					date: "2026-03-15T00:00:00Z",
					total_runtime_ms: 60_000,
					message_count: 2,
				},
				{
					date: "2026-03-16T00:00:00Z",
					total_runtime_ms: 120_000,
					message_count: 3,
				},
			],
			projected_yearly_runtime_ms: Math.round((180_000 / 30) * 365),
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await canvas.findByText("Agent Hours");
		// Small values should render in minutes.
		await expect(canvas.getByText("3.0m")).toBeInTheDocument();
	},
};

export const LargeValues: Story = {
	args: {
		data: {
			start_date: "2026-03-01T00:00:00Z",
			end_date: "2026-03-31T00:00:00Z",
			total_runtime_ms: 5_400_000_000, // 1500 hours
			daily: generateMockDaily(30).map((d) => ({
				...d,
				total_runtime_ms: d.total_runtime_ms * 100,
			})),
			projected_yearly_runtime_ms: Math.round((5_400_000_000 / 30) * 365),
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await canvas.findByText("Agent Hours");
		// Large values should use the "k hrs" format.
		await expect(canvas.getByText("1.5k hrs")).toBeInTheDocument();
	},
};
