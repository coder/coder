import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, within } from "storybook/test";
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
	args: { initialQuery: "user:me" },
};

export const SelectStatusOption: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.click(canvas.getByRole("button", { name: "Filters" }));
		await userEvent.click(
			await body.findByRole("option", { name: /running/i }),
		);
	},
};

// The default `user:me` renders as an `owner:me` chip with the scope pill, not as free text.
export const OrdinaryUserGetsOwnerChip: Story = {
	args: { initialQuery: "user:me" },
	parameters: { permissions: MockNoPermissions },
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Filters" }),
		);
	},
};

export const WithFilterError: Story = {
	args: {
		initialQuery: "user:me",
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

// With no filter applied, an ordinary user still sees the Owner and User rows,
// though each offers only themselves.
export const OrdinaryUserSeesSelfCategories: Story = {
	args: { initialQuery: "" },
	parameters: { permissions: MockNoPermissions },
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Filters" }),
		);
	},
};
