import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ComponentProps } from "react";
import { fn } from "storybook/test";
import {
	getDefaultFilterProps,
	MockMenu,
} from "#/components/Filter/storyHelpers";
import {
	mockInitialRenderResult,
	mockSuccessResult,
} from "#/components/PaginationWidget/PaginationContainer.mocks";
import { MockSession } from "#/testHelpers/entities";
import { ListSessionsPageView } from "./ListSessionsPageView";

type FilterProps = ComponentProps<typeof ListSessionsPageView>["filterProps"];

const defaultFilterProps = getDefaultFilterProps<FilterProps>({
	query: "owner:me",
	values: {
		username: undefined,
		provider: undefined,
	},
	menus: {
		user: MockMenu,
		provider: MockMenu,
		client: MockMenu,
	},
});

const meta: Meta<typeof ListSessionsPageView> = {
	title: "pages/AIBridgePage/ListSessionsPageView",
	component: ListSessionsPageView,
	args: {
		isLoading: false,
		isFetching: false,
		isAISessionsEntitled: true,
		isAISessionsEnabled: true,
		filterProps: defaultFilterProps,
		sessionsQuery: mockSuccessResult,
		onSessionRowClick: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof ListSessionsPageView>;

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

export const Loading: Story = {
	args: {
		isLoading: true,
		sessions: undefined,
		sessionsQuery: mockInitialRenderResult,
	},
};

export const Empty: Story = {
	args: {
		sessions: [],
	},
};

export const Loaded: Story = {
	args: {
		sessions: [MockSession],
	},
};

export const Fetching: Story = {
	args: {
		isFetching: true,
		sessions: [MockSession],
	},
};

export const MultipleSessions: Story = {
	args: {
		sessions: Array.from({ length: 5 }, (_, i) => ({
			...MockSession,
			id: `session-${i}`,
			threads: i + 1,
			last_prompt: [
				"But *can* I really fix it?",
				"Can you refactor the entire authentication module to use JWT tokens instead of session cookies?",
				"What's the best way to handle errors in Go?",
				"Help me write a Terraform module for a Kubernetes cluster.",
				"Explain how the agentic loop works in this codebase.",
			][i],
			token_usage_summary: {
				input_tokens: 1000 * (i + 1),
				output_tokens: 300 * (i + 1),
			},
		})),
	},
};
