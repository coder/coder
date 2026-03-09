import { MockSession } from "testHelpers/entities";
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
import { AISessionListPageView } from "./AISessionListPageView";

type FilterProps = ComponentProps<typeof AISessionListPageView>["filterProps"];

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

const sessions = [MockSession];

const meta: Meta<typeof AISessionListPageView> = {
	title: "pages/AIBridgePage/AISessionListPageView",
	component: AISessionListPageView,
	args: {},
};

export default meta;
type Story = StoryObj<typeof AISessionListPageView>;

export const Paywall: Story = {
	args: {
		isAISessionsEntitled: false,
		isAISessionsEnabled: false,
	},
};

export const NotEnabled: Story = {
	args: {
		isAISessionsEntitled: true,
		isAISessionsEnabled: false,
	},
};

export const Loaded: Story = {
	args: {
		isAISessionsEntitled: true,
		isAISessionsEnabled: true,
		sessions,
		filterProps: {
			...defaultFilterProps,
		},
		sessionsQuery: mockSuccessResult,
	},
};

export const Empty: Story = {
	args: {
		isAISessionsEntitled: true,
		isAISessionsEnabled: true,
		sessions: [],
		filterProps: {
			...defaultFilterProps,
		},
		sessionsQuery: mockSuccessResult,
	},
};

export const Loading: Story = {
	args: {
		isLoading: true,
		isAISessionsEntitled: true,
		isAISessionsEnabled: true,
		sessions: [],
		filterProps: {
			...defaultFilterProps,
		},
		sessionsQuery: mockInitialRenderResult,
	},
};
