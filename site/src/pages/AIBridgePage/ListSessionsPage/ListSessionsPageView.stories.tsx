import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ComponentProps } from "react";
import { expect, fn, waitFor } from "storybook/test";
import {
	getDefaultFilterProps,
	MockMenu,
} from "#/components/Filter/storyHelpers";
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
		model: MockMenu,
	},
});

const meta: Meta<typeof ListSessionsPageView> = {
	title: "pages/AIBridgePage/ListSessionsPageView",
	component: ListSessionsPageView,
	args: {
		isLoading: false,
		isAISessionsEntitled: true,
		isAISessionsEnabled: true,
		filterProps: defaultFilterProps,
		hasNextPage: false,
		isFetchingNextPage: false,
		onFetchNextPage: fn(),
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

export const LoadsMoreWhenSentinelVisible: Story = {
	args: {
		sessions: [MockSession],
		hasNextPage: true,
	},
	play: async ({ args }) => {
		await waitFor(() => expect(args.onFetchNextPage).toHaveBeenCalled());
	},
};

export const FetchingNextPage: Story = {
	args: {
		sessions: [MockSession],
		hasNextPage: true,
		isFetchingNextPage: true,
	},
	play: async ({ canvas }) => {
		await expect(
			await canvas.findByTitle("Loading spinner"),
		).toBeInTheDocument();
	},
};

export const MultipleSessions: Story = {
	args: {
		sessions: Array.from({ length: 5 }, (_, i) => ({
			...MockSession,
			id: `session-${i}`,
			threads: i + 1,
			providers: i % 2 === 0 ? ["anthropic", "openai"] : ["anthropic"],
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
				cache_read_input_tokens: 800 * (i + 1),
				cache_write_input_tokens: 50 * (i + 1),
			},
			network_calls: [
				{ total: 23, blocked: 2 },
				{ total: 5, blocked: 1 },
				{ total: 0, blocked: 0 },
				undefined,
				{ total: 150, blocked: 0 },
			][i],
		})),
	},
};
