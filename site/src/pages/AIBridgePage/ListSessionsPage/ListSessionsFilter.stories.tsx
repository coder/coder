import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { fn, screen, userEvent, within } from "storybook/test";
import { aiBridgeClientsKey, aiBridgeModelsKey } from "#/api/queries/aiBridge";
import { aiProvidersIncludingDeletedKey } from "#/api/queries/aiProviders";
import { permittedOrganizationsKey } from "#/api/queries/organizations";
import { usersKey } from "#/api/queries/users";
import type { DateTimeRangeValue } from "#/components/DateTimeRangePicker/dateTimeRange";
import type { UseFilterResult } from "#/components/Filter/Filter";
import {
	MockAIProviderAnthropic,
	MockAIProviders,
	MockDefaultOrganization,
	MockOrganization2,
	MockPermissions,
	MockUserMember,
	MockUserOwner,
	mockApiError,
} from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
} from "#/testHelpers/storybook";
import { ListSessionsFilter } from "./ListSessionsFilter";

const timeRange: DateTimeRangeValue = {
	start: new Date("2026-08-12T15:00:00Z"),
	end: new Date("2026-08-13T15:00:00Z"),
	preset: "last_24h",
};

const rangeQuery =
	'started_after:"2026-08-12T15:00:00Z" started_before:"2026-08-13T15:00:00Z"';

const ListSessionsFilterHarness = ({
	initialQuery = "",
	error,
	range = timeRange,
	minDate,
}: {
	initialQuery?: string;
	error?: unknown;
	range?: DateTimeRangeValue;
	minDate?: Date;
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
			minDate={minDate}
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
			{ key: aiProvidersIncludingDeletedKey, data: MockAIProviders },
			{
				key: aiBridgeClientsKey({ q: "", limit: 25 }),
				data: ["claude-code", "cursor"],
			},
			{
				key: aiBridgeModelsKey({ model: "", limit: 25 }),
				data: ["claude-sonnet-4-5", "us.anthropic.claude-3-5-sonnet-v2:0"],
			},
			{
				key: usersKey({ q: " include_system:true", limit: 25 }),
				data: { users: [MockUserOwner, MockUserMember], count: 2 },
			},
		],
	},
	decorators: [withAuthProvider, withDashboardProvider],
};

export default meta;
type Story = StoryObj<typeof ListSessionsFilterHarness>;

export const Default: Story = {
	args: { initialQuery: rangeQuery },
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("combobox", { name: "Active" }),
		);
	},
};

export const WithFilters: Story = {
	args: {
		initialQuery: `user:me provider_name:${MockAIProviderAnthropic.name} client:claude-code model:"us.anthropic.claude-3-5-sonnet-v2:0" ${rangeQuery}`,
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

/** Times before the retention can't be picked. */
export const DatePickerOpenWithRetention: Story = {
	args: {
		initialQuery: rangeQuery,
		minDate: new Date("2026-08-10T15:00:00Z"),
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("combobox", { name: "Active" }),
		);
		await userEvent.click(
			await screen.findByRole("radio", { name: "Custom range" }),
		);
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

export const WithDefaultOrganization: Story = {
	args: {
		initialQuery: `organization:${MockDefaultOrganization.name} ${rangeQuery}`,
	},
	parameters: {
		showOrganizations: true,
		organizations: [MockDefaultOrganization, MockOrganization2],
		queries: [
			{
				key: permittedOrganizationsKey({
					object: { resource_type: "audit_log" },
					action: "read",
				}),
				data: [MockDefaultOrganization, MockOrganization2],
			},
		],
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Filters" }),
		);
		await userEvent.hover(
			await screen.findByRole("option", { name: "Organization" }),
		);
	},
};
