import { MockInterception } from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import {
	getDefaultFilterProps,
	MockMenu,
} from "components/Filter/storyHelpers";
import {
	mockInitialRenderResult,
	mockSuccessResult,
} from "components/PaginationWidget/PaginationContainer.mocks";
import type { ComponentProps } from "react";
import { RequestLogsPageView } from "./RequestLogsPageView";

type FilterProps = ComponentProps<typeof RequestLogsPageView>["filterProps"];

const defaultFilterProps = getDefaultFilterProps<FilterProps>({
	query: "owner:me",
	values: {
		username: undefined,
		provider: undefined,
		model: undefined,
	},
	menus: {
		user: MockMenu,
		provider: MockMenu,
		model: MockMenu,
	},
});

const interceptions = [MockInterception, MockInterception, MockInterception];

const meta: Meta<typeof RequestLogsPageView> = {
	title: "pages/AIBridgePage/RequestLogsPageView",
	component: RequestLogsPageView,
	args: {},
};

export default meta;
type Story = StoryObj<typeof RequestLogsPageView>;

export const Paywall: Story = {
	args: {
		isRequestLogsVisible: false,
	},
};

export const Loaded: Story = {
	args: {
		isRequestLogsVisible: true,
		interceptions,
		filterProps: {
			...defaultFilterProps,
		},
		interceptionsQuery: mockSuccessResult,
	},
};

export const Empty: Story = {
	args: {
		isRequestLogsVisible: true,
		interceptions: [],
		filterProps: {
			...defaultFilterProps,
		},
		interceptionsQuery: mockSuccessResult,
	},
};

export const Loading: Story = {
	args: {
		isLoading: true,
		isRequestLogsVisible: true,
		interceptions: [],
		filterProps: {
			...defaultFilterProps,
		},
		interceptionsQuery: mockInitialRenderResult,
	},
};
