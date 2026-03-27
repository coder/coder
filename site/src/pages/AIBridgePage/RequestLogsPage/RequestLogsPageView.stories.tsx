import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ComponentProps } from "react";
import {
	getDefaultFilterProps,
	MockMenu,
} from "#/components/Filter/storyHelpers";
import {
	mockInitialRenderResult,
	mockSuccessResult,
} from "#/components/PaginationWidget/PaginationContainer.mocks";
import {
	MockInterception,
	MockInterceptionAnthropic,
	MockInterceptionCopilot,
} from "#/testHelpers/entities";
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
		model: MockMenu,
	},
});

const interceptions = [
	MockInterception,
	MockInterceptionAnthropic,
	MockInterceptionCopilot,
];

const meta: Meta<typeof RequestLogsPageView> = {
	title: "pages/AIBridgePage/RequestLogsPageView",
	component: RequestLogsPageView,
	args: {},
};

export default meta;
type Story = StoryObj<typeof RequestLogsPageView>;

export const Paywall: Story = {
	args: {
		isRequestLogsEntitled: false,
		isRequestLogsEnabled: false,
	},
};

export const NotEnabled: Story = {
	args: {
		isRequestLogsEntitled: true,
		isRequestLogsEnabled: false,
	},
};

export const Loaded: Story = {
	args: {
		isRequestLogsEntitled: true,
		isRequestLogsEnabled: true,
		interceptions,
		filterProps: {
			...defaultFilterProps,
		},
		interceptionsQuery: mockSuccessResult,
	},
};

export const Empty: Story = {
	args: {
		isRequestLogsEntitled: true,
		isRequestLogsEnabled: true,
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
		isRequestLogsEntitled: true,
		isRequestLogsEnabled: true,
		interceptions: [],
		filterProps: {
			...defaultFilterProps,
		},
		interceptionsQuery: mockInitialRenderResult,
	},
};
