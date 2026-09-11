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
import { TemplatesFilter } from "./TemplatesFilter";

const TemplatesFilterHarness = ({
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
			<TemplatesFilter filter={filter} error={error} />
			<output data-testid="filter-query">{query}</output>
		</div>
	);
};

const meta: Meta<typeof TemplatesFilterHarness> = {
	title: "pages/TemplatesPage/TemplatesFilter",
	component: TemplatesFilterHarness,
	parameters: {
		user: MockUserOwner,
		permissions: MockPermissions,
	},
	decorators: [withAuthProvider, withDashboardProvider],
};

export default meta;
type Story = StoryObj<typeof TemplatesFilterHarness>;

const PLACEHOLDER = "Search and filter templates…";

export const Default: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("combobox", { name: PLACEHOLDER }),
		).toBeVisible();
	},
};

export const SelectDeprecatedOption: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.click(
			canvas.getByRole("button", { name: "Toggle filters" }),
		);
		await userEvent.click(
			await body.findByRole("option", { name: "Attributes" }),
		);
		await userEvent.click(
			await body.findByRole("option", { name: /deprecated/i }),
		);

		await waitFor(() =>
			expect(canvas.getByTestId("filter-query")).toHaveTextContent(
				"deprecated:true",
			),
		);
		await expect(
			canvas.getByRole("button", { name: "Remove deprecated:true" }),
		).toBeVisible();
	},
};

export const OrdinaryUserKeepsAuthorChip: Story = {
	args: { initialQuery: "author:me" },
	parameters: { permissions: MockNoPermissions },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);

		await expect(
			canvas.getByRole("button", { name: "Remove author:me" }),
		).toBeVisible();

		await userEvent.click(
			canvas.getByRole("button", { name: "Toggle filters" }),
		);
		await waitFor(() => {
			const names = body
				.getAllByRole("option")
				.map((option) => option.textContent?.trim());
			expect(names).toEqual(expect.arrayContaining(["Attributes", "Author"]));
		});
	},
};

export const WithFilterError: Story = {
	args: {
		initialQuery: "author:me",
		error: mockApiError({
			message: "Invalid template search query.",
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
