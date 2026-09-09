import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import {
	expect,
	fn,
	screen,
	spyOn,
	userEvent,
	waitFor,
	within,
} from "storybook/test";
import { API } from "#/api/api";
import type { DateTimeRangeValue } from "#/components/DateTimeRangePicker/dateTimeRange";
import type { UseFilterResult } from "#/components/Filter/Filter";
import {
	parseFilterQuery,
	stringifyFilter,
} from "#/components/Filter/filterQuery";
import {
	MockAIProviderAnthropic,
	MockAIProviderOpenAI,
	MockPermissions,
	MockUserMember,
	MockUserOwner,
	mockApiError,
} from "#/testHelpers/entities";
import { withAuthProvider } from "#/testHelpers/storybook";
import { ListSessionsFilter } from "./ListSessionsFilter";

const timeRange: DateTimeRangeValue = {
	start: new Date("2026-08-12T15:00:00Z"),
	end: new Date("2026-08-13T15:00:00Z"),
	preset: "last_24h",
};

// The picker writes its range into the same `filter` query string as the
// combobox chips, so every story starts with the range already applied.
const TIME_RANGE_QUERY =
	'started_after:"2026-08-12T15:00:00Z" started_before:"2026-08-13T15:00:00Z"';

// Stateful harness so `filter.update` feeds back into the combobox value the way
// the real `useFilter` hook does, letting interactions assert the emitted query.
const ListSessionsFilterHarness = ({
	initialQuery = TIME_RANGE_QUERY,
	error,
	onUpdate,
}: {
	initialQuery?: string;
	error?: unknown;
	onUpdate?: (query: string) => void;
}) => {
	const [query, setQuery] = useState(initialQuery);
	const update: UseFilterResult["update"] = (next) => {
		const serialized = typeof next === "string" ? next : stringifyFilter(next);
		onUpdate?.(serialized);
		setQuery(serialized);
	};
	const filter: UseFilterResult = {
		query,
		values: parseFilterQuery(query),
		used: query.length > 0,
		update,
		debounceUpdate: update,
		cancelDebounce: () => {},
	};

	return (
		<div className="flex flex-col gap-2">
			<ListSessionsFilter
				filter={filter}
				error={error}
				timeRange={timeRange}
				onTimeRangeChange={fn()}
			/>
			<output data-testid="filter-query">{query}</output>
		</div>
	);
};

const meta: Meta<typeof ListSessionsFilterHarness> = {
	title: "pages/AIBridgePage/ListSessionsFilter",
	component: ListSessionsFilterHarness,
	args: { onUpdate: fn() },
	parameters: {
		user: MockUserOwner,
		permissions: MockPermissions,
	},
	decorators: [withAuthProvider],
	beforeEach: () => {
		spyOn(API, "getUsers").mockResolvedValue({
			users: [MockUserOwner, MockUserMember],
			count: 2,
		});
		spyOn(API.experimental, "listAIProviders").mockResolvedValue([
			MockAIProviderOpenAI,
			MockAIProviderAnthropic,
		]);
		spyOn(API, "getAIBridgeClients").mockResolvedValue([
			"Claude Code",
			"Codex",
		]);
		spyOn(API, "getAIBridgeModels").mockResolvedValue([
			"claude-sonnet-4-5",
			"gpt-5",
		]);
	},
};

export default meta;
type Story = StoryObj<typeof ListSessionsFilterHarness>;

const PLACEHOLDER = "Search and filter sessions…";

export const Default: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// The time range keys share the query string with the chips, but the
		// picker owns them, so they stay out of the combobox.
		await expect(
			canvas.getByRole("combobox", { name: PLACEHOLDER }),
		).toHaveValue("");
		await expect(canvas.queryByRole("button", { name: /^Remove/ })).toBeNull();
		await expect(
			canvas.getByRole("button", { name: /Last 24 hours/ }),
		).toBeVisible();
	},
};

export const OpenCategories: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.click(
			canvas.getByRole("button", { name: "Toggle filters" }),
		);

		await waitFor(() => {
			expect(body.getByRole("option", { name: /^User/ })).toBeVisible();
			expect(body.getByRole("option", { name: /^Provider/ })).toBeVisible();
			expect(body.getByRole("option", { name: /^Client/ })).toBeVisible();
			expect(body.getByRole("option", { name: /^Model/ })).toBeVisible();
		});
	},
};

export const AppliedProviderChip: Story = {
	args: { initialQuery: `provider_name:openai ${TIME_RANGE_QUERY}` },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(
			canvas.getByRole("button", { name: "Remove provider_name:openai" }),
		).toBeVisible();
		await expect(
			canvas.getByRole("combobox", { name: PLACEHOLDER }),
		).toHaveValue("");
	},
};

// Guards the split: committing and removing a chip must leave the picker's
// started_after / started_before pair untouched in the emitted query.
export const KeepsTimeRangeWhenChipsChange: Story = {
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.click(
			canvas.getByRole("button", { name: "Toggle filters" }),
		);
		await userEvent.click(
			await body.findByRole("option", { name: /^Provider/ }),
		);
		await userEvent.click(await body.findByRole("option", { name: "OpenAI" }));

		await waitFor(() =>
			expect(args.onUpdate).toHaveBeenLastCalledWith(
				`provider_name:openai ${TIME_RANGE_QUERY}`,
			),
		);

		await userEvent.click(
			canvas.getByRole("button", { name: "Remove provider_name:openai" }),
		);

		await waitFor(() =>
			expect(args.onUpdate).toHaveBeenLastCalledWith(TIME_RANGE_QUERY),
		);
		await expect(canvas.getByTestId("filter-query")).toHaveTextContent(
			TIME_RANGE_QUERY,
		);
	},
};

export const SelectUserOption: Story = {
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.click(
			canvas.getByRole("button", { name: "Toggle filters" }),
		);
		await userEvent.click(await body.findByRole("option", { name: /^User/ }));
		await userEvent.click(
			await body.findByRole("option", { name: MockUserMember.username }),
		);

		await waitFor(() =>
			expect(args.onUpdate).toHaveBeenLastCalledWith(
				`initiator:${MockUserMember.username} ${TIME_RANGE_QUERY}`,
			),
		);
	},
};

export const WithFilterError: Story = {
	args: {
		error: mockApiError({
			message: "Invalid filter query.",
			validations: [
				{ field: "q", detail: 'Query param "q" has an invalid value.' },
			],
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const input = canvas.getByRole("combobox", { name: PLACEHOLDER });
		await expect(input).toHaveAttribute("aria-invalid", "true");
		const alert = await canvas.findByRole("alert");
		await expect(input).toHaveAttribute("aria-errormessage", alert.id);
	},
};

export const ExplicitTimeRange: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// The picker trigger opens the quick-pick list with the committed preset
		// selected.
		await userEvent.click(
			canvas.getByRole("button", { name: /Last 24 hours/ }),
		);
		await waitFor(() => {
			expect(
				screen.getByRole("radio", { name: "Custom range" }),
			).toBeInTheDocument();
		});
		await expect(
			screen.getByRole("radio", { name: "Last 24 hours" }),
		).toBeChecked();
	},
};
