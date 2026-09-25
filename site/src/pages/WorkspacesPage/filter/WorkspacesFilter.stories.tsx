import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { userEvent, within } from "storybook/test";
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

export const OrdinaryUserSeesSelfCategories: Story = {
	args: { initialQuery: "" },
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
};
