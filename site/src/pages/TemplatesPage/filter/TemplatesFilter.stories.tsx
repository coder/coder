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

	return <TemplatesFilter filter={filter} error={error} />;
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

export const Default: Story = {};

export const Attributes: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(canvas.getByRole("button", { name: "Filters" }));
		await userEvent.click(
			await body.findByRole("option", { name: /^Attributes/ }),
		);
	},
};

export const Organization: Story = {
	parameters: { showOrganizations: true },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(canvas.getByRole("button", { name: "Filters" }));
		await userEvent.click(
			await body.findByRole("option", { name: /^Organization/ }),
		);
	},
};

export const SelectDeprecatedOption: Story = {
	play: async (context) => {
		await Attributes.play?.(context);
		await userEvent.click(
			await within(context.canvasElement.ownerDocument.body).findByRole(
				"button",
				{ name: /deprecated/i },
			),
		);
	},
};

export const OrdinaryUserKeepsAuthorChip: Story = {
	args: { initialQuery: "author:me" },
	parameters: { permissions: MockNoPermissions },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(canvas.getByRole("button", { name: "Filters" }));
		await userEvent.click(await body.findByRole("option", { name: /^Author/ }));
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
};

export const Mobile: Story = {
	...Attributes,
	globals: { viewport: { value: "iphone12" } },
};
