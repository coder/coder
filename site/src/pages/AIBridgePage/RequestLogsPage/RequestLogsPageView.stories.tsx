import {
	MockInterception,
	MockInterceptionAnthropic,
	MockInterceptionCopilot,
} from "testHelpers/entities";
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
	},
};

export const Loaded: Story = {
	args: {
		isRequestLogsEntitled: true,
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
		interceptions: [],
		filterProps: {
			...defaultFilterProps,
		},
		interceptionsQuery: mockInitialRenderResult,
	},
};
