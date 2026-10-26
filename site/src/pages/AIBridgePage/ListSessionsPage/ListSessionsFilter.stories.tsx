import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { fn, screen, userEvent, within } from "storybook/test";
import {
	aiBridgeClientsKey,
	aiBridgeModelsKey,
	aiBridgeProvidersKey,
} from "#/api/queries/aiBridge";
import { usersKey } from "#/api/queries/users";
import type { AIBridgeProvider } from "#/api/typesGenerated";
import type { DateTimeRangeValue } from "#/components/DateTimeRangePicker/dateTimeRange";
import type { UseFilterResult } from "#/components/Filter/Filter";
import {
	MockPermissions,
	MockUserMember,
	MockUserOwner,
	mockApiError,
} from "#/testHelpers/entities";
import { withAuthProvider } from "#/testHelpers/storybook";
import { ListSessionsFilter } from "./ListSessionsFilter";

const mockProviders: AIBridgeProvider[] = [
	{
		name: "anthropic-prod",
		type: "anthropic",
		display_name: "Anthropic (prod)",
		icon: "",
	},
	{ name: "openai-prod", type: "openai", display_name: "", icon: "" },
];

const timeRange: DateTimeRangeValue = {
	start: new Date("2026-08-12T15:00:00Z"),
	end: new Date("2026-08-13T15:00:00Z"),
	preset: "last_24h",
};

const rangeQuery =
	'started_after:"2026-08-12T15:00:00Z" started_before:"2026-08-13T15:00:00Z"';

// Stateful harness so `filter.update` feeds back into the combobox value the
// way the real `useFilter` hook does.
const ListSessionsFilterHarness = ({
	initialQuery = "",
	error,
	range = timeRange,
}: {
	initialQuery?: string;
	error?: unknown;
	range?: DateTimeRangeValue;
}) => {
	const [query, setQuery] = useState(initialQuery);
	const filter: UseFilterResult = {
		query,
		values: {},
		used: query.length > 0,
		update: (next) => setQuery(typeof next === "string" ? next : ""),
		debounceUpdate: (next) => setQuery(typeof next === "string" ? next : ""),
		cancelDebounce: () => {},
	};

	return (
		<ListSessionsFilter
			filter={filter}
			error={error}
			timeRange={range}
			onTimeRangeChange={fn()}
		/>
	);
};

const meta: Meta<typeof ListSessionsFilterHarness> = {
	title: "pages/AIBridgePage/ListSessionsFilter",
	component: ListSessionsFilterHarness,
	parameters: {
		user: MockUserOwner,
		permissions: MockPermissions,
		queries: [
			{ key: aiBridgeProvidersKey, data: mockProviders },
			{
				key: aiBridgeClientsKey("", 25),
				data: ["claude-code", "cursor"],
			},
			{
				key: aiBridgeModelsKey("", 25),
				data: ["claude-sonnet-4-5", "us.anthropic.claude-3-5-sonnet-v2:0"],
			},
			{
				key: usersKey({ q: "", limit: 25 }),
				data: { users: [MockUserOwner, MockUserMember], count: 2 },
			},
		],
	},
	decorators: [withAuthProvider],
};

export default meta;
type Story = StoryObj<typeof ListSessionsFilterHarness>;

export const Default: Story = {
	args: { initialQuery: rangeQuery },
};

export const WithFilters: Story = {
	args: {
		initialQuery: `initiator:me provider_name:anthropic-prod client:claude-code model:"us.anthropic.claude-3-5-sonnet-v2:0" ${rangeQuery}`,
	},
};

export const ExplicitTimeRange: Story = {
	args: {
		initialQuery:
			'started_after:"2026-08-01T09:30:00Z" started_before:"2026-08-02T17:45:00Z"',
		range: {
			start: new Date("2026-08-01T09:30:00"),
			end: new Date("2026-08-02T17:45:00"),
		},
	},
};

export const MenuOpen: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Filters" }),
		);
		await userEvent.hover(await screen.findByRole("option", { name: "Model" }));
	},
};

export const WithFilterError: Story = {
	args: {
		initialQuery: "claude",
		error: mockApiError({
			message: "Invalid filter query.",
			validations: [{ field: "q", detail: "no search key specified" }],
		}),
	},
};
