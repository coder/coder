import { MockInterception } from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import {
	getDefaultFilterProps,
	MockMenu,
} from "components/Filter/storyHelpers";
import { mockSuccessResult } from "components/PaginationWidget/PaginationContainer.mocks";
import type { UsePaginatedQueryResult } from "hooks/usePaginatedQuery";
import type { ComponentProps } from "react";
import { RequestLogsPageView } from "./RequestLogsPageView";

type FilterProps = ComponentProps<typeof RequestLogsPageView>["filterProps"];

const defaultFilterProps = getDefaultFilterProps<FilterProps>({
	query: "owner:me",
	values: {
		username: undefined,
		provider: undefined,
	},
	menus: {
		user: MockMenu,
		provider: MockMenu,
	},
});

const interceptions = [MockInterception, MockInterception, MockInterception];

const meta: Meta<typeof RequestLogsPageView> = {
	title: "pages/AIGovernancePage/RequestLogsPageView",
	component: RequestLogsPageView,
	args: {},
};

export default meta;
type Story = StoryObj<typeof RequestLogsPageView>;

export const NotEnabled: Story = {
	args: {
		isRequestLogsVisible: false,
	},
};

export const WithLogs: Story = {
	args: {
		isRequestLogsVisible: true,
		interceptions,
		filterProps: {
			...defaultFilterProps,
		},
		interceptionsQuery: {
			...mockSuccessResult,
			totalRecords: interceptions.length,
			data: { interceptions, total: interceptions.length },
		} as UsePaginatedQueryResult,
	},
};

export const EmptyLogs: Story = {
	args: {
		isRequestLogsVisible: true,
		interceptions: [],
		filterProps: {
			...defaultFilterProps,
		},
		interceptionsQuery: {
			...mockSuccessResult,
			totalRecords: 0,
			data: { interceptions: [], total: 0 },
		} as UsePaginatedQueryResult,
	},
};
