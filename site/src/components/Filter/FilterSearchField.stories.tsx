import { mockApiError } from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import type { UseFilterResult } from "./Filter";
import { FilterSearchField } from "./Filter";

const mockFilter: UseFilterResult = {
	query: "",
	values: {},
	used: false,
	update: fn(),
	debounceUpdate: fn(),
	cancelDebounce: fn(),
};

const meta: Meta<typeof FilterSearchField> = {
	title: "components/Filter/FilterSearchField",
	component: FilterSearchField,
	args: {
		filter: mockFilter,
	},
};

export default meta;
type Story = StoryObj<typeof FilterSearchField>;

export const Empty: Story = {};

export const WithValue: Story = {
	args: {
		filter: { ...mockFilter, query: "owner:me status:running", used: true },
	},
};

export const WithValidationError: Story = {
	args: {
		error: mockApiError({
			message: "Invalid filter query",
			validations: [{ field: "filter", detail: "Unknown filter key: ownerr" }],
		}),
	},
};

export const Typing: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const input = canvas.getByRole("textbox", { name: /filter/i });
		await userEvent.click(input);
		await userEvent.type(input, "owner:me");
		await expect(input).toHaveValue("owner:me");
	},
};

export const Clearing: Story = {
	args: {
		filter: { ...mockFilter, query: "owner:me", used: true },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: /clear/i }));
	},
};
