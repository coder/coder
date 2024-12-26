import { action } from "@storybook/addon-actions";
import type { Meta, StoryObj } from "@storybook/react";
import { expect, userEvent, within } from "@storybook/test";
import { Avatar } from "components/Avatar/Avatar";
import { useState } from "react";
import { withDesktopViewport } from "testHelpers/storybook";
import {
	SelectFilter,
	type SelectFilterOption,
	SelectFilterSearch,
} from "./SelectFilter";

const options: SelectFilterOption[] = Array.from({ length: 50 }, (_, i) => ({
	startIcon: <Avatar fallback={`username ${i + 1}`} size="sm" />,
	label: `Option ${i + 1}`,
	value: `option-${i + 1}`,
}));

const meta: Meta<typeof SelectFilter> = {
	title: "components/SelectFilter",
	component: SelectFilter,
	args: {
		options,
		placeholder: "All options",
	},
	decorators: [withDesktopViewport],
	render: function SelectFilterWithState(args) {
		const [selectedOption, setSelectedOption] = useState<
			SelectFilterOption | undefined
		>(args.selectedOption);
		return (
			<SelectFilter
				{...args}
				selectedOption={selectedOption}
				onSelect={setSelectedOption}
			/>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const button = canvas.getByRole("button");
		await userEvent.click(button);
	},
};

export default meta;
type Story = StoryObj<typeof SelectFilter>;

export const Closed: Story = {
	play: () => {},
};

export const Open: Story = {};

export const Selected: Story = {
	args: {
		selectedOption: options[25],
	},
};

export const WithSearch: Story = {
	args: {
		selectedOption: options[25],
		selectFilterSearch: (
			<SelectFilterSearch
				value=""
				onChange={action("onSearch")}
				placeholder="Search options..."
			/>
		),
	},
};

export const LoadingOptions: Story = {
	args: {
		options: undefined,
	},
};

export const NoOptionsFound: Story = {
	args: {
		options: [],
	},
};

export const SelectingOption: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const button = canvas.getByRole("button");
		await userEvent.click(button);
		const option = canvas.getByText("Option 25");
		await userEvent.click(option);
		await expect(button).toHaveTextContent("Option 25");
	},
};

export const UnselectingOption: Story = {
	args: {
		selectedOption: options[25],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const button = canvas.getByRole("button");
		await userEvent.click(button);
		const menu = canvasElement.querySelector<HTMLElement>("[role=menu]")!;
		const option = within(menu).getByText("Option 26");
		await userEvent.click(option);
		await expect(button).toHaveTextContent("All options");
	},
};

export const SearchingOption: Story = {
	render: function SelectFilterWithSearch(args) {
		const [selectedOption, setSelectedOption] = useState<
			SelectFilterOption | undefined
		>(args.selectedOption);
		const [search, setSearch] = useState("");
		const visibleOptions = options.filter((option) =>
			option.value.includes(search),
		);

		return (
			<SelectFilter
				{...args}
				selectedOption={selectedOption}
				onSelect={setSelectedOption}
				options={visibleOptions}
				selectFilterSearch={
					<SelectFilterSearch
						value={search}
						onChange={setSearch}
						placeholder="Search options..."
						inputProps={{ "aria-label": "Search options" }}
					/>
				}
			/>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const button = canvas.getByRole("button");
		await userEvent.click(button);
		const search = canvas.getByLabelText("Search options");
		await userEvent.type(search, "option-2");
	},
};
