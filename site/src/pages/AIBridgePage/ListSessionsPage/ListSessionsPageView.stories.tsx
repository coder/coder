import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ComponentProps } from "react";
import { fn } from "storybook/test";
import type { DateTimeRangeValue } from "#/components/DateTimeRangePicker/dateTimeRange";
import { parseFilterQuery } from "#/components/Filter/filterQuery";
import {
	mockInitialRenderResult,
	mockSuccessResult,
} from "#/components/PaginationWidget/PaginationContainer.mocks";
import {
	MockPermissions,
	MockSession,
	MockUserOwner,
} from "#/testHelpers/entities";
import { withAuthProvider } from "#/testHelpers/storybook";
import { ListSessionsPageView } from "./ListSessionsPageView";

type FilterProps = ComponentProps<typeof ListSessionsPageView>["filterProps"];

const timeRange: DateTimeRangeValue = {
	start: new Date("2026-08-12T15:00:00Z"),
	end: new Date("2026-08-13T15:00:00Z"),
	preset: "last_24h",
};

// The picker's range and the combobox chips share one filter query string.
const filterQuery =
	'provider_name:openai started_after:"2026-08-12T15:00:00Z" started_before:"2026-08-13T15:00:00Z"';

const defaultFilterProps: FilterProps = {
	filter: {
		query: filterQuery,
		values: parseFilterQuery(filterQuery),
		used: true,
		update: fn(),
		debounceUpdate: fn(),
		cancelDebounce: fn(),
	},
	timeRange,
	onTimeRangeChange: fn(),
};

const meta: Meta<typeof ListSessionsPageView> = {
	title: "pages/AIBridgePage/ListSessionsPageView",
	component: ListSessionsPageView,
	parameters: {
		user: MockUserOwner,
		permissions: MockPermissions,
	},
	decorators: [withAuthProvider],
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
