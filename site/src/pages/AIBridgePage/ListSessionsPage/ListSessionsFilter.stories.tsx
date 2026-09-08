import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ComponentProps } from "react";
import { expect, fn, screen, userEvent, waitFor, within } from "storybook/test";
import type { DateTimeRangeValue } from "#/components/DateTimeRangePicker/dateTimeRange";
import {
	getDefaultFilterProps,
	MockMenu,
} from "#/components/Filter/storyHelpers";
import { ListSessionsFilter } from "./ListSessionsFilter";

type FilterProps = ComponentProps<typeof ListSessionsFilter>;

type FilterAndMenus = Pick<FilterProps, "filter" | "menus">;

const timeRange: DateTimeRangeValue = {
	start: new Date("2026-08-12T15:00:00Z"),
	end: new Date("2026-08-13T15:00:00Z"),
	preset: "last_24h",
};

const timeRangeProps: Pick<FilterProps, "timeRange" | "onTimeRangeChange"> = {
	timeRange,
	onTimeRangeChange: fn(),
};

const defaultFilterProps = {
	...getDefaultFilterProps<FilterAndMenus>({
		query: "",
		values: {
			username: undefined,
			provider: undefined,
		},
		menus: {
			user: MockMenu,
			provider: MockMenu,
			client: MockMenu,
			model: MockMenu,
		},
	}),
	...timeRangeProps,
};

const meta: Meta<typeof ListSessionsFilter> = {
	title: "pages/AIBridgePage/ListSessionsFilter",
	component: ListSessionsFilter,
};

export default meta;
type Story = StoryObj<typeof ListSessionsFilter>;

export const Default: Story = {
	args: {
		...defaultFilterProps,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// The preset queries were removed, so the Filters button is gone
		// while the search field and dropdown menus remain.
		expect(canvas.queryByRole("button", { name: /filters/i })).toBeNull();
		expect(canvas.getByLabelText("Filter")).toBeVisible();

		// The picker trigger renders the committed preset label and opens
		// to the quick-pick list with that preset selected.
		await userEvent.click(
			canvas.getByRole("button", { name: /Last 24 hours/ }),
		);
		await waitFor(() => {
			expect(
				screen.getByRole("radio", { name: "Custom range" }),
			).toBeInTheDocument();
		});
		expect(screen.getByRole("radio", { name: "Last 24 hours" })).toBeChecked();
	},
};

export const WithQuery: Story = {
	args: {
		...getDefaultFilterProps<FilterAndMenus>({
			query: "initiator:me",
			values: {
				username: "me",
				provider: undefined,
			},
			menus: {
				user: MockMenu,
				provider: MockMenu,
				client: MockMenu,
				model: MockMenu,
			},
			used: true,
		}),
		...timeRangeProps,
	},
};

export const ExplicitTimeRange: Story = {
	args: {
		...defaultFilterProps,
		timeRange: {
			start: new Date("2026-08-01T09:30:00"),
			end: new Date("2026-08-02T17:45:00"),
		},
	},
};

export const Loading: Story = {
	args: {
		...defaultFilterProps,
		menus: {
			user: { ...MockMenu, isInitializing: true },
			provider: MockMenu,
			client: MockMenu,
			model: MockMenu,
		},
	},
};
