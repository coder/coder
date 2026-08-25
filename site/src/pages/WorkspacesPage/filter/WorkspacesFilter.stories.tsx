import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";
import type { UseFilterResult } from "#/components/Filter/Filter";
import {
	MockNoPermissions,
	MockPermissions,
	MockUserOwner,
	mockApiError,
} from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
} from "#/testHelpers/storybook";
import { WorkspacesFilter } from "./WorkspacesFilter";

// Stateful harness so `filter.update` feeds back into the combobox value the way
// the real `useFilter` hook does, letting interactions assert the emitted query.
const WorkspacesFilterHarness = ({
	initialQuery = "",
	error,
}: {
	initialQuery?: string;
	error?: unknown;
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
		<div className="flex flex-col gap-2">
			<WorkspacesFilter filter={filter} error={error} />
			<output data-testid="filter-query">{query}</output>
		</div>
	);
};

const meta: Meta<typeof WorkspacesFilterHarness> = {
	title: "pages/WorkspacesPage/WorkspacesFilter",
	component: WorkspacesFilterHarness,
	parameters: {
		user: MockUserOwner,
		permissions: MockPermissions,
	},
	decorators: [withAuthProvider, withDashboardProvider],
};

export default meta;
type Story = StoryObj<typeof WorkspacesFilterHarness>;

const PLACEHOLDER = "Search and filter workspaces…";

export const Default: Story = {
	args: { initialQuery: "owner:me" },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// The default `owner:me` renders as a committed chip, not free text.
		await expect(
			canvas.getByRole("button", { name: "Remove owner:me" }),
		).toBeVisible();
	},
};

// Opens the menu, drills into a static category, commits an option, and asserts
// the query the integration emits.
export const SelectStatusOption: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.click(
			canvas.getByRole("button", { name: "Toggle filters" }),
		);
		await userEvent.click(await body.findByRole("option", { name: "Status" }));
		await userEvent.click(
			await body.findByRole("option", { name: /running/i }),
		);

		await waitFor(() =>
			expect(canvas.getByTestId("filter-query")).toHaveTextContent(
				"status:running",
			),
		);
		await expect(
			canvas.getByRole("button", { name: "Remove status:running" }),
		).toBeVisible();
	},
};

// Regression guard: a user who cannot list others still gets an Owner category
// (scoped to themselves), so `owner` stays a chip key and the category list is
// browsable instead of `owner:me` collapsing into free text.
export const OrdinaryUserKeepsOwnerChip: Story = {
	args: { initialQuery: "owner:me" },
	parameters: { permissions: MockNoPermissions },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);

		await expect(
			canvas.getByRole("button", { name: "Remove owner:me" }),
		).toBeVisible();

		await userEvent.click(
			canvas.getByRole("button", { name: "Toggle filters" }),
		);
		// Categories browse normally (Owner included) rather than being masked by
		// free-text search.
		await waitFor(() => {
			const names = body
				.getAllByRole("option")
				.map((option) => option.textContent?.trim());
			expect(names).toEqual(expect.arrayContaining(["Status", "Owner"]));
		});
	},
};

export const WithFilterError: Story = {
	args: {
		initialQuery: "owner:me",
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
