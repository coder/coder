import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ComponentProps } from "react";
import { fn } from "storybook/test";
import {
	getDefaultFilterProps,
	MockMenu,
} from "#/components/Filter/storyHelpers";
import { ListSessionsFilter } from "./ListSessionsFilter";
import type { TimeRange } from "./timeRange";

type FilterProps = ComponentProps<typeof ListSessionsFilter>;

type FilterAndMenus = Pick<FilterProps, "filter" | "menus">;

const timeRange: TimeRange = {
	startedAfter: new Date("2026-08-12T15:00:00Z"),
	startedBefore: new Date("2026-08-13T15:00:00Z"),
};

const timeRangeProps: Pick<
	FilterProps,
	"timeRange" | "defaultTimeRange" | "onTimeRangeChange"
> = {
	timeRange,
	defaultTimeRange: timeRange,
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
			startedAfter: new Date("2026-08-01T09:30:00"),
			startedBefore: new Date("2026-08-02T17:45:00"),
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
